package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

const (
	testApp   = "otopoly"
	testKeyID = "lic-test"
)

// testVerifier builds a verifier with a freshly generated key, returning the
// private key so a test can sign tokens the verifier will accept, plus a
// fixed clock.
func testVerifier(t *testing.T, now time.Time) (*Verifier, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v := NewVerifier(testApp, KeySet{testKeyID: pub}).withClock(func() time.Time { return now })
	return v, priv
}

func mustSign(t *testing.T, priv ed25519.PrivateKey, c Claims) string {
	t.Helper()
	tok, err := Sign(priv, c)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

func TestVerifyValid(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	tok := mustSign(t, priv, Claims{
		App:        testApp,
		Deployment: "acme-prod",
		IssuedAt:   now.Unix(),
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
		Features:   []string{"billing", "ai"},
		KeyID:      testKeyID,
	})

	s := v.Verify(tok)
	if !s.Valid {
		t.Fatalf("expected valid, got reason=%s", s.Reason)
	}
	if s.Reason != ReasonOK {
		t.Errorf("reason = %s, want ok", s.Reason)
	}
	if s.AppID != testApp || s.Deployment != "acme-prod" || s.KeyID != testKeyID {
		t.Errorf("unexpected metadata: %+v", s)
	}
	if !s.HasFeature("billing") || s.HasFeature("nope") {
		t.Errorf("feature gating wrong: %+v", s.Features)
	}
	if s.ExpiresAt == nil || !s.ExpiresAt.Equal(time.Unix(now.Add(24*time.Hour).Unix(), 0).UTC()) {
		t.Errorf("expiry not surfaced: %v", s.ExpiresAt)
	}
}

func TestVerifyValidNoExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	tok := mustSign(t, priv, Claims{App: testApp, IssuedAt: now.Unix(), KeyID: testKeyID})

	s := v.Verify(tok)
	if !s.Valid {
		t.Fatalf("expected valid, got reason=%s", s.Reason)
	}
	if s.ExpiresAt != nil {
		t.Errorf("expected no expiry, got %v", s.ExpiresAt)
	}
}

func TestVerifyForgedSignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	tok := mustSign(t, priv, Claims{App: testApp, IssuedAt: now.Unix(), KeyID: testKeyID})

	// Flip a byte in the claims segment; the signature no longer matches.
	b := []byte(tok)
	// segment layout: lic.v1.<claims>.<sig> — mutate inside <claims>.
	b[8] ^= 0x01
	s := v.Verify(string(b))
	if s.Valid {
		t.Fatal("forged token accepted")
	}
	if s.Reason != ReasonBadSignature && s.Reason != ReasonMalformed {
		t.Errorf("reason = %s, want bad_signature/malformed", s.Reason)
	}
}

func TestVerifyForgedBySecondKey(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, _ := testVerifier(t, now)

	// Sign with a DIFFERENT private key but claim the embedded key_id: an
	// attacker who re-signs the token must still fail verification.
	_, attacker, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}
	tok := mustSign(t, attacker, Claims{App: testApp, IssuedAt: now.Unix(), KeyID: testKeyID})

	s := v.Verify(tok)
	if s.Valid {
		t.Fatal("token signed by foreign key accepted")
	}
	if s.Reason != ReasonBadSignature {
		t.Errorf("reason = %s, want bad_signature", s.Reason)
	}
}

func TestVerifyExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	tok := mustSign(t, priv, Claims{
		App:       testApp,
		IssuedAt:  now.Add(-48 * time.Hour).Unix(),
		ExpiresAt: now.Add(-time.Second).Unix(),
		KeyID:     testKeyID,
	})

	s := v.Verify(tok)
	if s.Valid {
		t.Fatal("expired token accepted")
	}
	if s.Reason != ReasonExpired {
		t.Errorf("reason = %s, want expired", s.Reason)
	}
	if s.ExpiresAt == nil {
		t.Error("expected expiry surfaced on expired token")
	}
}

