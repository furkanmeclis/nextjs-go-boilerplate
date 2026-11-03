# Development

## Prerequisites

- Go 1.22+ (match `backend/go.mod`)
- Node 20.9+ and pnpm
- Docker + Compose
- golang-migrate, sqlc, air, golangci-lint, gofumpt — see [backend/TOOLCHAIN.md](../backend/TOOLCHAIN.md)

## First clone

```sh
cp .env.example .env
```

Set these **before** sharing a Docker host with another stack:

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `*_VOLUME_NAME` | Named volumes so SeaweedFS/Postgres do not collide |

In `compose.prod.yml` every internal host is qualified as `<service>.${APP_NETWORK_NAME}` (`postgres.myapp-app`, `backend.myapp-app`, …). Never use a bare service name: on the shared `dokploy-network` it can resolve to another stack's container.

Root `.env` is canonical. `backend/.env` is a fallback only. Do not keep a long-lived `frontend/.env.local` for API URLs; `API_URL` comes from the root env.

`API_URL` **must** include `/v1` (Go routes are `/v1/...`). Default: `http://127.0.0.1:8080/v1`.

## Local

```sh
make local-dev
make create-super-admin
```

Helpers:

| Target | What it does |
|--------|----------------|
| `make infra` / `make infra-down` | Compose infra only |
| `make backend-dev` | air on `:8080` |
| `make frontend-dev` | `pnpm dev` on `:3000` |
| `make migrate-up` | Apply SQL migrations |
| `make create-super-admin` | Upsert platform admin + `super_admin` role |
| `make search-reindex` | Rebuild Meilisearch indexes (users, roles, …) |
| `make check-i18n` | Find missing / unwired translation keys (en+tr, activity, RBAC, exports) |

### Local URLs

| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000 |
| API | http://127.0.0.1:8080 |
| Scalar docs | http://127.0.0.1:8080/docs/ |
| Health / ready | http://127.0.0.1:8080/healthz · `/readyz` |
| Adminer | http://127.0.0.1:8081 |
| SeaweedFS S3 gateway (signed requests only; no console) | http://127.0.0.1:9000 |
| MailHog | http://127.0.0.1:8025 |
| Meilisearch | http://127.0.0.1:7700 |
| Centrifugo WS | ws://127.0.0.1:8000/connection/websocket |

Login: http://localhost:3000/platform/login

## Branding

Swap name and colors in `frontend/src/config/brand.ts` and the CSS tokens in the frontend theme. Default product name is **App**.

## VAPID (optional Web Push)

```sh
npx web-push generate-vapid-keys
```

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` in `.env` (production: `prod-env-init` generates the pair). The frontend registers via `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Closed-browser push does not work until these three are set.

