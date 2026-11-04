---
name: boilerplate-first-setup
description: >-
  First-time setup for this Next.js + Go boilerplate: root .env, NAME_PREFIX,
  compose local/prod, the schema-driven production env (scripts/prod-env.sh),
  Dokploy network, VAPID, media URL rules, super admin, and make local-dev. Use when initializing the repo, configuring Docker naming,
  or onboarding a new clone.
---

# Boilerplate first setup

Human docs: `docs/DEVELOPMENT.md`. Canonical env is the **repo-root** `.env`.

## Layout

- `backend/` — Go API + worker (`:8080`, `/v1/...`)
- `frontend/` — Next.js BFF (`/platform` + CMS `/`)
- `compose.local.yml` — infra only (Postgres, Redis, SeaweedFS, MailHog, Adminer, Centrifugo, Meilisearch)
- `compose.prod.yml` — full stack on its own app network; frontend + centrifugo also join `dokploy-network`

Do **not** keep a long-lived `frontend/.env.local` for API URLs. Do **not** treat `backend/.env` as canonical.

## 1. Copy env

```sh
cp .env.example .env          # local dev: safe defaults, nothing to generate
```

Production never starts from `.env.example`: `make prod-env-init` generates `.env.server` from `deploy/env/schema` (§7).

`API_URL` **must** include `/v1` (default `http://127.0.0.1:8080/v1`).

## 2. Stack identity (required before sharing a host)

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `POSTGRES_VOLUME_NAME` / `REDIS_VOLUME_NAME` / `SEAWEEDFS_VOLUME_NAME` / `MEILI_VOLUME_NAME` | Named volumes so data does not collide |

In `compose.prod.yml` every internal host is qualified as `<service>.${APP_NETWORK_NAME}` (`postgres.myapp-app`, `backend.myapp-app`, …). Never use a bare service name: on the shared `dokploy-network` it can resolve to another stack's container.

Change secrets in `.env` even for local if the machine is shared: `JWT_*`, `AUTH_SECRET`, `CENTRIFUGO_*`, `S3_SECRET_KEY`, `DB_PASSWORD`, `MEILI_MASTER_KEY`, `APP_ENCRYPTION_KEY`.

## 3. Local run

```sh
make local-dev
make create-super-admin
# optional: make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1' SA_NAME=Ada SA_SURNAME=Admin
```

`make local-dev` starts Docker infra, migrates, then **air** + **pnpm dev** on the host. It does **not** build app images.

| Helper | What |
|--------|------|
| `make infra` / `make infra-down` | Compose infra only |
| `make backend-dev` | air on `:8080` |
| `make frontend-dev` | pnpm on `:3000` |
| `make migrate-up` | SQL migrations |
| `make search-reindex` | Rebuild Meilisearch indexes |

### URLs

| Service | URL |
|---------|-----|
| Frontend / login | http://localhost:3000/platform/login |
| API / Scalar | http://127.0.0.1:8080 · `/docs/` |
| Health | `/healthz` · `/readyz` |
| Adminer | http://127.0.0.1:8081 |
| SeaweedFS S3 gateway (signed requests only; no console) | http://127.0.0.1:9000 |
| MailHog | http://127.0.0.1:8025 |
| Meilisearch | http://127.0.0.1:7700 |

Sign-in role: `super_admin` → `/platform`. Roleless / `cms_user` → `/` (CMS).

## 4. Shells & RBAC

- Platform (`/platform`): `super_admin` or any `platform.*` permission
- CMS (`/`): other authenticated users
- Public register creates a user with **no** roles
- Assign roles from platform admin
- No tenants / workspaces / `/business`

Brand: `frontend/src/config/brand.ts` + `NAME_PREFIX`.

## 5. VAPID (Web Push)

Required for closed-browser notifications; optional otherwise.

