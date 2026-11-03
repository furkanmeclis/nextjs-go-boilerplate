// Package tenant holds /v1/tenant routes: requests scoped to the organization
// named by the X-Organization header (see docs/design/tenant-core.md), plus
// the invitation-link routes a future member uses before they belong to it.
package tenant

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the tenant routes. A nil resolver (tenancy off)
// registers nothing. A nil handler mounts only /v1/tenant/context.
func RegisterRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
	h *Handler,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission

	mux.Handle("GET /v1/tenant/context", middleware.Chain(
		http.HandlerFunc(getContext), authn, org,
	))
	if h == nil {
		return
	}

	// Members of the current organization.
	mux.Handle("GET /v1/tenant/members", middleware.Chain(
		http.HandlerFunc(h.listMembers), authn, org, perm(rbac.PermTenantMembersRead),
	))
	mux.Handle("PATCH /v1/tenant/members/{userUuid}", middleware.Chain(
		http.HandlerFunc(h.changeMemberRole), authn, org, perm(rbac.PermTenantMembersWrite),
	))
	mux.Handle("DELETE /v1/tenant/members/{userUuid}", middleware.Chain(
		http.HandlerFunc(h.removeMember), authn, org, perm(rbac.PermTenantMembersWrite),
	))
	// Any member may leave (the last owner is refused).
	mux.Handle("POST /v1/tenant/leave", middleware.Chain(
		http.HandlerFunc(h.leave), authn, org,
	))

	// Invitations sent by the current organization.
	mux.Handle("GET /v1/tenant/invitations", middleware.Chain(
		http.HandlerFunc(h.listInvitations), authn, org, perm(rbac.PermTenantMembersRead),
	))
	mux.Handle("POST /v1/tenant/invitations", middleware.Chain(
		http.HandlerFunc(h.createInvitation), authn, org, perm(rbac.PermTenantMembersWrite),
	))
	mux.Handle("DELETE /v1/tenant/invitations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.revokeInvitation), authn, org, perm(rbac.PermTenantMembersWrite),
	))

	// Organization settings.
	mux.Handle("GET /v1/tenant/settings", middleware.Chain(
		http.HandlerFunc(h.getSettings), authn, org, perm(rbac.PermTenantSettingsRead),
	))
	mux.Handle("PATCH /v1/tenant/settings", middleware.Chain(
		http.HandlerFunc(h.patchSettings), authn, org, perm(rbac.PermTenantSettingsWrite),
	))

	// Invitation links: no organization header (the holder is not a member
	// yet). Preview is public; accepting needs a session.
	mux.Handle("POST /v1/invitations/preview", http.HandlerFunc(h.previewInvitation))
	mux.Handle("POST /v1/invitations/accept", middleware.Chain(
		http.HandlerFunc(h.acceptInvitation), authn,
	))
}
