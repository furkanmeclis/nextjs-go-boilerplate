# Environment config core — design study

Status: **implemented** (phases 1–5: schema, `scripts/prod-env.sh`, generated `.env.server.example`, compose-env check, runtime guard, docs + skill; see §12). Operator docs: `docs/DEVELOPMENT.md` → Production; adding a variable: `docs/EXTENDING.md` → Environment variables. Later phases are proposals. Reference implementations live in sibling forks of this boilerplate: `rentopia-app-go` (mature schema-driven generator), `olexfilms` (earlier non-schema generator with a careful rotation policy), `clickfix` (a compose-env CI check). This document surveys that prior art, recommends the **healthiest** version for the core, and lays out a phased plan. It deliberately does **not** copy any one fork.

## 1. Goal

Every app cloned from the boilerplate must configure its environment the same predictable way it already gets a platform shell or a tenant shell. Today it does not: there are **four** hand-maintained env surfaces that drift against each other and against the code that reads them, and production secrets are produced by hand.

- `.env.example` — the real local-dev file (safe defaults, ~180 lines).
- `.env.server.example` — a hand-written *prose description* of the prod key list (not a real env file; drifts the moment a key is added).
- `backend/.env.example`, `frontend/.env.example` — pointer stubs.
- `backend/internal/config/config.go` — the actual source of truth for what the backend reads, with four hand-written "refuse the default secret in prod" checks (`APP_ENCRYPTION_KEY`, `AUTH_ADAPTER_SECRET`, `CENTRIFUGO_*`, `MEILI_MASTER_KEY`, plus JWT).
- `compose.prod.yml`'s `x-backend-environment` block — the surface that actually decides whether a value set in Dokploy reaches the container.

A key can be added to `config.go` and silently forgotten in `compose.prod.yml` (set in Dokploy, never delivered). A secret can be documented in `.env.server.example` but never generated. The prod operator today copies `.env.example` to `.env.server` and edits ~15 secrets by hand, which is where mistakes and reused dev secrets come from.

**Goal:** make a single versioned schema the source of truth, generate the prod env from it (asking the operator only for the app URL), generate the prod key-list doc from it, and add CI + runtime guards that fail when the surfaces fall out of sync. Local `make local-dev` must keep working untouched throughout — no generation step to run locally.

Out of scope for the core: committing encrypted secrets to git (SOPS/age — see §3), per-developer shell env (direnv — §3), and any app-specific key (`rentopia`, domain seeds). Those stay in forks or plug in later.

## 2. Prior art

| Fork | What it does | Keep | Drop |
|---|---|---|---|
| **rentopia** `deploy/env/schema` + `scripts/prod-env.sh init\|upgrade\|check` | One schema file is the source of truth. Asks only `APP_URL`; derives all URLs, generates all secrets, writes `.env.server`, stamps `ENV_SCHEMA_VERSION`. `upgrade` preserves every value, applies renames, generates only missing keys, comments out removed ones, keeps a backup. `check` reports missing keys + version gap, exits non-zero. bash 4+, openssl, awk. | The whole shape: schema → generator, version + migrations, preserve-on-upgrade, derive-from-URL. | `const:rentopia` for `NAME_PREFIX` (app-specific — must be parameterised, §4). Domain-specific keys. |
| **olexfilms** `scripts/gen-env-server.sh` | Earlier, non-schema generator (keys hard-coded in the script). | Its hard lesson: an **explicit** `--rotate`, and **never** implicitly rotate `APP_ENCRYPTION_KEY` (breaks data encrypted at rest) or any PII key (§6). | The non-schema approach itself — keys in a script drift the same way the four lists do. |
| **clickfix** `scripts/check-compose-env.sh` | CI check: every env var the Go backend reads is present in `compose.prod.yml`'s `x-backend-environment`, so a Dokploy value actually reaches the container. | The check, generalised with an explicit "has a safe Go default, deliberately not wired" allowlist (§8). | — |
| **rentopia** `backend/internal/platform/envguard` | Runtime boot guard: validates `APP_ENV`, refuses fake providers in prod, etc. | The **generic** idea only: validate `APP_ENV`, refuse known default/example secret values in prod (§9). | Rentopia's domain-specific integration rules. |

The boilerplate is in a good starting position for this: **all** backend env reads are centralised in `config.go` (`getEnv`/`getInt`/`getBool`/…), and `validate()` already refuses four default secrets outside development. This work generalises that, rather than inventing it.

## 3. Options considered — the "healthiest way"

The owner asked for the healthiest approach, not a blind copy. Five candidates:

