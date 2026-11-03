package tenant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/clientip"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// OrganizationService is the slice of the organizations usecase the tenant
// routes use (*orgusecase.Service implements it).
type OrganizationService interface {
	GetOrganization(ctx context.Context, id uuid.UUID) (orgmodel.Organization, error)
	PatchOrganization(ctx context.Context, id uuid.UUID, in orgmodel.PatchOrganizationInput) (orgmodel.Organization, error)
	ListMembers(ctx context.Context, orgUUID uuid.UUID, f orgmodel.MemberListFilter) ([]orgmodel.Member, int64, error)
	GetMember(ctx context.Context, orgID int64, userUUID uuid.UUID) (orgmodel.Member, error)
	ChangeMemberRole(ctx context.Context, orgUUID, userUUID uuid.UUID, roleKey, actorRole string) (orgmodel.Member, error)
	RemoveMember(ctx context.Context, orgUUID, userUUID uuid.UUID, actorRole string) error
	ListInvitations(ctx context.Context, orgID int64, limit, offset int32) ([]orgmodel.Invitation, int64, error)
	CreateInvitation(ctx context.Context, in orgmodel.CreateInvitationInput) (orgmodel.Invitation, error)
	RevokeInvitation(ctx context.Context, orgID int64, id uuid.UUID, actorRole string) error
	PreviewInvitation(ctx context.Context, token string) (orgmodel.InvitationPreview, error)
	AcceptInvitation(ctx context.Context, token string, userID int64) (orgmodel.AcceptedInvitation, error)
}

// Rate limits (per window) for the invitation endpoints.
const (
	limitWindow          = 15 * time.Minute
	limitPreviewPerIP    = 60
	limitAcceptPerIP     = 20
	limitInvitesPerOrg   = 50 // emails one organization may trigger per window
	maxSettingsJSONBytes = 16 << 10
)

// Handler serves the organization-scoped tenant routes.
type Handler struct {
	svc      OrganizationService
	activity *activity.Recorder
	limiter  *ratelimit.Limiter
}

// NewHandler builds the tenant handler. activity and limiter may be nil.
func NewHandler(svc OrganizationService, rec *activity.Recorder, limiter *ratelimit.Limiter) *Handler {
	return &Handler{svc: svc, activity: rec, limiter: limiter}
}

// --- Members ---

type memberPage struct {
	Items  []orgmodel.Member `json:"items"`
	Total  int64             `json:"total"`
	Limit  int32             `json:"limit"`
	Offset int32             `json:"offset"`
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	limit, offset := pageParams(r)
	items, total, err := h.svc.ListMembers(r.Context(), s.UUID, orgmodel.MemberListFilter{
		Limit: limit, Offset: offset, Q: strings.TrimSpace(r.URL.Query().Get("q")),
		SortField: "created_at", SortDesc: false,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if items == nil {
		items = []orgmodel.Member{}
	}
	response.JSON(w, r, http.StatusOK, memberPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) changeMemberRole(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	userUUID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	var in struct {
		RoleKey string `json:"role_key"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	target, err := h.svc.GetMember(r.Context(), s.ID, userUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := orgusecase.CheckRoleChange(s.MemberRole, target.Role.Key, strings.ToLower(strings.TrimSpace(in.RoleKey))); err != nil {
		writeError(w, r, err)
		return
	}
	member, err := h.svc.ChangeMemberRole(r.Context(), s.UUID, userUUID, in.RoleKey, s.MemberRole)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.member_role_updated", map[string]any{
		"user_uuid": userUUID.String(), "from": target.Role.Key, "to": member.Role.Key,
	})
	response.JSON(w, r, http.StatusOK, member)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	userUUID, ok := pathUUID(w, r, "userUuid")
	if !ok {
		return
	}
	target, err := h.svc.GetMember(r.Context(), s.ID, userUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// Removing an owner is an owner-only action.
	if err := orgusecase.CheckRoleChange(s.MemberRole, target.Role.Key, ""); err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.svc.RemoveMember(r.Context(), s.UUID, userUUID, s.MemberRole); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.member_removed", map[string]any{"user_uuid": userUUID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "removed"})
}

// leave removes the caller's own membership (any role; the last owner is
// refused by the usecase).
func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "Cannot leave an organization while impersonating")
		return
	}
	if err := h.svc.RemoveMember(r.Context(), s.UUID, p.UserID, s.MemberRole); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.member_left", nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "left"})
}

// --- Invitations (organization side) ---

type invitationPage struct {
	Items  []orgmodel.Invitation `json:"items"`
	Total  int64                 `json:"total"`
	Limit  int32                 `json:"limit"`
	Offset int32                 `json:"offset"`
}

func (h *Handler) listInvitations(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	limit, offset := pageParams(r)
	items, total, err := h.svc.ListInvitations(r.Context(), s.ID, limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if items == nil {
		items = []orgmodel.Invitation{}
	}
	response.JSON(w, r, http.StatusOK, invitationPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Email   string `json:"email"`
		RoleKey string `json:"role_key"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	if !h.allow(w, r, "tenant_invite_org", strconv.FormatInt(s.ID, 10), limitInvitesPerOrg) {
		return
	}
	inv, err := h.svc.CreateInvitation(r.Context(), orgmodel.CreateInvitationInput{
		OrgID:          s.ID,
		OrgName:        s.Name,
		InviterUserID:  p.UserInternal,
		InviterRoleKey: s.MemberRole,
		Email:          in.Email,
		RoleKey:        in.RoleKey,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.invitation_created", map[string]any{
		"invitation_uuid": inv.UUID.String(), "role": inv.Role.Key,
	})
	response.JSON(w, r, http.StatusCreated, inv)
}

func (h *Handler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.RevokeInvitation(r.Context(), s.ID, id, s.MemberRole); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.invitation_revoked", map[string]any{"invitation_uuid": id.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "revoked"})
}

// --- Invitations (link holder side; no organization header) ---

type tokenBody struct {
	Token string `json:"token"`
}

// previewInvitation is public: the token itself is the credential, and it
// is sent in the body so it never lands in access logs.
func (h *Handler) previewInvitation(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "invitation_preview_ip", clientip.From(r), limitPreviewPerIP) {
		return
	}
	var in tokenBody
	if !decode(w, r, &in, 1<<10) {
		return
	}
	out, err := h.svc.PreviewInvitation(r.Context(), in.Token)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "Invitations cannot be accepted while impersonating")
		return
	}
	if !h.allow(w, r, "invitation_accept_ip", clientip.From(r), limitAcceptPerIP) {
		return
	}
	var in tokenBody
	if !decode(w, r, &in, 1<<10) {
		return
	}
	out, err := h.svc.AcceptInvitation(r.Context(), in.Token, p.UserInternal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &p.UserInternal, "organizations.invitation_accepted",
			"platform.organizations", nil, map[string]any{"organization_slug": out.OrganizationSlug}, r)
	}
	response.JSON(w, r, http.StatusOK, out)
}

