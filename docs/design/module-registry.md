# Module self-registration — design note

Status: **step 1 implemented** (`internal/platform/module`; `access`, `search`,
`activity`, `logs` migrated). The remaining modules still use explicit
`RegisterRoutes(mux, h, tokens, loader, …)` calls in
`internal/httpserver/server.go` and move over one at a time.

## Problem

`server.go` wires every module by hand (~500 lines): each module re-builds the
same guard chains (`Authenticate` → `RequirePermission`), takes a different
argument list, and nothing checks that the routes it mounts are in
`backend/docs/openapi.yaml`. A scan while writing this note (each `"METHOD /v1/…"` pattern in
`internal/modules/**/routes.go` vs. the `paths:` keys of `openapi.yaml`) found
72 mounted paths missing from the spec, in auth, exports, imports, logs,
storage, activity and notifications.

## Contract

```go
type Module interface {
    Name() string            // unique, stable
    Routes(r *module.Router) // declares routes
}
```

`module.Router` holds the shared deps (mux, JWT manager, identity loader,
org resolver) and exposes one method per access level:

| Method | Chain |
|---|---|
| `Public(pattern, h)` | none |
| `SignedIn(pattern, h)` | `Authenticate` |
| `Session(pattern, h)` | `Authenticate` → `RequirePermission(auth.session)` |
| `Platform(pattern, perm, h)` | `Authenticate` → `RequirePermission(perm)` |
| `Tenant(pattern, perm, h)` | `Authenticate` → `RequireOrganization` → `RequireTenantPermission(perm)`; skipped when tenancy is off |

Extra middleware (step-up, rate limits) goes in the variadic tail. Every call
is recorded (`Router.Routes()`), so route-table tests do not need a running
server.

Modules keep building their own services; `server.go` constructs the handler
and passes it in the module struct (`activitymodule.Module{H: …}`), then calls
`modRouter.Mount(...)`. Construction stays explicit on purpose: dependency
order (notifications before invitations, the org resolver before tenant
routes) is easier to read in one function than through a DI container.

## Guard rail

`internal/httpserver/modules_test.go` mounts the self-registered modules in
record-only mode and fails when a `/v1` route is missing from
`openapi.yaml`. Modules that predate the rule sit in `undocumentedModules`
(today: `activity`, `logs`). The mounted list lives in one place,
`internal/httpserver/modules.go` (`selfRegisteredModules`), used by both the
server and the test; the test also fails when one of them becomes
fully documented and is still listed, so the set only shrinks.

## Next steps

1. Move the remaining modules, smallest first (`authsettings`, the
   integrations, `bulk`, `settings`, `organizations`, `tenant`, `qrlogin`,
   `notifications`, `exports`, `imports`, `storage`, `auth`), each with its
   paths documented or listed in `undocumentedModules`.
2. Optional capability interfaces on modules, discovered by the registry:
   `IOAdapters() []ioengine.Adapter`, `BulkAdapters()`, `SearchSpecs()`,
   `EventHandlers(bus)`, `Jobs(worker)`, so registries stop being assembled
   by hand.
3. Once every module is migrated, delete the `RegisterRoutes` functions and
   make the OpenAPI test cover the whole mux.
