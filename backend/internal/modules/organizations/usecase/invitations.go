package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// InvitationTemplateCode is the notification template seeded by migration
// 000032 (vars: org_name, inviter_name, role_name, link, expires_days).
const InvitationTemplateCode = events.TenantInvitation

// InvitationRepository is the storage the invitation flow needs on top of
// Repository. *repository.Postgres implements both.
type InvitationRepository interface {
	CreateInvitation(ctx context.Context, q *db.Queries, params db.CreateOrganizationInvitationParams) (db.OrganizationInvitation, error)
	HasPendingOwnerInvitationForEmail(ctx context.Context, q *db.Queries, orgID int64, email string) (bool, error)
	RevokePendingInvitationsForEmail(ctx context.Context, q *db.Queries, orgID int64, email string) error
	ListPendingInvitations(ctx context.Context, orgID int64, limit, offset int32) ([]db.ListPendingOrganizationInvitationsRow, int64, error)
	RevokeInvitation(ctx context.Context, orgID int64, id uuid.UUID, allowOwner bool) error
	GetInvitationByTokenHash(ctx context.Context, hash string) (db.GetOrganizationInvitationByTokenHashRow, error)
	AcceptInvitation(ctx context.Context, q *db.Queries, id int64) error
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
}

// Notifier enqueues outbound notifications (never sends inline).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// InvitationConfig controls invitation links.
type InvitationConfig struct {
	TTL         time.Duration
	FrontendURL string
}

type invitations struct {
	repo     InvitationRepository
	notifier Notifier
	cfg      InvitationConfig
	log      *slog.Logger
}

// SetInvitations enables the invitation flow. Without it every invitation
// method returns ErrInvitationsDisabled.
func (s *Service) SetInvitations(repo InvitationRepository, notifier Notifier, cfg InvitationConfig, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	s.inv = &invitations{repo: repo, notifier: notifier, cfg: cfg, log: log}
}

// CheckRoleChange enforces the owner-escalation rule for tenant admins: only
// an owner may hand out, take away or remove the owner role.
func CheckRoleChange(actorRole, fromRole, toRole string) error {
	if actorRole == orgctx.RoleOwner {
		return nil
	}
	if fromRole == orgctx.RoleOwner || toRole == orgctx.RoleOwner {
		return ErrOwnerRoleRequired
	}
	return nil
}

// ListInvitations returns the open invitations of an organization.
func (s *Service) ListInvitations(ctx context.Context, orgID int64, limit, offset int32) ([]orgmodel.Invitation, int64, error) {
	if s.inv == nil {
		return nil, 0, ErrInvitationsDisabled
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.inv.repo.ListPendingInvitations(ctx, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	out := make([]orgmodel.Invitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, orgmodel.Invitation{
			UUID:      row.Uuid,
			Email:     row.Email,
			Role:      orgmodel.RoleSummary{Key: row.RoleKey, Name: row.RoleName},
			Status:    invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, now),
			ExpiresAt: row.ExpiresAt.Time,
			CreatedAt: row.CreatedAt.Time,
			InvitedBy: fullName(row.InviterName, row.InviterSurname),
		})
	}
	return out, total, nil
}

