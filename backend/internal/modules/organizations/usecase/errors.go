package usecase

import "errors"

var (
	ErrNotFound              = errors.New("organization not found")
	ErrMemberNotFound        = errors.New("organization member not found")
	ErrUserNotFound          = errors.New("user not found")
	ErrRoleNotFound          = errors.New("tenant role not found")
	ErrInvalidRequest        = errors.New("invalid request")
	ErrConflict              = errors.New("conflict")
	ErrLastOrganizationOwner = errors.New("last organization owner")

	// ErrOwnerRoleRequired: only owners may grant, revoke or remove the
	// owner role (or invite as owner).
	ErrOwnerRoleRequired = errors.New("owner role required")
	// ErrAlreadyMember: the invited address already belongs to a member.
	ErrAlreadyMember = errors.New("already a member")
	// ErrInvitationNotFound: unknown token, or the organization is gone.
	ErrInvitationNotFound = errors.New("invitation not found")
	// ErrInvitationUnavailable: accepted, revoked or expired.
	ErrInvitationUnavailable = errors.New("invitation is no longer valid")
	// ErrInvitationEmailMismatch: signed in as another address.
	ErrInvitationEmailMismatch = errors.New("invitation email mismatch")
	// ErrInvitationsDisabled: no invitation repository/notifier wired.
	ErrInvitationsDisabled = errors.New("invitations are not configured")
)
