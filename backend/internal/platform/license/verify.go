package license

import (
	"crypto/ed25519"
	"errors"
	"time"
)

// KeySet maps a key_id to the Ed25519 public key that verifies tokens signed
// by it. Holding a set (rather than one key) lets an old key keep verifying
// during rotation: a token names its signer in the key_id claim.
type KeySet map[string]ed25519.PublicKey

// Verifier checks tokens against a fixed app identity and key set. It is pure
// and safe for concurrent use; the only non-determinism (expiry) comes from
// the injected clock.
type Verifier struct {
	appID string
	keys  KeySet
	now   func() time.Time
}

// NewVerifier builds a Verifier for an app identity and key set. A nil or
// empty key set is allowed: every token then fails with ReasonUnknownKey,
// which is the correct behaviour for a build with no license keys embedded.
func NewVerifier(appID string, keys KeySet) *Verifier {
	return &Verifier{appID: appID, keys: keys, now: time.Now}
}

// withClock returns a copy of the verifier using a fixed clock (for tests).
func (v *Verifier) withClock(now func() time.Time) *Verifier {
	cp := *v
	cp.now = now
	return &cp
}

// Verify parses and checks a token, returning the resulting State. It never
// returns an error: an invalid token is reported as a State with Valid=false
// and a classifying Reason, so callers can log or gate uniformly.
func (v *Verifier) Verify(token string) State {
	base := State{AppID: v.appID}

	parsed, err := parseToken(token)
	if err != nil {
		return refuse(base, err)
	}
	c := parsed.claims
	base.KeyID = c.KeyID
	base.Deployment = c.Deployment
	base.ExpiresAt = c.expiry()

	pub, ok := v.keys[c.KeyID]
	if !ok || len(pub) == 0 {
		base.Reason = ReasonUnknownKey
		return base
	}
	if !ed25519.Verify(pub, signingInput(parsed.claimsB64), parsed.signature) {
		base.Reason = ReasonBadSignature
		return base
	}
	if c.App != v.appID {
		base.Reason = ReasonWrongApp
		return base
	}
	if exp := c.expiry(); exp != nil && !v.now().Before(*exp) {
		base.Reason = ReasonExpired
		return base
	}

	base.Valid = true
	base.Reason = ReasonOK
	base.Features = c.Features
	return base
}

// refuse maps a parse/verify error (which carries a Reason) onto the base
// State.
func refuse(base State, err error) State {
	var re reasonError
	if errors.As(err, &re) {
		base.Reason = re.reason
	} else {
		base.Reason = ReasonMalformed
	}
	return base
}