```sh
npx web-push generate-vapid-keys
```

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` in `.env`. In production `make prod-env-init` generates the pair.

Frontend: `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Optional public vars (already in `.env.example`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## 6. Media URLs

Browser must **never** assemble S3 URLs from object keys. Use API-returned `public_url` / `logo_url`. Guard: `frontend/src/lib/media/urls.ts`. Skill: `.agents/skills/storage-media`.

## 7. Production / Dokploy

**Prebuilt images.** The server never builds. On a green push to `main`, CI pushes `ghcr.io/<owner>/<repo>-<service>:main`, `:sha-<short>` and `:v0.N` for every entry in `deploy/images.json`, then cuts the next `v0.N` git tag. Dokploy deploys on `v0.*` tags and pulls whatever `BACKEND_IMAGE` / `FRONTEND_IMAGE` name: `:main` follows the latest build, so pin with `:v0.N` to roll back. Full flow and diagram: `docs/DEVELOPMENT.md` → Production.

1. GHCR access: make the packages public, or add a `ghcr.io` registry with a `read:packages` PAT in Dokploy (or run `docker login ghcr.io` on the host).
2. Dokploy deploy trigger: git tags matching `v0.*`.
3. Image variables come from the schema: `prod-env-init` asks `IMAGE_REPO` (`ghcr.io/<owner>/<repo>`) and derives `BACKEND_IMAGE`, `FRONTEND_IMAGE` (`:main`) and `IMAGE_PULL_POLICY=always`.

Secrets: `GITHUB_TOKEN` is built in. The workflow grants it `packages: write` for GHCR and `contents: write` for the tag. Optional `EXPO_TOKEN` enables `mobile-release`; it skips green without it. `auto-pr` needs either Settings → Actions → General → "Allow GitHub Actions to create and approve pull requests" or an `AUTO_PR_TOKEN` PAT. Otherwise merge to `main` by hand. Make `ci` the single required check.

New container (e.g. a widget)? Add `{ "service", "context", "dockerfile" }` to `deploy/images.json` and a service in `compose.prod.yml` that uses `pull_policy: ${IMAGE_PULL_POLICY:-always}`. CI then builds and publishes it automatically.

The production env comes from `deploy/env/schema` (design: `docs/design/env-core.md`). Needs bash 4.3+ and openssl.

1. `make prod-env-init`: asks `APP_URL` and `IMAGE_REPO`, offers defaults for `NAME_PREFIX` (first host label) and `CENTRIFUGO_WS_URL` (`wss://realtime.<host>/…`), derives every URL, generates every secret + the VAPID pair, writes `.env.server` (mode 600, git-ignored). Agents / CI: `make prod-env-init ARGS="--set APP_URL=https://app.example.com --set IMAGE_REPO=ghcr.io/<owner>/<repo> --yes"`. Never invent secret values by hand.
2. Fill `SMTP_*` (and `BACKEND_SENTRY_DSN` / `FRONTEND_SENTRY_DSN` if used)
3. `make prod-config` then `make prod-up`. Dokploy: paste the file into the app's environment; domains frontend:3000 (`APP_URL` host) and centrifugo:8000 (`CENTRIFUGO_WS_URL` host)
4. Plain Docker host only: `docker network create dokploy-network` (once; Dokploy creates it itself)

Later:

| When | Run |
|------|-----|
| Schema changed (new release) | `make prod-env-upgrade`: keeps every value, migrates renames/removals, adds new keys, backup `.env.server.bak` |
| Before a deploy | `make prod-env-check`: exit 1 on version gap, missing keys, dev secrets |
| Existing hand-made `.env.server` | `make prod-env-upgrade` → `make prod-env-check` → rotate what it flags (`scripts/prod-env.sh upgrade --rotate KEY`), deliberately |

Never `--rotate` `APP_ENCRYPTION_KEY` or `DB_PASSWORD` (the script refuses). The backend refuses to boot in staging/production with an empty or development secret, and only accepts `APP_ENV` = `development` | `test` | `staging` | `production`.

Compose creates the app's own private network (`${NAME_PREFIX}-app`). Only **frontend** and **centrifugo** join `dokploy-network` (set their domains in Dokploy: service + container port). Backend, SeaweedFS, Postgres, Redis, Meilisearch and the worker stay private.

`SEARCH_*` / `MEILI_*` enable Cmd+K. `APP_ENCRYPTION_KEY` (32-byte raw or base64) encrypts GitHub App secrets; `prod-env-init` generates it.

New env var? Follow `docs/EXTENDING.md` → "Environment variables" (config.go → `.env.example` → schema → compose → guard), then `make env-check`.

## 8. After clone checklist

- [ ] Unique `NAME_PREFIX` + volume names on a shared Docker host
- [ ] `API_URL` ends with `/v1`
- [ ] `make local-dev` then `make create-super-admin`
- [ ] Login at `/platform/login`
- [ ] MailHog receives verify/reset mail
- [ ] VAPID keys if push is in scope
- [ ] Production: `make prod-env-init` (not a copy of `.env.example`), then `make prod-env-check` passes
- [ ] Production: first admin from the image, `/app/create-super-admin` in the backend container (`docs/DEVELOPMENT.md` → Super admin)
- [ ] Brand tokens swapped if this is a named product
- [ ] `ci` is the required check on `main`; Dokploy pulls `ghcr.io/<owner>/<repo>-*` and deploys on `v0.*` tags