| Option | Solves drift? | Generates prod env + secrets? | "Ask one question" ergonomics? | New dependency | Verdict |
|---|---|---|---|---|---|
| **A. Schema-driven generator** (rentopia, generalised) | **Yes** — one schema drives generator + docs + guards | **Yes** | **Yes** | bash 4+, openssl, awk (all already present) | **Recommended core.** |
| B. `dotenv-linter` | No — lints format/dupes/order only; knows nothing about required keys, secrets, or derivation | No | No | Rust binary in CI | Rejected for core; optional format lint later. |
| C. `direnv` | No — per-dev shell loading; prod secrets and schema untouched; local dev already loads `.env` via the Makefile | No | No | per-dev install | Rejected; orthogonal dev-ergonomics tool, not a core concern. |
| D. **SOPS + age** (encrypt secrets in git) | Partly — versions *values*, not the key list | Stores, doesn't generate | No | sops + age + key management | Rejected for core. The prod model is "generate on the host, Dokploy holds the env"; nothing is committed, so there is no secret-in-git problem to solve. Note as a future opt-in for GitOps forks. |
| E. **Typed loader** (`caarlos0/env`, `koanf`, `viper`) replacing `config.go` | No for the shell surfaces — only restructures the Go read side | No | No | Go module + large refactor | Rejected as the driver. `config.go` is already a centralised, validating loader; a rewrite is risk with no drift payoff. The **runtime guard** (§9) is the piece worth adding, as a small extension to the existing `validate()`. |

**Recommendation: Option A as the spine, with the runtime guard from E's spirit layered on the existing `config.go`.** Only A kills the drift across all four lists *and* gives the one-question ergonomics *and* adds zero real dependencies (bash/openssl/awk are in CI and on every Dokploy host). B/C/D/E each solve a narrower or different problem; they are noted as optional add-ons, not core.

Healthiest-version refinements beyond rentopia's fork:
- **Boilerplate-neutral.** No app name is ever `const:`. `NAME_PREFIX` is asked (with a sensible default derived from the host label), never hardcoded.
- **The schema also generates `.env.server.example`.** Stop hand-maintaining the prose file; a CI sync check regenerates it and `git diff --exit-code`s.
- **The schema is the single source for the runtime guard too.** A Go test embeds the schema and asserts that every `secret:` key has a matching default-refusal entry in `config.go`, so adding a secret to the schema *forces* a guard entry (§9). This is the link that makes "single source of truth" real rather than aspirational.
- `.env.example` stays hand-written and frictionless for local dev — not generated. An optional lint (§10) keeps its key *set* a superset of the schema's, so a prod key always has a local default.

## 4. Schema format

One file, `deploy/env/schema` (implemented in phase 1; the file itself is the reference, this is an excerpt). Line-oriented and diffable. Adapted from rentopia and generalised.

```
@version 1

@norotate APP_ENCRYPTION_KEY
@norotate DB_PASSWORD

# --- Identity ---
# Public origin of the app (the frontend domain). Every URL below derives from it.
APP_URL | ask:{AUTH_URL}
# Unique per app on a host: names containers, volumes and the app network.
NAME_PREFIX | ask:{APP_LABEL}
BACKEND_IMAGE | derive:{NAME_PREFIX}-backend:latest

# --- Public URLs ---
AUTH_URL | derive:{APP_URL}
CORS_ALLOWED_ORIGINS | derive:{APP_URL}
# Centrifugo has its own domain (Dokploy: service centrifugo, container port 8000).
CENTRIFUGO_WS_URL | ask:wss://realtime.{APP_HOST}/connection/websocket

# --- Signing and encryption secrets ---
JWT_ACCESS_SECRET | secret:hex:32
# Encrypts credentials stored in the database. Never rotate it: they become unreadable.
APP_ENCRYPTION_KEY | secret:b64:32
VAPID_PUBLIC_KEY | vapid:public
VAPID_PRIVATE_KEY | vapid:private
SMTP_HOST | empty
```

`#` comment lines and blank lines are copied into the generated files, so the schema documents itself for operators; `##` lines are schema-only notes. Keys the schema does not list (tenancy, mobile auth, log level, …) keep their `compose.prod.yml` defaults and stay documented in `.env.example`; the schema covers what compose **requires** plus the few optional keys operators actually set (SMTP, Sentry).

Rules (one per managed key, `KEY | rule`):