// CreateInvitation stores a new invitation (replacing older open ones for the
// same address) and enqueues the invitation email.
func (s *Service) CreateInvitation(ctx context.Context, in orgmodel.CreateInvitationInput) (orgmodel.Invitation, error) {
	if s.inv == nil {
		return orgmodel.Invitation{}, ErrInvitationsDisabled
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	roleKey := normalizeRole(in.RoleKey)
	if err := CheckRoleChange(in.InviterRoleKey, "", roleKey); err != nil {
		return orgmodel.Invitation{}, err
	}

	// An existing account that is already a member needs no invitation.
	invitee, err := s.inv.repo.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		if _, mErr := s.repo.GetOrganizationMemberByUserUUID(ctx, in.OrgID, invitee.Uuid); mErr == nil {
			return orgmodel.Invitation{}, ErrAlreadyMember
		} else if !errors.Is(mErr, repository.ErrNotFound) {
			return orgmodel.Invitation{}, mErr
		}
	case !errors.Is(err, repository.ErrNotFound):
		return orgmodel.Invitation{}, err
	}

	if strings.TrimSpace(in.InviterName) == "" && in.InviterUserID > 0 {
		if u, uErr := s.repo.GetUserByID(ctx, in.InviterUserID); uErr == nil {
			in.InviterName = strings.TrimSpace(u.Name + " " + u.Surname)
		}
	}

	token, hash, err := newInvitationToken()
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	expires := s.now().Add(s.inv.cfg.TTL)

	var (
		row  db.OrganizationInvitation
		role db.Role
	)
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		role, err = s.repo.GetTenantRoleByKey(ctx, q, in.OrgID, roleKey)
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		if in.InviterRoleKey != orgctx.RoleOwner {
			// Replacing an open owner invitation would revoke it.
			pending, err := s.inv.repo.HasPendingOwnerInvitationForEmail(ctx, q, in.OrgID, email)
			if err != nil {
				return err
			}
			if pending {
				return ErrOwnerRoleRequired
			}
		}
		if err := s.inv.repo.RevokePendingInvitationsForEmail(ctx, q, in.OrgID, email); err != nil {
			return err
		}
		row, err = s.inv.repo.CreateInvitation(ctx, q, db.CreateOrganizationInvitationParams{
			OrganizationID: in.OrgID,
			Email:          email,
			RoleID:         role.ID,
			TokenHash:      hash,
			ExpiresAt:      pgtype.Timestamptz{Time: expires, Valid: true},
			InvitedBy:      pgtype.Int8{Int64: in.InviterUserID, Valid: in.InviterUserID > 0},
		})
		return err
	})
	if err != nil {
		return orgmodel.Invitation{}, err
	}

	s.sendInvitationEmail(ctx, in, email, role.Name, token, invitee)

	inviter := strings.TrimSpace(in.InviterName)
	var invitedBy *string
	if inviter != "" {
		invitedBy = &inviter
	}
	return orgmodel.Invitation{
		UUID:      row.Uuid,
		Email:     row.Email,
		Role:      orgmodel.RoleSummary{Key: role.Slug, Name: role.Name},
		Status:    orgmodel.InvitationPending,
		ExpiresAt: row.ExpiresAt.Time,
		CreatedAt: row.CreatedAt.Time,
		InvitedBy: invitedBy,
	}, nil
}

// sendInvitationEmail enqueues the email. A failure is logged, not returned:
// the invitation exists and the admin can re-send it (a new invitation
// replaces the old one).
func (s *Service) sendInvitationEmail(ctx context.Context, in orgmodel.CreateInvitationInput, email, roleName, token string, invitee db.User) {
	if s.inv.notifier == nil {
		return
	}
	link := strings.TrimRight(s.inv.cfg.FrontendURL, "/") + "/invite/" + token
	days := int(s.inv.cfg.TTL.Hours() / 24)
	if days < 1 {
		days = 1
	}
	inviter := strings.TrimSpace(in.InviterName)
	enqueue := notifmodel.EnqueueInput{
		Channels:     []string{notifmodel.ChannelEmail},
		Recipient:    &email,
		TemplateCode: InvitationTemplateCode,
		SourceEvent:  InvitationTemplateCode,
		// Delivered regardless of the invitee's email preferences: it is
		// addressed to them on purpose and carries a single-use link.
		SecurityEmail: true,
		TemplateVars: map[string]string{
			"org_name":     in.OrgName,
			"inviter_name": inviter,
			"role_name":    roleName,
			"link":         link,
			"expires_days": strconv.Itoa(days),
		},
	}
	if invitee.ID > 0 {
		// Known account: render in their language. Never link the row to the
		// account (UserID): the address may be unverified, and the link must
		// only be readable from the mailbox.
		enqueue.Language = invitee.Locale
	}
	if _, err := s.inv.notifier.Enqueue(ctx, enqueue); err != nil {
		s.inv.log.Warn("organization_invitation_email_enqueue_failed", "error", err, "organization_id", in.OrgID)
	}
}

// RevokeInvitation cancels an open invitation of the organization. Only an
// owner may cancel an owner-role invitation; for other actors it is not found.
func (s *Service) RevokeInvitation(ctx context.Context, orgID int64, id uuid.UUID, actorRole string) error {
	if s.inv == nil {
		return ErrInvitationsDisabled
	}
	allowOwner := actorRole == orgctx.RoleOwner
	return mapRepoNotFound(s.inv.repo.RevokeInvitation(ctx, orgID, id, allowOwner), ErrInvitationNotFound)
}