func TestVerifyExpiryBoundaryExclusive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	// exp exactly == now: the instant of expiry is already expired.
	tok := mustSign(t, priv, Claims{App: testApp, IssuedAt: now.Add(-time.Hour).Unix(), ExpiresAt: now.Unix(), KeyID: testKeyID})
	if s := v.Verify(tok); s.Valid {
		t.Error("token at its exact expiry instant should be invalid")
	}
}

func TestVerifyWrongApp(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	tok := mustSign(t, priv, Claims{App: "someone-else", IssuedAt: now.Unix(), KeyID: testKeyID})

	s := v.Verify(tok)
	if s.Valid {
		t.Fatal("token for another app accepted")
	}
	if s.Reason != ReasonWrongApp {
		t.Errorf("reason = %s, want wrong_app", s.Reason)
	}
}

func TestVerifyUnknownKey(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, priv := testVerifier(t, now)
	tok := mustSign(t, priv, Claims{App: testApp, IssuedAt: now.Unix(), KeyID: "no-such-key"})

	s := v.Verify(tok)
	if s.Valid {
		t.Fatal("token naming an unknown key accepted")
	}
	if s.Reason != ReasonUnknownKey {
		t.Errorf("reason = %s, want unknown_key", s.Reason)
	}
}

func TestVerifyMalformed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v, _ := testVerifier(t, now)
	for _, tok := range []string{
		"",
		"garbage",
		"lic.v1.only-three",
		"lic.v2.aaaa.bbbb",
		"jwt.v1.aaaa.bbbb",
		"lic.v1.!!!.bbbb",
	} {
		s := v.Verify(tok)
		if s.Valid {
			t.Errorf("malformed token %q accepted", tok)
		}
		if s.Reason != ReasonMalformed {
			t.Errorf("token %q reason = %s, want malformed", tok, s.Reason)
		}
	}
}

func TestResolveDevelopment(t *testing.T) {
	// Dev mode grants a synthetic license with no key and enables all features.
	s := Resolve(Input{Env: "development", AppID: "anything", Token: "", Keys: nil})
	if !s.Valid || !s.Development || s.Reason != ReasonDevelopment {
		t.Fatalf("dev mode not synthetic-valid: %+v", s)
	}
	if !s.HasFeature("any-feature-at-all") {
		t.Error("dev license should enable every feature")
	}
}

func TestResolveNoLicenseOutsideDev(t *testing.T) {
	s := Resolve(Input{Env: "production", AppID: testApp, Token: "", Keys: nil})
	if s.Valid {
		t.Fatal("empty token should not be valid outside development")
	}
	if s.Reason != ReasonNoLicense {
		t.Errorf("reason = %s, want no_license", s.Reason)
	}
}

func TestResolveVerifiesWithKeys(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tok := mustSign(t, priv, Claims{App: testApp, IssuedAt: now.Unix(), KeyID: testKeyID})

	s := Resolve(Input{Env: "production", AppID: testApp, Token: tok, Keys: KeySet{testKeyID: pub}})
	if !s.Valid {
		t.Fatalf("expected valid, got reason=%s", s.Reason)
	}
}

func TestEmbeddedKeysLoad(t *testing.T) {
	// The build must embed at least one parseable public key, otherwise the
	// package cannot verify anything in production.
	keys, err := EmbeddedKeys()
	if err != nil {
		t.Fatalf("embedded keys failed to load: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("no embedded keys")
	}
	for id, pub := range keys {
		if len(pub) != ed25519.PublicKeySize {
			t.Errorf("key %q has wrong size %d", id, len(pub))
		}
	}
}

func TestSignRejectsBadInput(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Sign(priv, Claims{KeyID: testKeyID}); err == nil {
		t.Error("expected error for missing app")
	}
	if _, err := Sign(priv, Claims{App: testApp}); err == nil {
		t.Error("expected error for missing key_id")
	}
	if _, err := Sign(ed25519.PrivateKey{1, 2, 3}, Claims{App: testApp, KeyID: testKeyID}); err == nil {
		t.Error("expected error for bad key size")
	}
}