| Rule | Meaning |
|---|---|
| `ask:<default>` | Prompt the operator (or take `--set KEY=…`). `<default>` is a template; with `--yes` the default is accepted. Only `APP_URL` has no usable default on a fresh file; `NAME_PREFIX` defaults to the first host label (`app` when that would not be a valid prefix); `CENTRIFUGO_WS_URL` defaults to `wss://realtime.<host>/…`. A default that names a missing key resolves to empty, which is how `APP_URL | ask:{AUTH_URL}` upgrades legacy files. |
| `derive:<template>` | Computed. `{KEY}` = any **earlier** key, plus `{APP_HOST}`, `{APP_SCHEME}`, `{APP_LABEL}` taken from `APP_URL`. Checked at parse time. |
| `const:<value>` | Fixed literal (e.g. `DB_USER=app`, `SMTP_PORT=587`). |
| `secret:hex\|b64\|alnum:<n>` | Generated by openssl (`hex`/`b64`: n bytes, `alnum`: n chars). Generated once, preserved forever. `b64:32` is what `crypto.NewSecretBox` expects for `APP_ENCRYPTION_KEY`. |
| `vapid:public` / `vapid:private` | One P-256 key pair in web-push format (base64url, no padding), generated together. |
| `empty` | Managed but blank; the operator fills it. |

Directives:

| Directive | Meaning |
|---|---|
| `@version N` | Current schema version; stamped as `ENV_SCHEMA_VERSION=N`. Bump it with every change. |
| `@rename OLD NEW <ver>` | At `<ver>` OLD became NEW; `upgrade` moves the **existing value** (a deployed secret survives the rename). |
| `@remove KEY <ver>` | At `<ver>` KEY was dropped; `upgrade` comments it out (`# removed@<ver>: KEY=…`) so a rollback can recover it. |
| `@norotate KEY` | `upgrade --rotate` refuses KEY because rotating it destroys data. Forks add their PII keys here. |

Image names are **derived** (`{NAME_PREFIX}-backend:latest`, what `make prod-up` builds) rather than asked, so `init` still needs a single answer. The GHCR session will likely change these two rules to a registry path; that is a one-line schema change plus a `@version` bump.

## 5. Generator CLI — `scripts/prod-env.sh`

Four subcommands. bash 4.3+ and openssl; no network. Tests: `scripts/prod-env.test.sh`.

- **`init`** — fresh `.env.server`. Prompts `APP_URL` then `NAME_PREFIX` (default offered); derives every URL/name; generates every secret and the VAPID pair; writes `const`/`empty` keys; stamps `ENV_SCHEMA_VERSION`. Refuses to clobber an existing `.env.server` (use `upgrade`).
- **`upgrade`** — migrate an existing `.env.server` to the current `@version`. Backs up to `.env.server.bak` first. Keeps **every existing value verbatim** (secrets never change), applies `@rename`/`@remove` in version order, generates only keys that are newly present and missing, re-stamps `ENV_SCHEMA_VERSION`. Idempotent: a second run is a no-op.
- **`check`** — read-only. Reports keys in the schema but missing from `.env.server`, keys present but removed by the schema, and the `ENV_SCHEMA_VERSION` gap (file N vs schema M). Exits non-zero on any finding. Used by operators and by CI against committed example files.
- **`example`** — render the schema to `.env.server.example` on stdout: every managed key with a placeholder (`APP_URL` → `https://app.example.com`, secrets → `# generate: openssl …`, derives resolved against the example host), plus a generated header warning not to hand-edit. This file is committed and kept in sync by §7.

Secret rotation is **not** a subcommand; it is an explicit flag (§6).

## 6. Secret safety & rotation

The non-negotiable lesson from olexfilms:

- `upgrade` **never** regenerates an existing secret. Only missing keys are generated.
- Rotation is explicit opt-in: `scripts/prod-env.sh upgrade --rotate KEY[,KEY]`. It prints a loud warning naming each key and what rotating it invalidates (sessions, realtime tokens, …) and backs up first.
- A **block list** (`@norotate` in the schema) refuses rotation of keys whose rotation destroys data, regardless of `--rotate`: `APP_ENCRYPTION_KEY` (data encrypted at rest becomes unreadable) and `DB_PASSWORD` (Postgres keeps the password in its volume, so a new value only locks the app out; change it with `ALTER USER` first). Forks add PII keys the same way. These change only through a deliberate, documented procedure, never through this script.
- Secrets are generated on the host and land only in `.env.server`, which is `.gitignore`d. Nothing secret is ever committed or logged (the script echoes key *names*, never values).
- **Gap to close in phase 1:** `.gitignore` ignores `.env.server` and `.env` but **not** `.env.server.bak`, the backup `upgrade` writes. Add `.env.server.bak` (and `.env.server.*` except `.env.server.example`) to `.gitignore` so a backup full of live secrets can never be committed.

