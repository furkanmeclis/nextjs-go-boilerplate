// Package module is the self-registration contract for feature modules.
//
// A module describes its own routes through a Router instead of being wired
// line by line in internal/httpserver/server.go. The Router applies the
// standard auth chains (public, signed-in, platform permission, tenant
// permission) and records every route, so tests can check the route table
// against backend/docs/openapi.yaml. Design: docs/design/module-registry.md.
package module

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Module is a feature that mounts its own routes.
type Module interface {
	// Name is a unique, stable identifier ("activity", "logs").
	Name() string
	// Routes registers the module's HTTP routes.
	Routes(r *Router)
}

// Route is one registered pattern, kept for route-table tests.
type Route struct {
	Module  string
	Method  string
	Path    string
	Access  Access
	Pattern string
}

// Access names the guard chain a route was mounted with.
type Access string

const (
	AccessPublic   Access = "public"
	AccessSignedIn Access = "signed_in"
	AccessPlatform Access = "platform"
	AccessTenant   Access = "tenant"
)

// Deps are the shared pieces a Router needs to build guard chains.
type Deps struct {
	Mux    *http.ServeMux
	Tokens *jwt.Manager
	Loader middleware.IdentityLoader
	// Resolver resolves X-Organization; nil when tenancy is off, in which
	// case Tenant routes are not mounted at all.
	Resolver orgctx.Resolver
}

// Router mounts routes for one module at a time and records them.
type Router struct {
	deps   Deps
	authn  func(http.Handler) http.Handler
	module string
	routes []Route
	names  map[string]bool
}

// NewRouter builds a Router over the shared mux.
func NewRouter(d Deps) *Router {
	return &Router{
		deps:  d,
		authn: middleware.Authenticate(d.Tokens, d.Loader),
		names: map[string]bool{},
	}
}

// Mount registers modules in order. Names must be unique.
func (r *Router) Mount(mods ...Module) error {
	for _, m := range mods {
		name := m.Name()
		if name == "" {
			return fmt.Errorf("module: empty name (%T)", m)
		}
		if r.names[name] {
			return fmt.Errorf("module: %q registered twice", name)
		}
		r.names[name] = true
		r.module = name
		m.Routes(r)
	}
	r.module = ""
	return nil
}

// Public mounts a route without authentication.
func (r *Router) Public(pattern string, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	r.handle(pattern, AccessPublic, h, mw...)
}

// SignedIn mounts a route for any authenticated principal.
func (r *Router) SignedIn(pattern string, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	r.handle(pattern, AccessSignedIn, h, append([]func(http.Handler) http.Handler{r.authn}, mw...)...)
}

// Session mounts a route for any signed-in session holding auth.session
// (every normal account; revoked when the account is deactivated).
func (r *Router) Session(pattern string, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	chain := append([]func(http.Handler) http.Handler{r.authn, middleware.RequirePermission(rbac.PermAuthSession)}, mw...)
	r.handle(pattern, AccessSignedIn, h, chain...)
}

// Platform mounts a route that needs a platform permission slug.
func (r *Router) Platform(pattern, permission string, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	chain := append([]func(http.Handler) http.Handler{r.authn, middleware.RequirePermission(permission)}, mw...)
	r.handle(pattern, AccessPlatform, h, chain...)
}

// Tenant mounts an organization-scoped route (X-Organization + tenant
// permission of the membership). It is skipped when tenancy is off.
func (r *Router) Tenant(pattern, permission string, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	if r.deps.Resolver == nil {
		return
	}
	chain := append([]func(http.Handler) http.Handler{
		r.authn,
		middleware.RequireOrganization(r.deps.Resolver),
		middleware.RequireTenantPermission(permission),
	}, mw...)
	r.handle(pattern, AccessTenant, h, chain...)
}

// Routes returns the recorded routes sorted by path, then method.
func (r *Router) Routes() []Route {
	out := append([]Route(nil), r.routes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

func (r *Router) handle(pattern string, access Access, h http.HandlerFunc, mw ...func(http.Handler) http.Handler) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("module %s: pattern %q must be \"METHOD /path\"", r.module, pattern))
	}
	if r.deps.Mux != nil {
		r.deps.Mux.Handle(pattern, middleware.Chain(h, mw...))
	}
	r.routes = append(r.routes, Route{
		Module: r.module, Method: method, Path: path, Access: access, Pattern: pattern,
	})
}
