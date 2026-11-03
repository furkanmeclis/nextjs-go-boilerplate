package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeInvitationRepo struct {
	f      *fakeRepo
	nextID int64
	rows   map[int64]*db.OrganizationInvitation
}

func newFakeInvitationRepo(f *fakeRepo) *fakeInvitationRepo {
	return &fakeInvitationRepo{f: f, nextID: 1, rows: map[int64]*db.OrganizationInvitation{}}
}

func (r *fakeInvitationRepo) CreateInvitation(_ context.Context, _ *db.Queries, p db.CreateOrganizationInvitationParams) (db.OrganizationInvitation, error) {
	row := db.OrganizationInvitation{
		ID: r.nextID, Uuid: uuid.New(), OrganizationID: p.OrganizationID, Email: p.Email,
		RoleID: p.RoleID, TokenHash: p.TokenHash, ExpiresAt: p.ExpiresAt, InvitedBy: p.InvitedBy,
		CreatedAt: pgtype.Timestamptz{Time: r.f.now, Valid: true},
	}
	r.nextID++
	r.rows[row.ID] = &row
	return row, nil
}

func (r *fakeInvitationRepo) RevokePendingInvitationsForEmail(_ context.Context, _ *db.Queries, orgID int64, email string) error {
	for _, row := range r.rows {
		if row.OrganizationID == orgID && row.Email == email && !row.AcceptedAt.Valid && !row.RevokedAt.Valid {
			row.RevokedAt = pgtype.Timestamptz{Time: r.f.now, Valid: true}
		}
	}
	return nil
}

func (r *fakeInvitationRepo) ListPendingInvitations(_ context.Context, orgID int64, _, _ int32) ([]db.ListPendingOrganizationInvitationsRow, int64, error) {
	var out []db.ListPendingOrganizationInvitationsRow
	for _, row := range r.rows {
		if row.OrganizationID == orgID && !row.AcceptedAt.Valid && !row.RevokedAt.Valid {
			out = append(out, db.ListPendingOrganizationInvitationsRow{
				ID: row.ID, Uuid: row.Uuid, Email: row.Email, ExpiresAt: row.ExpiresAt,
				RoleKey: r.roleKey(row.RoleID), RoleName: r.roleKey(row.RoleID),
			})
		}
	}
	return out, int64(len(out)), nil
}

func (r *fakeInvitationRepo) RevokeInvitation(_ context.Context, orgID int64, id uuid.UUID) error {
	for _, row := range r.rows {
		if row.Uuid == id && row.OrganizationID == orgID && !row.AcceptedAt.Valid && !row.RevokedAt.Valid {
			row.RevokedAt = pgtype.Timestamptz{Time: r.f.now, Valid: true}
			return nil
		}
	}
	return repository.ErrNotFound
}

func (r *fakeInvitationRepo) GetInvitationByTokenHash(_ context.Context, hash string) (db.GetOrganizationInvitationByTokenHashRow, error) {
	for _, row := range r.rows {
		if row.TokenHash == hash {
			org := r.f.orgs[row.OrganizationID]
			return db.GetOrganizationInvitationByTokenHashRow{
				ID: row.ID, Uuid: row.Uuid, OrganizationID: row.OrganizationID, Email: row.Email,
				RoleID: row.RoleID, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt,
				AcceptedAt: row.AcceptedAt, RevokedAt: row.RevokedAt,
				OrganizationUuid: org.Uuid, OrganizationSlug: org.Slug, OrganizationName: org.Name,
				OrganizationStatus: org.Status, RoleKey: r.roleKey(row.RoleID), RoleName: r.roleKey(row.RoleID),
			}, nil
		}
	}
	return db.GetOrganizationInvitationByTokenHashRow{}, repository.ErrNotFound
}

func (r *fakeInvitationRepo) AcceptInvitation(_ context.Context, _ *db.Queries, id int64) error {
	row, ok := r.rows[id]
	if !ok || row.AcceptedAt.Valid || row.RevokedAt.Valid || !r.f.now.Before(row.ExpiresAt.Time) {
		return repository.ErrNotFound
	}
	row.AcceptedAt = pgtype.Timestamptz{Time: r.f.now, Valid: true}
	return nil
}

func (r *fakeInvitationRepo) GetUserByEmail(_ context.Context, email string) (db.User, error) {
	for _, u := range r.f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return db.User{}, repository.ErrNotFound
}

func (r *fakeInvitationRepo) roleKey(id int64) string {
	for k, v := range r.f.roleIDs {
		if v == id {
			return k
		}
	}
	return ""
}

type fakeNotifier struct{ sent []notifmodel.EnqueueInput }

func (n *fakeNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.sent = append(n.sent, in)
	return nil, nil
}