Optional frontend public vars (root `.env`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## Production / Dokploy

The production env is generated from `deploy/env/schema`; never start from `.env.example` (design: [design/env-core.md](./design/env-core.md)). Needs bash 4.3+ and openssl (macOS: `brew install bash`).

1. `make prod-env-init` asks for `APP_URL` and offers defaults for `NAME_PREFIX` (first host label) and the Centrifugo URL (`wss://realtime.<host>/…`). It derives every URL, generates every secret and the VAPID pair, and writes `.env.server` (mode 600, git-ignored). Non-interactive: `make prod-env-init ARGS="--set APP_URL=https://app.example.com --yes"`.
2. Fill `SMTP_*` (and the Sentry DSNs if you use them) in `.env.server`.
3. `make prod-config` then `make prod-up`. On Dokploy, paste the file into the compose app's environment and add two domains: **frontend** (port 3000) on the `APP_URL` host and **centrifugo** (port 8000) on the `CENTRIFUGO_WS_URL` host.
4. Plain Docker host only (Dokploy creates it for you): `docker network create dokploy-network` once

Keep the generated file as the master copy (password manager or the server) and run the commands below on it; values edited only in the Dokploy UI are invisible to `check`.

| Task | Command |
|---|---|
| After pulling a schema change | `make prod-env-upgrade`: keeps every value, applies renames/removals, fills only new keys, re-stamps `ENV_SCHEMA_VERSION`, backup in `.env.server.bak` |
| Verify a file | `make prod-env-check`: exits 1 on a schema-version gap, missing or blank keys, or secrets still holding `.env.example` values |
| Rotate a secret on purpose | `scripts/prod-env.sh upgrade --rotate JWT_REFRESH_SECRET` (confirmation; restart the stack). Refuses `APP_ENCRYPTION_KEY` (encrypted data becomes unreadable) and `DB_PASSWORD` (Postgres keeps it in its volume: `ALTER USER` first, then edit the file) |
| Older hand-made `.env.server` | `make prod-env-upgrade`, then `make prod-env-check`, then rotate every secret it reports as insecure. Rotation is never automatic |

The backend enforces the same rules at boot: `APP_ENV` must be `development`, `test`, `staging` or `production`, and in staging/production it refuses to start while any managed secret is empty or still holds its public development value, naming every offending key.

Networks: Compose creates a private bridge network per app (`${APP_NETWORK_NAME:-${NAME_PREFIX}-app}`) and every service joins it. Only **frontend** (`:3000`) and **centrifugo** (`:8000`) also join the shared `dokploy-network`, so the proxy can route their domains. The backend is reached through the frontend's `/api` proxy, and SeaweedFS, Postgres, Redis, Meilisearch and the worker stay private (files reach browsers through API endpoints, never a bucket URL). No service publishes host ports.

Object storage is SeaweedFS's S3 gateway (`seaweedfs`, port 8333 on the app network), reached through the backend's S3 driver (`STORAGE_DRIVER=s3`). The only S3 identity is the backend's (`S3_ACCESS_KEY` / `S3_SECRET_KEY`); anonymous requests are denied. `seaweedfs-init` creates `S3_BUCKET` before the backend starts. **Coming from MinIO:** SeaweedFS starts on a new, empty volume (`SEAWEEDFS_VOLUME_NAME`); the old `*-minio` volume is left untouched. Copy the bucket across with any S3 client (for example `rclone sync` between two S3 remotes, path-style, same credentials) before switching an existing deployment, or point `S3_ENDPOINT` at the old store until you have.

`APP_ENCRYPTION_KEY` (32-byte, raw or base64) encrypts GitHub App secrets at rest. `prod-env-init` generates it; never rotate it once data is stored.

## Error tracking (optional, Sentry protocol)

The tracker is a self-hosted, Sentry-protocol endpoint (events only; it has no `sentry-cli` API, so nothing is uploaded at build time: no source maps, no releases, no auth token). Every variable is optional; an empty DSN turns reporting off.

| Env (Dokploy / `.env.server`) | Passed as | Read by |
|---|---|---|
| `BACKEND_SENTRY_DSN` | `SENTRY_DSN` | backend API + worker |
| `FRONTEND_SENTRY_DSN` | `SENTRY_DSN` | Next.js server, edge, browser |
| `SENTRY_ENVIRONMENT` (default `production`) | same | both |
| `SENTRY_TRACES_SAMPLE_RATE` (default `0`) | same | backend only |

Local dev reads `SENTRY_DSN` from the root `.env` directly.

- **Backend**: 5xx responses and panics (route pattern, status, `request_id` tag), auth misconfiguration warnings (adapter secret mismatch, provider disabled, id_token audience/issuer), failed queue tasks (`task_type` tag), and `ERROR` log lines (one event per message per minute). Requests the client cancelled are logged as `499` at INFO and never reported. One `http_request` INFO line is written per 4xx and per `/v1/auth/*` / `/v1/internal/auth/*` request (method, route, status, duration, request_id, error_code, scrubbed reason).
- **Frontend**: `instrumentation.ts` (server/edge, `onRequestError`), `instrumentation-client.ts` (browser), `global-error.tsx`, route error boundaries, the `/api/v1` BFF proxy, and the NextAuth `logger.error` hook (user errors like `CredentialsSignin` are logged, not reported). The browser gets its DSN at runtime from `/api/client-env` (loaded `beforeInteractive` in the root layout), so the image needs no build arg.
- **Never sent**: request bodies, cookies, query strings, `Authorization` / adapter headers, tokens, secrets, emails, phone numbers. The user is the internal user uuid only. Code: `backend/internal/observability`, `frontend/src/lib/observability`.

## Super admin

From the repo root (loads `.env`):

```sh
make create-super-admin
make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1' SA_NAME=Ada SA_SURNAME=Admin
```

The same target exists under `backend/` (`make -C backend create-super-admin …`).

## After a backend contract change

```sh
make -C backend sqlc
make -C backend openapi-lint
cd frontend && pnpm api:generate && pnpm typecheck
```