## 7. Generated `.env.server.example` + sync check

`.env.server.example` stops being a hand-written prose file and becomes `scripts/prod-env.sh example` output, committed. `scripts/check-env-sync.sh` (`make env-check`) regenerates it and asserts no diff, checks that every schema key has a local default in `.env.example` (open question 2, on), and runs `scripts/prod-env.test.sh`. In essence:

```sh
scripts/prod-env.sh example > /tmp/env.server.example.gen
git diff --no-index --exit-code .env.server.example /tmp/env.server.example.gen
```

Adding a key to the schema without regenerating the example fails CI. This is what stops the prose file from drifting.

## 8. compose-env CI check — `scripts/check-compose-env.sh`

Generalises clickfix's check (implemented in phase 3; `make compose-env-check`). Every env var the Go backend **reads** must either reach the `backend` and `worker` containers or carry a recorded decision in `deploy/env/compose-env.allow`.

- **Read set:** string-literal `getEnv`/`getInt`/`getInt64`/`getBool`/`getDuration`/`getFloat`/`os.Getenv`/`os.LookupEnv` calls in non-test Go files under `backend/` (87 keys today, all in `config.go`). Dynamic key names are not detected; keep reads literal.
- **Delivered set:** not a parse of the raw YAML but each service's environment from `docker compose config`, resolved with an env that `prod-env.sh init` generates from the schema. That is exactly what the container receives (anchors, `<<:` merges, per-service overrides), and it keeps working while other sessions restructure the file. As a side effect, the check fails if compose starts **requiring** a variable the schema does not manage.
- **Allowlist** (`KEY | omit|wire | reason`):
  - `omit` = deliberately not delivered. Storage aliases (`MINIO_*`, `S3_PUBLIC_BASE_URL`, `STORAGE_LOCAL_PATH`), `SMTP_FROM` (alias of `MAIL_FROM`), `REDIS_DB`, `CENTRIFUGO_ENABLED`.
  - `wire` = should be delivered but is not yet: 11 operator tunables (`JWT_*_TTL`, `DB_MAX_CONNS`, `STORAGE_MAX_BYTES`, `BULK_*`, `HTTP_*_TIMEOUT`, `CENTRIFUGO_TOKEN_TTL`, `SENTRY_RELEASE`). A value set in Dokploy is silently dropped today. They are reported as a warning together with the graft snippet (`KEY: ${KEY:-}`), which is behaviour-neutral: an empty value makes every `config.go` helper fall back to its default. The snippet is left for the compose owners, since this work does not edit `compose.prod.yml`.
- **Fails on:** a read key with no delivery and no decision; a stale entry (key no longer read, or listed but now delivered, which is how grafting the snippet prompts the cleanup); a service with no environment (renamed: pass `--services`); a malformed allowlist.
- Tests: `scripts/check-compose-env.test.sh` (dropped keys incl. names with digits, new reads, test files ignored, stale entries, the snippet graft end to end).

The frontend container (open question 6) is not covered yet: its reads (`process.env.*`, with `NEXT_PUBLIC_*` inlined at build time) need a different extractor.

## 9. Runtime guard

Implemented in phase 4 as `backend/internal/config/guard.go`, called from the existing `validate()` (open question 4: extend `config.go` rather than add `platform/envguard`).

1. **`APP_ENV` must be one of** `development`, `test`, `staging`, `production`. Anything else stops the process at boot, with a hint for common slips (`prod` → `production`, `dev` → `development`). Before this, any unknown value silently got the production checks; only the redis key namespaces use `APP_ENV` otherwise.
2. **One table, `managedSecrets`**: `{env key, how to read it from Config, its public development values, when it applies}`. Outside `development`/`test` (staging included), every applicable secret that is empty or still holds a public development value is refused, and **all** offenders are reported in one error. It replaces the hand-written checks for JWT, `APP_ENCRYPTION_KEY`, `AUTH_ADAPTER_SECRET`, `CENTRIFUGO_*` and `MEILI_MASTER_KEY`, and adds `DB_PASSWORD` (`change_me`), `REDIS_PASSWORD` (empty) and the S3 credentials (`minioadmin`, checked against the store the active `STORAGE_DRIVER` really uses). Conditional rows: Centrifugo keys only when it is enabled, the Meili key only with search on, S3 keys only for the `minio`/`s3` drivers. The existing `JWT_ACCESS_SECRET` ≥ 32 chars rule stays.
3. **Single-source link** (`guard_test.go`). `go:embed` cannot reach `deploy/` from the `backend/` module, so the tests read the schema and `.env.example` from the checkout:
   - every `secret:`/`vapid:` key in the schema has a table row, or an entry in `unguardedSchemaSecrets` with a reason (`AUTH_SECRET`: frontend only; the VAPID pair: public / no dev default), and no row guards a key the schema dropped;
   - every non-empty `.env.example` value of a guarded key is in that row's defaults, so a dev value can never be accepted in production;
   - each row reads the env key it names (checked through the real `Load()`), plus behaviour tests for defaults, empties, disabled features, the active object store, staging and `APP_ENV` hints.