type invFixture struct {
	f     *fakeRepo
	inv   *fakeInvitationRepo
	notif *fakeNotifier
	svc   *Service
	org   db.Organization
	owner db.User
}

func newInvFixture(t *testing.T) invFixture {
	t.Helper()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	f := newFakeRepo(now)
	org := f.addOrg("acme", "active", now.Add(-time.Hour), nil)
	owner := db.User{ID: 10, Uuid: uuid.New(), Email: "owner@example.test", Name: "Olcay", Surname: "Sahip", Status: "active"}
	f.users[owner.ID] = owner
	f.addMembership(org.ID, owner.ID, "owner")
	inv := newFakeInvitationRepo(f)
	notif := &fakeNotifier{}
	svc := New(f, nil)
	svc.now = func() time.Time { return f.now }
	svc.SetInvitations(inv, notif, InvitationConfig{TTL: 7 * 24 * time.Hour, FrontendURL: "https://app.example.test/"}, nil)
	return invFixture{f: f, inv: inv, notif: notif, svc: svc, org: org, owner: owner}
}

// tokenFromLink extracts the bearer token from the enqueued email link.
func tokenFromLink(t *testing.T, n *fakeNotifier) string {
	t.Helper()
	if len(n.sent) == 0 {
		t.Fatal("no invitation email enqueued")
	}
	link := n.sent[len(n.sent)-1].TemplateVars["link"]
	const prefix = "https://app.example.test/invite/"
	if !strings.HasPrefix(link, prefix) {
		t.Fatalf("link = %q", link)
	}
	return strings.TrimPrefix(link, prefix)
}

func (x invFixture) invite(t *testing.T, email, role, actorRole string) (orgmodel.Invitation, string) {
	t.Helper()
	got, err := x.svc.CreateInvitation(context.Background(), orgmodel.CreateInvitationInput{
		OrgID: x.org.ID, OrgName: "Acme", InviterUserID: x.owner.ID, InviterRoleKey: actorRole,
		Email: email, RoleKey: role,
	})
	if err != nil {
		t.Fatalf("invite %s: %v", email, err)
	}
	return got, tokenFromLink(t, x.notif)
}

func TestCreateInvitationStoresHashAndEnqueuesEmail(t *testing.T) {
	t.Parallel()
	x := newInvFixture(t)
	got, token := x.invite(t, "  New.Person@Example.TEST ", "admin", "owner")

	if got.Email != "new.person@example.test" || got.Role.Key != "admin" || got.Status != orgmodel.InvitationPending {
		t.Fatalf("invitation = %+v", got)
	}
	stored := x.inv.rows[1]
	if stored.TokenHash == token || stored.TokenHash != hashInvitationToken(token) {
		t.Fatal("token must be stored only as its hash")
	}
	if len(token) < 40 {
		t.Fatalf("token too short: %d", len(token))
	}
	mail := x.notif.sent[0]
	if mail.TemplateCode != InvitationTemplateCode || *mail.Recipient != "new.person@example.test" || !mail.SecurityEmail {
		t.Fatalf("email = %+v", mail)
	}
	if mail.TemplateVars["inviter_name"] != "Olcay Sahip" || mail.TemplateVars["expires_days"] != "7" {
		t.Fatalf("template vars = %v", mail.TemplateVars)
	}
}

func TestReinviteRevokesOlderOpenInvitation(t *testing.T) {
	t.Parallel()
	x := newInvFixture(t)
	_, first := x.invite(t, "a@example.test", "member", "owner")
	_, _ = x.invite(t, "a@example.test", "member", "owner")
	if prev, _ := x.svc.PreviewInvitation(context.Background(), first); prev.Status != orgmodel.InvitationRevoked {
		t.Fatalf("first invitation status = %q, want revoked", prev.Status)
	}
	items, total, err := x.svc.ListInvitations(context.Background(), x.org.ID, 50, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("pending = %d/%d err %v", len(items), total, err)
	}
}

func TestInvitationValidation(t *testing.T) {
	t.Parallel()
	x := newInvFixture(t)
	ctx := context.Background()
	base := orgmodel.CreateInvitationInput{OrgID: x.org.ID, InviterUserID: x.owner.ID, RoleKey: "member"}

	bad := base
	bad.Email, bad.InviterRoleKey = "not-an-email", "owner"
	if _, err := x.svc.CreateInvitation(ctx, bad); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("bad email err = %v", err)
	}
	// Admins may not hand out the owner role.
	esc := base
	esc.Email, esc.RoleKey, esc.InviterRoleKey = "b@example.test", "owner", "admin"
	if _, err := x.svc.CreateInvitation(ctx, esc); !errors.Is(err, ErrOwnerRoleRequired) {
		t.Fatalf("admin inviting owner err = %v", err)
	}
	// Existing members need no invitation.
	dup := base
	dup.Email, dup.InviterRoleKey = x.owner.Email, "owner"
	if _, err := x.svc.CreateInvitation(ctx, dup); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("existing member err = %v", err)
	}
	if len(x.notif.sent) != 0 {
		t.Fatal("no email may be sent for rejected invitations")
	}
}

