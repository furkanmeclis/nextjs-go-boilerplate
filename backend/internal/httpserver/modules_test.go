package httpserver

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"testing"

	apidocs "github.com/furkanmeclis/nextjs-go-boilerplate/backend/docs"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/module"
)

// undocumentedModules predate the OpenAPI-with-every-route rule: their routes
// are missing from openapi.yaml. Remove a module from this set once its paths
// are documented; never add one.
var undocumentedModules = map[string]bool{
	"activity": true,
	"logs":     true,
}

// TestModuleRoutesDocumented: every route a self-registering module mounts
// has the same path and method in backend/docs/openapi.yaml (hard rule).
func TestModuleRoutesDocumented(t *testing.T) {
	t.Parallel()
	r := module.NewRouter(module.Deps{}) // no mux: record only
	if err := r.Mount(selfRegisteredModules(moduleHandlers{})...); err != nil {
		t.Fatal(err)
	}
	spec := openAPIOperations(t)
	routes := r.Routes()
	if len(routes) == 0 {
		t.Fatal("no module routes recorded")
	}
	stillMissing := map[string]bool{}
	for _, rt := range routes {
		if !strings.HasPrefix(rt.Path, "/v1/") || undocumentedModules[rt.Module] {
			continue
		}
		if !spec[strings.ToLower(rt.Method)+" "+rt.Path] {
			t.Errorf("%s: %s is not in backend/docs/openapi.yaml", rt.Module, rt.Pattern)
		}
	}
	// Keep the allowlist honest: a module whose routes are all documented
	// must leave it.
	for _, rt := range routes {
		if undocumentedModules[rt.Module] && !spec[strings.ToLower(rt.Method)+" "+rt.Path] {
			stillMissing[rt.Module] = true
		}
	}
	for name := range undocumentedModules {
		if !stillMissing[name] {
			t.Errorf("module %q is fully documented now: remove it from undocumentedModules", name)
		}
	}
}

// openAPIOperations returns "method /path" for every operation in the spec.
func openAPIOperations(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := apidocs.FS.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pathRe := regexp.MustCompile(`^  (/[^:\s]+):\s*$`)
	opRe := regexp.MustCompile(`^    (get|post|put|patch|delete|head|options):\s*$`)
	out := map[string]bool{}
	var current string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if m := pathRe.FindStringSubmatch(line); m != nil {
			current = strings.Trim(m[1], `"'`)
			continue
		}
		if current != "" {
			if m := opRe.FindStringSubmatch(line); m != nil {
				out[m[1]+" "+current] = true
			} else if !strings.HasPrefix(line, " ") && strings.TrimSpace(line) != "" {
				current = "" // left the paths block
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no operations parsed from openapi.yaml")
	}
	return out
}
