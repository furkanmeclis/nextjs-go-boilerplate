// Package buildinfo exposes the release provenance baked into the binary.
//
// CI (publish-images / image-build) passes VERSION, GIT_SHA and BUILD_TIME as
// Docker build args; the Dockerfile turns them into
//
//	-ldflags "-X <module>/internal/buildinfo.Version=v0.N
//	          -X <module>/internal/buildinfo.GitSHA=<sha>
//	          -X <module>/internal/buildinfo.BuildTime=<RFC3339>"
//
// and overwrites release.json (version, git_sha, built_at, notes — notes are
// auto-generated from `git log` since the previous v0.* tag) before `go build`,
// so it is embedded below. The committed release.json is a `{}` placeholder:
// local / dev builds still compile and report Version "dev".
package buildinfo

import (
	_ "embed"
	"encoding/json"
	"strings"
	"time"
)

// Set via -ldflags -X. Empty in local builds.
var (
	Version   string
	GitSHA    string
	BuildTime string
)

//go:embed release.json
var releaseJSON []byte

// DevVersion is reported when nothing was baked in.
const DevVersion = "dev"

// Release is the provenance of the running binary.
type Release struct {
	Version string `json:"version"`
	GitSHA  string `json:"git_sha"`
	BuiltAt string `json:"built_at"`
	Notes   string `json:"notes"`
}

// Current merges the embedded release.json with the ldflags values (ldflags
// win for version / sha / build time; notes only exist in release.json).
// It never fails: a missing or malformed file yields what ldflags provide,
// and Version falls back to "dev".
func Current() Release {
	return merge(releaseJSON, Version, GitSHA, BuildTime)
}

func merge(raw []byte, version, sha, builtAt string) Release {
	var r Release
	_ = json.Unmarshal(raw, &r) // fail-soft: placeholder `{}` or garbage -> zero value
	if v := strings.TrimSpace(version); v != "" {
		r.Version = v
	}
	if s := strings.TrimSpace(sha); s != "" {
		r.GitSHA = s
	}
	if b := strings.TrimSpace(builtAt); b != "" {
		r.BuiltAt = b
	}
	r.Version = strings.TrimSpace(r.Version)
	if r.Version == "" {
		r.Version = DevVersion
	}
	return r
}

// IsDev reports whether no version was baked into this binary.
func (r Release) IsDev() bool { return r.Version == DevVersion }

// BuiltAtTime parses BuiltAt (RFC 3339). ok is false when it is empty or invalid.
func (r Release) BuiltAtTime() (t time.Time, ok bool) {
	if r.BuiltAt == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, r.BuiltAt)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