func TestAcceptInvitation(t *testing.T) {
	t.Parallel()
	x := newInvFixture(t)
	ctx := context.Background()
	invitee := db.User{ID: 20, Uuid: uuid.New(), Email: "c@example.test", Name: "C", Surname: "D", Status: "active"}
	other := db.User{ID: 21, Uuid: uuid.New(), Email: "other@example.test", Name: "O", Surname: "T", Status: "active"}
	x.f.users[invitee.ID], x.f.users[other.ID] = invitee, other
	_, token := x.invite(t, "C@example.test", "admin", "owner")

	if _, err := x.svc.AcceptInvitation(ctx, token, other.ID); !errors.Is(err, ErrInvitationEmailMismatch) {
		t.Fatalf("wrong account err = %v", err)
	}
	if _, err := x.svc.AcceptInvitation(ctx, "nope", invitee.ID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("unknown token err = %v", err)
	}
	inv := &fakeInvalidator{}
	x.svc.SetInvalidator(inv)
	got, err := x.svc.AcceptInvitation(ctx, token, invitee.ID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got.OrganizationSlug != "acme" || got.Role.Key != "admin" {
		t.Fatalf("accepted = %+v", got)
	}
	if role := x.f.memberships[x.org.ID][invitee.ID]; role != "admin" {
		t.Fatalf("membership role = %q", role)
	}
	if len(inv.ids) == 0 {
		t.Fatal("cache must be invalidated after accepting")
	}
	// Single use.
	if _, err := x.svc.AcceptInvitation(ctx, token, invitee.ID); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("second accept err = %v", err)
	}
}

func TestAcceptExpiredOrRevokedOrMember(t *testing.T) {
	t.Parallel()
	x := newInvFixture(t)
	ctx := context.Background()
	invitee := db.User{ID: 20, Uuid: uuid.New(), Email: "e@example.test", Name: "E", Surname: "F", Status: "active"}
	x.f.users[invitee.ID] = invitee

	inv, token := x.invite(t, "e@example.test", "member", "owner")
	if err := x.svc.RevokeInvitation(ctx, x.org.ID, inv.UUID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := x.svc.AcceptInvitation(ctx, token, invitee.ID); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("revoked accept err = %v", err)
	}
	if err := x.svc.RevokeInvitation(ctx, x.org.ID, inv.UUID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("double revoke err = %v", err)
	}

	_, token = x.invite(t, "e@example.test", "member", "owner")
	x.f.now = x.f.now.Add(8 * 24 * time.Hour)
	if prev, _ := x.svc.PreviewInvitation(ctx, token); prev.Status != orgmodel.InvitationExpired {
		t.Fatalf("status = %q, want expired", prev.Status)
	}
	if _, err := x.svc.AcceptInvitation(ctx, token, invitee.ID); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("expired accept err = %v", err)
	}

	x.f.now = x.f.now.Add(-8 * 24 * time.Hour)
	_, token = x.invite(t, "e@example.test", "admin", "owner")
	x.f.addMembership(x.org.ID, invitee.ID, "member")
	if _, err := x.svc.AcceptInvitation(ctx, token, invitee.ID); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("member accept err = %v", err)
	}
	if prev, _ := x.svc.PreviewInvitation(ctx, token); prev.Status != orgmodel.InvitationPending {
		t.Fatal("a refused accept must not consume the invitation")
	}
}

func TestCheckRoleChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		actor, from, to string
		wantErr         bool
	}{
		{"owner", "member", "owner", false},
		{"owner", "owner", "member", false},
		{"admin", "member", "admin", false},
		{"admin", "admin", "member", false},
		{"admin", "member", "owner", true},
		{"admin", "owner", "admin", true},
		{"admin", "owner", "", true},         // removing an owner
		{"member", "member", "admin", false}, // permission middleware decides this
	}
	for _, c := range cases {
		err := CheckRoleChange(c.actor, c.from, c.to)
		if (err != nil) != c.wantErr {
			t.Errorf("CheckRoleChange(%q,%q,%q) = %v, wantErr %v", c.actor, c.from, c.to, err, c.wantErr)
		}
	}
}

func TestInvitationsDisabledWithoutWiring(t *testing.T) {
	t.Parallel()
	svc := New(newFakeRepo(time.Now()), nil)
	if _, err := svc.PreviewInvitation(context.Background(), "x"); !errors.Is(err, ErrInvitationsDisabled) {
		t.Fatalf("err = %v", err)
	}
}
