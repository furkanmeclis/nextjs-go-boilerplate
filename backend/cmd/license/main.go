// Command license is the author-side tool for the license core. It is NOT
// shipped to deployments; it signs tokens and manages the signing keypair
// that lives only on the author's machine. See docs/design/license-core.md.
//
// Generate a signing keypair (writes <key_id>.pub.pem public + <key_id>.key.pem
// private; keep the private key offline, commit the public key into
// internal/platform/license/keys/):
//
//	go run ./cmd/license keygen --key-id lic-2026-01 --out ./secrets
//
// Sign a license token for a deployment:
//
//	go run ./cmd/license sign \
//	  --app my-app --deployment acme-prod \
//	  --features billing,ai --exp 2027-01-01 \
//	  --key-id lic-2026-01 --key ./secrets/lic-2026-01.key.pem
//
// The printed LICENSE_KEY goes into that deployment's .env.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/license"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		keygen(os.Args[2:])
	case "sign":
		sign(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "license: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `license — author-side license signing tool (not shipped to deployments)

Usage:
  license keygen --key-id <id> [--out <dir>]
  license sign   --app <id> --key-id <id> --key <privkey.pem> [options]

Run "license keygen -h" or "license sign -h" for command flags.
`)
}

// keygen generates an Ed25519 signing keypair.
func keygen(args []string) {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	keyID := fs.String("key-id", "", "key identifier (also the file base name and token key_id)")
	out := fs.String("out", ".", "directory to write the keypair into")
	_ = fs.Parse(args)

	if strings.TrimSpace(*keyID) == "" {
		fatal("keygen: --key-id is required")
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		fatal("keygen: create out dir: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal("keygen: generate key: %v", err)
	}

	pubPEM, err := marshalPublicPEM(pub)
	if err != nil {
		fatal("keygen: encode public key: %v", err)
	}
	privPEM, err := marshalPrivatePEM(priv)
	if err != nil {
		fatal("keygen: encode private key: %v", err)
	}

	pubPath := filepath.Join(*out, *keyID+".pub.pem")
	privPath := filepath.Join(*out, *keyID+".key.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		fatal("keygen: write public key: %v", err)
	}
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		fatal("keygen: write private key: %v", err)
	}

	fmt.Printf("wrote public key:  %s\n", pubPath)
	fmt.Printf("wrote private key: %s  (keep offline; never commit)\n", privPath)
	fmt.Printf("\nTo activate this key, commit the public key into:\n")
	fmt.Printf("  backend/internal/platform/license/keys/%s%s\n", *keyID, ".pub.pem")
}

// sign signs a license token.
func sign(args []string) {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	app := fs.String("app", "", "application id (must equal the deployment's APP_NAME)")
	deployment := fs.String("deployment", "", "optional human label for the deployment")
	features := fs.String("features", "", "optional comma-separated feature gates")
	exp := fs.String("exp", "", "optional expiry as YYYY-MM-DD or RFC3339 (empty = never)")
	keyID := fs.String("key-id", "", "signing key id (must match an embedded public key)")
	keyPath := fs.String("key", "", "path to the Ed25519 private key PEM")
	_ = fs.Parse(args)

	if strings.TrimSpace(*app) == "" {
		fatal("sign: --app is required")
	}
	if strings.TrimSpace(*keyID) == "" {
		fatal("sign: --key-id is required")
	}
	if strings.TrimSpace(*keyPath) == "" {
		fatal("sign: --key is required")
	}

	priv, err := loadPrivateKey(*keyPath)
	if err != nil {
		fatal("sign: %v", err)
	}

	claims := license.Claims{
		App:        strings.TrimSpace(*app),
		Deployment: strings.TrimSpace(*deployment),
		IssuedAt:   time.Now().UTC().Unix(),
		KeyID:      strings.TrimSpace(*keyID),
	}
	if f := splitCSV(*features); len(f) > 0 {
		claims.Features = f
	}
	if strings.TrimSpace(*exp) != "" {
		t, err := parseExpiry(*exp)
		if err != nil {
			fatal("sign: --exp: %v", err)
		}
		claims.ExpiresAt = t.Unix()
	}

	token, err := license.Sign(priv, claims)
	if err != nil {
		fatal("sign: %v", err)
	}

	// Sanity: verify what we just signed against the matching public key so a
	// bad key pairing fails here rather than silently in a deployment.
	pub := priv.Public().(ed25519.PublicKey)
	if s := license.NewVerifier(claims.App, license.KeySet{claims.KeyID: pub}).Verify(token); !s.Valid {
		fatal("sign: self-check failed (%s)", s.Reason)
	}

	fmt.Printf("LICENSE_KEY=%s\n", token)
}

func parseExpiry(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("want YYYY-MM-DD or RFC3339, got %q", s)
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 private key (%T)", parsed)
	}
	return priv, nil
}

func marshalPublicPEM(pub ed25519.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

func marshalPrivatePEM(priv ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func splitCSV(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
