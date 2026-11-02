package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"embed"
	"encoding/pem"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
)

// embeddedKeys holds the author's license public keys, compiled into the
// binary. Each file is a PKIX/PEM Ed25519 public key named "<key_id>.pub.pem";
// the key_id is a token's claim selecting which key verifies it.
//
// The ".pub.pem" convention is load-bearing: the embed glob matches only
// public keys, so a private key ("<key_id>.key.pem") dropped into this
// directory can never be compiled into the binary even by mistake.
//
// The committed key (keys/lic-dev-2026.pub.pem) is a BOOTSTRAP key for local
// and staging use. Before issuing production licenses, generate a real keypair
// with `license keygen`, drop the new public key here, and keep the private
// key offline. See docs/design/license-core.md §8.
//
//go:embed keys/*.pub.pem
var embeddedKeys embed.FS

// pubKeySuffix is the required file name suffix for an embedded public key.
const pubKeySuffix = ".pub.pem"

var (
	embeddedOnce sync.Once
	embeddedSet  KeySet
	embeddedErr  error
)

// EmbeddedKeys parses and caches the public keys embedded at build time. A
// parse failure is an author/build error, not a runtime condition, so it is
// returned for the caller to surface loudly rather than panicking.
func EmbeddedKeys() (KeySet, error) {
	embeddedOnce.Do(func() {
		embeddedSet, embeddedErr = parseKeyFS(embeddedKeys, "keys")
	})
	return embeddedSet, embeddedErr
}

// parseKeyFS reads every *.pem under dir into a KeySet keyed by base name.
func parseKeyFS(fsys fs.FS, dir string) (KeySet, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("license: read embedded keys: %w", err)
	}
	set := make(KeySet)
	for _, e := range entries {
		// Only public keys are read, so a private key that somehow lands here
		// is never parsed into the trusted set (belt-and-braces with the embed
		// glob, which already only matches *.pub.pem).
		if e.IsDir() || !strings.HasSuffix(e.Name(), pubKeySuffix) {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("license: read %s: %w", e.Name(), err)
		}
		pub, err := parsePublicKeyPEM(raw)
		if err != nil {
			return nil, fmt.Errorf("license: parse %s: %w", e.Name(), err)
		}
		keyID := strings.TrimSuffix(e.Name(), pubKeySuffix)
		set[keyID] = pub
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("license: no public keys found in %s", dir)
	}
	return set, nil
}

// parsePublicKeyPEM decodes a single PKIX/PEM Ed25519 public key.
func parsePublicKeyPEM(raw []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 public key (%T)", parsed)
	}
	return pub, nil
}