Local dev is unchanged: `development`/`test` accept every default. A `.env.server` from `prod-env.sh init` passes; a copied `.env.example` is refused with every offending key listed.

## 10. Local dev stays frictionless

`.env.example` remains hand-written with working defaults; `cp .env.example .env && make local-dev` needs no generation. The schema governs the **prod** surface only.

Optional guard against the two surfaces drifting apart: a lint asserting every managed key in the schema also appears in `.env.example` (key set, not value). This guarantees a prod key always has a local default. Proposed as a light CI grep, togglable; flagged as an open question (§13) because it couples the two files more tightly than some forks may want.

## 11. Versioning & migrations

`ENV_SCHEMA_VERSION` is stamped into generated files. A deployed `.env.server` behind the schema is detected by `check` (the version gap) and fixed by `upgrade`, which walks `@rename`/`@remove` directives in version order so a years-old deployment can be brought current without losing a single secret. CI runs `check` against the committed example files, so the schema and `ENV_SCHEMA_VERSION` can never silently diverge.

## 12. Delivery plan

Each phase is one commit on the session branch (auto-PR is off; the owner merges to `main` and manages merge order). `main` stays shippable and `make local-dev` keeps working after every phase.

| Phase | Commit | Scope | Done when |
|---|---|---|---|
| 0 | this doc | `docs/design/env-core.md` | Owner approves the approach (done) |
| 1 | schema + generator | `deploy/env/schema`, `scripts/prod-env.sh init\|upgrade\|check\|example`, `.gitignore` fix for `.env.server.bak` (§6) | `init` produces a valid `.env.server` that `make prod-config` accepts; `upgrade` preserves secrets across a rename; `check` exits non-zero on a gap |
| 2 | generated example + sync | replace `.env.server.example` with `example` output; `make env-check` + CI step; refresh `backend/.env.example`, `frontend/.env.example` pointers | Editing the schema without regenerating fails CI |
| 3 | compose-env check | `scripts/check-compose-env.sh` + CI step (+ allowlist) | A backend-read key missing from `x-backend-environment` and the allowlist fails CI |
| 4 | runtime guard | table-driven default-refusal in `config.go` + `APP_ENV` set validation + the embed-schema test | Booting staging/prod with any default managed secret fails with a named error; the schema↔guard test passes |
| 5 | docs + skill | `boilerplate-first-setup` SKILL, `docs/DEVELOPMENT.md`, `AGENTS.md`/`CLAUDE.md` pointers for the new `prod-env.sh` flow | A new clone can reach prod by following the skill: `init` → fill optionals → `make prod-up` |

**Coordination with the two concurrent `compose.prod.yml` sessions (SeaweedFS storage, GHCR images):** phases 1–2 and 4 do not touch `compose.prod.yml`. Phase 3 only **reads** it. If phase 3's check finds a backend key that must be delivered, the only edit is adding that key to the `x-backend-environment` block — never storage or image wiring. Expect to rebase; the owner sequences the merges. `BACKEND_IMAGE`/`FRONTEND_IMAGE` schema rules (§4) may need adjusting once the GHCR work settles their exact form — flagged for the owner.

## 13. Decisions on the open questions

1. **`NAME_PREFIX`**: asked, defaulting to the first host label (`app` when that is not a valid prefix).
2. **`.env.example` ⊆ schema lint**: on (`scripts/check-env-sync.sh`).
3. **Image rules**: derived `{NAME_PREFIX}-backend:latest` for now; the GHCR merge switches them to registry paths (one-line change + `@version` bump).
4. **Guard home**: `backend/internal/config/guard.go`, called from `validate()`.
5. **SOPS/age**: out of core.
6. **Frontend container env**: still open. Its `process.env.*` reads (with `NEXT_PUBLIC_*` inlined at build time) need their own extractor before `check-compose-env.sh` can cover the frontend service.

Pending at merge (owner): graft the `KEY: ${KEY:-}` snippet that `check-compose-env.sh` prints into `x-backend-environment` and drop the matching `wire` lines from `deploy/env/compose-env.allow`; graft the `env` CI job onto the reworked `ci.yml`.
