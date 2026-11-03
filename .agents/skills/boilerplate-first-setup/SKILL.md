---
name: boilerplate-first-setup
description: >-
  First-time setup for this Next.js + Go boilerplate: root .env, NAME_PREFIX,
  compose local/prod, Dokploy network, VAPID, media URL rules, super admin,
  and make local-dev. Use when initializing the repo, configuring Docker naming,
  or onboarding a new clone.
---

# Boilerplate first setup

Human docs: `docs/DEVELOPMENT.md`. Canonical env is the **repo-root** `.env`.

## Layout

- `backend/` — Go API + worker (`:8080`, `/v1/...`)
- `frontend/` — Next.js BFF (`/platform` + CMS `/`)
- `compose.local.yml` — infra only (Postgres, Redis, MinIO, MailHog, Adminer, Centrifugo, Meilisearch)
- `compose.prod.yml` — full stack on its own app network; frontend + centrifugo also join `dokploy-network`

Do **not** keep a long-lived `frontend/.env.local` for API URLs. Do **not** treat `backend/.env` as canonical.

## 1. Copy env

```sh
cp .env.example .env
# production later:
cp .env.example .env.server   # then replace every secret
```

`API_URL` **must** include `/v1` (default `http://127.0.0.1:8080/v1`).

## 2. Stack identity (required before sharing a host)

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `POSTGRES_VOLUME_NAME` / `REDIS_VOLUME_NAME` / `MINIO_VOLUME_NAME` / `MEILI_VOLUME_NAME` | Named volumes so data does not collide |

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
| MinIO console | http://127.0.0.1:9001 |
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

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` in `.env`.

Frontend: `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Optional public vars (already in `.env.example`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## 6. Media URLs

Browser must **never** assemble MinIO/S3 URLs from object keys. Use API-returned `public_url` / `logo_url`. Guard: `frontend/src/lib/media/urls.ts`. Skill: `.agents/skills/storage-media`.

## 7. Production / Dokploy

**Prebuilt images.** The server never builds. On a green push to `main`, CI pushes `ghcr.io/<owner>/<repo>-<service>:main` and `:sha-<short>` for every entry in `deploy/images.json`, then cuts the next `v0.N` tag. Dokploy deploys on `v0.*` tags and pulls the images. Full flow and diagram: `docs/DEVELOPMENT.md` → Production.

1. Fill `.env.server` (or the Dokploy env) with a unique `NAME_PREFIX`. Every `${VAR:?…}` is required.
2. Set `BACKEND_IMAGE=ghcr.io/<owner>/<repo>-backend:main`, `FRONTEND_IMAGE=ghcr.io/<owner>/<repo>-frontend:main` and `IMAGE_PULL_POLICY=always`.
3. GHCR access: make the packages public, or add a `ghcr.io` registry with a `read:packages` PAT in Dokploy (or run `docker login ghcr.io` on the host).
4. Dokploy deploy trigger: git tags matching `v0.*`.
5. Plain Docker host only: run `docker network create dokploy-network` once (Dokploy creates it itself). For a manual deploy, run `make prod-config`, then `make prod-up` (pulls, never builds).

Secrets: `GITHUB_TOKEN` is built in. The workflow grants it `packages: write` for GHCR and `contents: write` for the tag. Optional `EXPO_TOKEN` enables `mobile-release`; it skips green without it. `auto-pr` needs either Settings → Actions → General → "Allow GitHub Actions to create and approve pull requests" or an `AUTO_PR_TOKEN` PAT. Otherwise merge to `main` by hand. Make `ci` the single required check.

New container (e.g. a widget)? Add `{ "service", "context", "dockerfile" }` to `deploy/images.json` and a service in `compose.prod.yml` that uses `pull_policy: ${IMAGE_PULL_POLICY:-always}`. CI then builds and publishes it automatically.

Compose creates the app's own private network (`${NAME_PREFIX}-app`). Only **frontend** and **centrifugo** join `dokploy-network` (set their domains in Dokploy: service + container port). Backend, MinIO, Postgres, Redis, Meilisearch and the worker stay private.

`SEARCH_*` / `MEILI_*` enable Cmd+K. `APP_ENCRYPTION_KEY` (32-byte raw or base64) encrypts GitHub App secrets. Generate: `openssl rand -base64 32`.

## 8. After clone checklist

- [ ] Unique `NAME_PREFIX` + volume names on a shared Docker host
- [ ] `API_URL` ends with `/v1`
- [ ] `make local-dev` then `make create-super-admin`
- [ ] Login at `/platform/login`
- [ ] MailHog receives verify/reset mail
- [ ] VAPID keys if push is in scope
- [ ] `APP_ENCRYPTION_KEY` replaced if GitHub OAuth will be used
- [ ] Brand tokens swapped if this is a named product
- [ ] `ci` is the required check on `main`; Dokploy pulls `ghcr.io/<owner>/<repo>-*` and deploys on `v0.*` tags
