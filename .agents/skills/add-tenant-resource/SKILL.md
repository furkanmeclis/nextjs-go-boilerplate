---
name: add-tenant-resource
description: >-
  Vertical slice for a new organization-owned resource: table with
  organization_id, org-filtered sqlc queries + isolation lint, tenant.*
  permissions for the owner/admin/member templates, /v1/tenant routes behind
  RequireOrganization, OpenAPI, and a /t/{slug} page using tenantRequest.
  Use when data belongs to one organization (tenant core, TENANCY_ENABLED).
---

# Add a tenant resource

For data that belongs to **one organization**. Platform-wide data uses
`add-platform-resource` instead. Design: `docs/design/tenant-core.md`;
request chain and error codes: `backend/docs/auth.md` → "Tenancy".

Rules that do not bend:

- The organization comes from `orgctx.Must(ctx)` (resolved from the
  `X-Organization` header). Never from the body, the path, or the JWT.
- Repositories take `orgID int64` explicitly and **every** query filters on it.
- `tenant.*` permissions come from the membership role in the current
  organization. No super-admin bypass.

## 1. Migration (`db-migration` skill)

```sql
CREATE TABLE projects (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_projects_org ON projects (organization_id, created_at DESC);
-- + set_updated_at() trigger
```

Uniqueness is per organization: `UNIQUE (organization_id, slug)`, not `UNIQUE (slug)`.

Permissions in the same migration, granted to the tenant **template** roles
(and any per-organization copies, matched by slug + scope):

```sql
INSERT INTO permissions (name, slug) VALUES
    ('Read projects', 'tenant.projects.read'),
    ('Write projects', 'tenant.projects.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r
JOIN permissions p ON p.slug IN ('tenant.projects.read', 'tenant.projects.write')
WHERE r.scope = 'tenant' AND r.slug IN ('owner', 'admin')
ON CONFLICT DO NOTHING;
-- members: read only
```

The `down` deletes the role_permissions rows, then the permissions. Resolved
scopes are cached for `TENANCY_CACHE_TTL` (60s), so new grants show up within
that window after deploy.

## 2. Queries + isolation lint

```sql
-- name: ListProjects :many
SELECT * FROM projects WHERE organization_id = $1
ORDER BY created_at DESC LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetProject :one
SELECT * FROM projects WHERE organization_id = $1 AND uuid = $2;

-- name: UpdateProject :one
UPDATE projects SET name = $3 WHERE organization_id = $1 AND uuid = $2 RETURNING *;
```

Add the table to `backend/internal/database/tenant_tables.txt`. Then
`go test ./internal/database/` fails for any query on it that never mentions
`organization_id`. A query that is cross-organization on purpose carries
`-- tenant-lint: cross-org <reason>` on the line above its `-- name:`.

`make -C backend sqlc` and commit `backend/internal/database/db`.

## 3. RBAC triad

- `backend/internal/platform/rbac/rbac.go`: `PermTenantProjectsRead = "tenant.projects.read"`, … (add to the tenant list next to `PermTenantMembersRead`).
- `frontend/src/config/permissions.ts`: the flat `TenantProjectsRead` key and the nested `permissions.tenant.projects.read`.
- Labels `labels.tenant.projects.read` / `.write` in
  `frontend/src/locales/{en,tr}/permissions.json` (the core tenant slugs have
  no labels yet; add the `tenant` group if it is missing).

## 4. Backend module

`backend/internal/modules/projects/{handler,usecase,repository,model}` +
`routes.go`. Handlers:

```go
s := orgctx.Must(r.Context())
items, total, err := h.svc.List(r.Context(), s.ID, q.Limit, q.Offset)
```

Return 404 (not 403) for a uuid of another organization: the org-filtered
query simply finds nothing.

Routes, mounted only when tenancy is on (same resolver as
`internal/modules/tenant/routes.go`):

```go
authn := middleware.Authenticate(tokens, loader)
org := middleware.RequireOrganization(resolver)
perm := middleware.RequireTenantPermission
mux.Handle("GET /v1/tenant/projects", middleware.Chain(
    http.HandlerFunc(h.list), authn, org, perm(rbac.PermTenantProjectsRead)))
```

Mount it like `tenant/routes.go`: `server.go` builds the handler inside
`if cfg.Tenancy.Enabled` and passes `tenantResolver`; `RegisterRoutes`
returns early when the resolver is nil. With the module registry
(`docs/design/module-registry.md`) use `r.Tenant(pattern, perm, h)` in a
`module.Module` instead: it skips the route when tenancy is off. Paths live
under `/v1/tenant/` so the BFF forwards `X-Organization`.

Engines pick the organization up from the context: activity
(`activity.Recorder.Record`), notifications (`Enqueue`), export/import/bulk
jobs are stamped automatically. Adapters that run in workers read
`orgctx.Must(ctx).ID` (the worker restores it from the job row) and must
filter by it.

Audit with `h.record(...)`-style activity calls; invalidate the org cache
only when something changes membership resolution (`orgctx.Invalidate`).

## 5. OpenAPI

Every route in `backend/docs/openapi.yaml` in the same change, with the
`XOrganization` header parameter and the 400/403 tenant responses used by the
existing `/v1/tenant/*` operations. Then:

```sh
make -C backend openapi-lint
cd frontend && pnpm api:generate
```

## 6. Frontend

- Service: `frontend/src/features/projects/services/*.ts` calls
  `tenantRequest<T>(slug, "GET", "/v1/tenant/projects", { query })` (adds
  `X-Organization`; see `features/tenant/services/tenant.service.ts`). Never
  `apiRequest` for tenant routes.
- Hooks: query keys include the slug (see `tenantAdminKeys` in
  `features/tenant/services/tenant.service.ts`) so organizations never share
  cache entries.
- Page: `frontend/src/app/(tenant)/t/[slug]/projects/page.tsx`, rendering a
  feature component that reads `useTenant()` and gates with
  `usePermission().can(permissions.tenant.projects.read)` (inside the tenant
  shell, `usePermission` answers with the current membership's permissions).
- Routes: `routes.tenant.projects(slug)` in `config/routes.ts`.
- Nav: an item in `tenantNav(slug)` (`config/nav.ts`) with its `permission`.
- i18n: keys in `en` and `tr`; `pnpm -C frontend i18n:check`.

## 7. Verify

`verify-changes` skill, plus a two-organization check: create a row in org A,
request it with `X-Organization: B` as a member of both → 404; list in B does
not show it.
