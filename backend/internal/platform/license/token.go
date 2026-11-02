// Package license verifies an offline, author-signed deployment license.
//
// The running app never calls out to check a license: a signed token is read
// from the LICENSE_KEY environment variable and its Ed25519 signature is
// verified against a public key embedded at build time (keys/*.pem). The
// private half lives only on the author's machine and signs tokens with the
// license CLI (cmd/license). See docs/design/license-core.md.
//
// This package is pure: no network, no database, no clock except the one the
// caller injects. Enforcement (gating traffic, the suspend switch, the
// outbound control channel) is layered on top in later phases; phase 1 only
// verifies and reports State.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// tokenPrefix is the versioned, domain-separated header of a license token.
// It is part of the signed bytes, so a signature produced for a license can
// never be replayed as some other Ed25519 message (e.g. a future control
// command) and vice versa.
const tokenPrefix = "lic.v1"

// Claims are the fields carried by a license token. They are signed as-is;
// the verifier re-derives nothing from them beyond what is written here.
type Claims struct {
	// App is the application identifier this key is valid for. It must equal
	// the deployment's APP_NAME for the license to be accepted.
	App string `json:"app"`
	// Deployment is an optional human label shown in health reporting.
	Deployment string `json:"deployment,omitempty"`
	// IssuedAt is the unix second the token was signed.
	IssuedAt int64 `json:"iat"`
	// ExpiresAt is the unix second the token expires. Zero means no expiry.
	ExpiresAt int64 `json:"exp,omitempty"`
	// Features are optional per-deployment feature gates.
	Features []string `json:"features,omitempty"`
	// KeyID names the signing key, so the verifier can select the matching
	// public key from its embedded set and keys can be rotated.
	KeyID string `json:"key_id"`
}

// signingInput returns the bytes that are signed and verified for a token
// whose claims segment is claimsB64: "lic.v1.<claimsB64>".
func signingInput(claimsB64 string) []byte {
	return []byte(tokenPrefix + "." + claimsB64)
}

// Sign produces a license token string from claims using an Ed25519 private
// key. It is used author-side by cmd/license and by tests; it never touches
// the embedded public key.
//
// The returned token is "lic.v1.<base64url(claims)>.<base64url(signature)>".
func Sign(priv ed25519.PrivateKey, c Claims) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("license: invalid private key size %d", len(priv))
	}
	if strings.TrimSpace(c.App) == "" {
		return "", errors.New("license: app is required")
	}
	if strings.TrimSpace(c.KeyID) == "" {
		return "", errors.New("license: key_id is required")
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("license: marshal claims: %w", err)
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(priv, signingInput(claimsB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return tokenPrefix + "." + claimsB64 + "." + sigB64, nil
}

// parsedToken is the decoded-but-not-yet-verified content of a token.
type parsedToken struct {
	claims    Claims
	claimsB64 string
	signature []byte
}

// parseToken splits and decodes a token without checking its signature.
func parseToken(token string) (parsedToken, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 4 || parts[0] != "lic" || parts[1] != "v1" {
		return parsedToken{}, errReasonf(ReasonMalformed, "unexpected token format")
	}
	claimsB64, sigB64 := parts[2], parts[3]

	payload, err := base64.RawURLEncoding.DecodeString(claimsB64)
	if err != nil {
		return parsedToken{}, errReasonf(ReasonMalformed, "decode claims: %v", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return parsedToken{}, errReasonf(ReasonMalformed, "decode signature: %v", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return parsedToken{}, errReasonf(ReasonMalformed, "signature size %d", len(sig))
	}

	var c Claims
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return parsedToken{}, errReasonf(ReasonMalformed, "decode claims json: %v", err)
	}
	return parsedToken{claims: c, claimsB64: claimsB64, signature: sig}, nil
}

// expiry returns the claim's expiry as a time, or nil when it never expires.
func (c Claims) expiry() *time.Time {
	if c.ExpiresAt == 0 {
		return nil
	}
	t := time.Unix(c.ExpiresAt, 0).UTC()
	return &t
}
