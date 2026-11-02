package license

import (
	"fmt"
	"log/slog"
	"time"
)

// Reason is a stable, machine-readable explanation of a license State. It is
// safe to log and to branch on; enforcement (later phases) maps a subset of
// these to a 503.
type Reason string

const (
	// ReasonOK means the token verified and is currently valid.
	ReasonOK Reason = "ok"
	// ReasonDevelopment means a synthetic license was granted because the
	// process runs with APP_ENV=development.
	ReasonDevelopment Reason = "development"
	// ReasonNoLicense means no LICENSE_KEY was configured.
	ReasonNoLicense Reason = "no_license"
	// ReasonMalformed means the token could not be parsed.
	ReasonMalformed Reason = "malformed"
	// ReasonUnknownKey means no embedded public key matches the token's key_id.
	ReasonUnknownKey Reason = "unknown_key"
	// ReasonBadSignature means the signature did not verify against the key.
	ReasonBadSignature Reason = "bad_signature"
	// ReasonWrongApp means the token's app claim does not match this app.
	ReasonWrongApp Reason = "wrong_app"
	// ReasonExpired means the token's exp claim is in the past.
	ReasonExpired Reason = "expired"
)

// State is the result of resolving a license. It is a value, safe to copy and
// to log (it carries no secrets — only the public token metadata).
type State struct {
	// Valid reports whether the license verified and is currently in force.
	Valid bool
	// Reason explains Valid (why granted, or why refused).
	Reason Reason
	// Development is true when Valid was granted by dev mode rather than a key.
	Development bool
	// AppID is the application this license was resolved against.
	AppID string
	// Deployment is the optional human label from the token.
	Deployment string
	// Features are the token's feature gates (nil in dev / when unlicensed).
	Features []string
	// KeyID is the signing key named by a parsed token, if any.
	KeyID string
	// ExpiresAt is the token's expiry, or nil when it never expires.
	ExpiresAt *time.Time
}

// HasFeature reports whether a feature gate is enabled. A development license
// enables every feature; otherwise the token's feature list is consulted.
func (s State) HasFeature(name string) bool {
	if s.Development {
		return true
	}
	for _, f := range s.Features {
		if f == name {
			return true
		}
	}
	return false
}

// LogValue renders the State for slog without leaking anything sensitive
// (the token itself and any private material are never part of a State).
func (s State) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.Bool("valid", s.Valid),
		slog.String("reason", string(s.Reason)),
		slog.String("app", s.AppID),
	}
	if s.Development {
		attrs = append(attrs, slog.Bool("development", true))
	}
	if s.Deployment != "" {
		attrs = append(attrs, slog.String("deployment", s.Deployment))
	}
	if s.KeyID != "" {
		attrs = append(attrs, slog.String("key_id", s.KeyID))
	}
	if len(s.Features) > 0 {
		attrs = append(attrs, slog.Any("features", s.Features))
	}
	if s.ExpiresAt != nil {
		attrs = append(attrs, slog.Time("expires_at", *s.ExpiresAt))
	}
	return slog.GroupValue(attrs...)
}

// reasonError carries a Reason alongside a human message, so parse/verify can
// fail with a classified reason that Verify turns into a State.
type reasonError struct {
	reason Reason
	msg    string
}

func (e reasonError) Error() string { return fmt.Sprintf("license: %s: %s", e.reason, e.msg) }

func errReasonf(r Reason, format string, args ...any) error {
	return reasonError{reason: r, msg: fmt.Sprintf(format, args...)}
}