// --- Settings ---

type settingsDTO struct {
	UUID     string          `json:"uuid"`
	Slug     string          `json:"slug"`
	Name     string          `json:"name"`
	LogoURL  *string         `json:"logo_url,omitempty"`
	Settings json.RawMessage `json:"settings"`
}

func toSettingsDTO(org orgmodel.Organization) settingsDTO {
	settings := json.RawMessage(org.Settings)
	if len(bytes.TrimSpace(settings)) == 0 {
		settings = json.RawMessage(`{}`)
	}
	return settingsDTO{
		UUID: org.UUID.String(), Slug: org.Slug, Name: org.Name,
		LogoURL: org.LogoURL, Settings: settings,
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	org, err := h.svc.GetOrganization(r.Context(), s.UUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, toSettingsDTO(org))
}

// patchSettings updates the name and/or the app-defined settings object.
// The slug is platform-managed (it is part of every /t/{slug} link).
func (h *Handler) patchSettings(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	var in struct {
		Name     *string         `json:"name"`
		Settings json.RawMessage `json:"settings"`
	}
	if !decode(w, r, &in, maxSettingsJSONBytes+1<<10) {
		return
	}
	patch := orgmodel.PatchOrganizationInput{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 255 {
			response.BadRequest(w, r, response.CodeValidationError, "name must be 1-255 characters")
			return
		}
		patch.Name = &name
	}
	if len(in.Settings) > 0 {
		trimmed := bytes.TrimSpace(in.Settings)
		if len(trimmed) > maxSettingsJSONBytes || len(trimmed) == 0 || trimmed[0] != '{' {
			response.BadRequest(w, r, response.CodeValidationError, "settings must be a JSON object up to 16 KiB")
			return
		}
		patch.Settings = trimmed
	}
	if patch.Name == nil && patch.Settings == nil {
		response.BadRequest(w, r, response.CodeValidationError, "nothing to update")
		return
	}
	org, err := h.svc.PatchOrganization(r.Context(), s.UUID, patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.updated", map[string]any{"source": "tenant_settings"})
	response.JSON(w, r, http.StatusOK, toSettingsDTO(org))
}

// --- helpers ---

func (h *Handler) record(r *http.Request, s orgctx.Scope, action string, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id := s.UUID
	h.activity.Record(r.Context(), &p.UserInternal, action, "platform.organizations", &id, payload, r)
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	if h.limiter == nil || strings.TrimSpace(subject) == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, limitWindow)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "")
	return false
}

func pageParams(r *http.Request) (int32, int32) {
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = int32(v)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = int32(v)
	}
	return limit, offset
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any, max int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, orgusecase.ErrInvitationNotFound), errors.Is(err, orgusecase.ErrInvitationsDisabled):
		response.Error(w, r, http.StatusNotFound, response.CodeInvitationNotFound, "Invitation was not found")
	case errors.Is(err, orgusecase.ErrInvitationUnavailable):
		response.Error(w, r, http.StatusGone, response.CodeInvitationUnavailable, "Invitation was already used, revoked or has expired")
	case errors.Is(err, orgusecase.ErrInvitationEmailMismatch):
		response.Error(w, r, http.StatusForbidden, response.CodeInvitationEmailMismatch, "Invitation was sent to another email address")
	case errors.Is(err, orgusecase.ErrAlreadyMember):
		response.Conflict(w, r, response.CodeAlreadyMember, "Already a member of this organization")
	case errors.Is(err, orgusecase.ErrOwnerRoleRequired):
		response.Error(w, r, http.StatusForbidden, response.CodeOwnerRoleRequired, "Only an owner can grant, change or remove the owner role")
	case errors.Is(err, orgusecase.ErrLastOrganizationOwner):
		response.Conflict(w, r, response.CodeLastOrganizationOwner, "Cannot remove or demote the last organization owner")
	case errors.Is(err, orgusecase.ErrNotFound), errors.Is(err, orgusecase.ErrMemberNotFound),
		errors.Is(err, orgusecase.ErrUserNotFound), errors.Is(err, orgusecase.ErrRoleNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, orgusecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, orgusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