// PreviewInvitation describes an invitation to the holder of its link.
func (s *Service) PreviewInvitation(ctx context.Context, token string) (orgmodel.InvitationPreview, error) {
	row, err := s.lookupInvitation(ctx, token)
	if err != nil {
		return orgmodel.InvitationPreview{}, err
	}
	var out orgmodel.InvitationPreview
	out.Organization.Slug = row.OrganizationSlug
	out.Organization.Name = row.OrganizationName
	out.Email = row.Email
	out.Role = orgmodel.RoleSummary{Key: row.RoleKey, Name: row.RoleName}
	out.Status = invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, s.now())
	out.ExpiresAt = row.ExpiresAt.Time
	out.InvitedBy = fullName(row.InviterName, row.InviterSurname)
	return out, nil
}

// AcceptInvitation turns an invitation into a membership for the signed-in
// user. The account email must be the invited address; the invitation is
// consumed atomically (single use) in the same transaction as the insert.
func (s *Service) AcceptInvitation(ctx context.Context, token string, userID int64) (orgmodel.AcceptedInvitation, error) {
	row, err := s.lookupInvitation(ctx, token)
	if err != nil {
		return orgmodel.AcceptedInvitation{}, err
	}
	if invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, s.now()) != orgmodel.InvitationPending {
		return orgmodel.AcceptedInvitation{}, ErrInvitationUnavailable
	}
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return orgmodel.AcceptedInvitation{}, mapRepoNotFound(err, ErrUserNotFound)
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), row.Email) {
		return orgmodel.AcceptedInvitation{}, ErrInvitationEmailMismatch
	}
	if _, err := s.repo.GetOrganizationMemberByUserUUID(ctx, row.OrganizationID, user.Uuid); err == nil {
		// Keep the invitation: accepting must not silently change a role.
		return orgmodel.AcceptedInvitation{}, ErrAlreadyMember
	} else if !errors.Is(err, repository.ErrNotFound) {
		return orgmodel.AcceptedInvitation{}, err
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.inv.repo.AcceptInvitation(ctx, q, row.ID); err != nil {
			// Raced with another accept, a revoke or the expiry instant.
			return mapRepoNotFound(err, ErrInvitationUnavailable)
		}
		return s.repo.AddOrganizationMember(ctx, q, db.AddOrganizationMemberParams{
			OrganizationID: row.OrganizationID, UserID: user.ID, RoleID: row.RoleID,
		})
	})
	if err != nil {
		return orgmodel.AcceptedInvitation{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.OrganizationID)
	return orgmodel.AcceptedInvitation{
		OrganizationSlug: row.OrganizationSlug,
		Role:             orgmodel.RoleSummary{Key: row.RoleKey, Name: row.RoleName},
	}, nil
}

func (s *Service) lookupInvitation(ctx context.Context, token string) (db.GetOrganizationInvitationByTokenHashRow, error) {
	if s.inv == nil {
		return db.GetOrganizationInvitationByTokenHashRow{}, ErrInvitationsDisabled
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return db.GetOrganizationInvitationByTokenHashRow{}, ErrInvitationNotFound
	}
	row, err := s.inv.repo.GetInvitationByTokenHash(ctx, hashInvitationToken(token))
	if err != nil {
		return row, mapRepoNotFound(err, ErrInvitationNotFound)
	}
	if row.OrganizationDeleted {
		return row, ErrInvitationNotFound
	}
	return row, nil
}

func invitationStatus(accepted, revoked, expires pgtype.Timestamptz, now time.Time) string {
	switch {
	case accepted.Valid:
		return orgmodel.InvitationAccepted
	case revoked.Valid:
		return orgmodel.InvitationRevoked
	case !now.Before(expires.Time):
		return orgmodel.InvitationExpired
	default:
		return orgmodel.InvitationPending
	}
}

// newInvitationToken returns a 256-bit URL-safe token and the hash stored
// for it. Only the hash is persisted; the token travels in the email link.
func newInvitationToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("invitation token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashInvitationToken(token), nil
}

func hashInvitationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeEmail(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || len(v) > 254 {
		return "", fmt.Errorf("%w: email is invalid", ErrInvalidRequest)
	}
	return v, nil
}

func fullName(name, surname pgtype.Text) *string {
	full := strings.TrimSpace(strings.TrimSpace(name.String) + " " + strings.TrimSpace(surname.String))
	if full == "" {
		return nil
	}
	return &full
}
