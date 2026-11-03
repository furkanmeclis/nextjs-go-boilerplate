package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive    = "active"
	StatusSuspended = "suspended"

	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type Organization struct {
	ID             int64      `json:"-"`
	UUID           uuid.UUID  `json:"uuid"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	LogoURL        *string    `json:"logo_url,omitempty"`
	Status         string     `json:"status"`
	AccessStartsAt time.Time  `json:"access_starts_at"`
	AccessEndsAt   *time.Time `json:"access_ends_at,omitempty"`
	Settings       []byte     `json:"settings,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type User struct {
	ID      int64
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Status  string    `json:"status"`
}

type RoleSummary struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Member struct {
	ID        int64       `json:"-"`
	User      User        `json:"user"`
	Role      RoleSummary `json:"role"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type Membership struct {
	Organization Organization `json:"organization"`
	Role         RoleSummary  `json:"role"`
	Permissions  []string     `json:"permissions"`
}

type CreateOrganizationInput struct {
	Name string
	Slug string
	// OwnerUserUUID selects the owner; when zero, OwnerUserID is used.
	OwnerUserUUID uuid.UUID
	OwnerUserID   int64
}

type PatchOrganizationInput struct {
	Name     *string
	Slug     *string
	Settings []byte
}

type ListFilter struct {
	Limit     int32
	Offset    int32
	Q         string
	Status    string
	SortField string
	SortDesc  bool
}

type MemberListFilter struct {
	Limit     int32
	Offset    int32
	Q         string
	SortField string
	SortDesc  bool
}

// Invitation states as seen by the API (derived, not stored).
const (
	InvitationPending  = "pending"
	InvitationExpired  = "expired"
	InvitationAccepted = "accepted"
	InvitationRevoked  = "revoked"
)

// Invitation is an open invitation listed for an organization's admins.
type Invitation struct {
	UUID      uuid.UUID   `json:"uuid"`
	Email     string      `json:"email"`
	Role      RoleSummary `json:"role"`
	Status    string      `json:"status"`
	ExpiresAt time.Time   `json:"expires_at"`
	CreatedAt time.Time   `json:"created_at"`
	InvitedBy *string     `json:"invited_by,omitempty"`
}

// InvitationPreview is what the holder of an invitation link may see.
type InvitationPreview struct {
	Organization struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"organization"`
	Email     string      `json:"email"`
	Role      RoleSummary `json:"role"`
	Status    string      `json:"status"`
	ExpiresAt time.Time   `json:"expires_at"`
	InvitedBy *string     `json:"invited_by,omitempty"`
}

type CreateInvitationInput struct {
	OrgID          int64
	OrgName        string
	InviterUserID  int64
	InviterName    string
	InviterRoleKey string
	Email          string
	RoleKey        string
}

// AcceptedInvitation tells the caller where the new membership lives.
type AcceptedInvitation struct {
	OrganizationSlug string      `json:"organization_slug"`
	Role             RoleSummary `json:"role"`
}
