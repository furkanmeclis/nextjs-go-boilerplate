# License core — design study

Status: **proposal**. A core feature for apps cloned from this boilerplate: an offline-verified license key, a remote suspend/resume switch for payment disputes, and a small health channel — all owned by the boilerplate author, with no customer data ever leaving the deployment. Default off; a deployment with no license configured behaves exactly as today.

## 0. Legal note (read first)

A remote suspend switch on software other people operate is an **enforcement mechanism, not a covert backdoor** — but only if it is disclosed. Every deployment's operator agreement must state, in plain words, that the software verifies a license and that the author can suspend it on breach (e.g. non-payment), that suspension never destroys data, and what health signals the author can read. With that clause this is ordinary license enforcement. Without it, it is a hidden backdoor and a legal liability. The clause is one paragraph; everything below assumes it exists.

## 1. Goal

- A license key held in the deployment's `.env`, verified **offline** — the running app never calls GitHub or any other host to check it.
- Only a key **signed by the author** is accepted; forged or edited keys are rejected.
- The author can, from their own control plane, **suspend** a deployment (API stops answering) and **resume** it, and read a few **health** metrics.
- Suspension is **reversible and non-destructive**: data is untouched, migrations do not run, nothing is deleted; the API simply refuses `/v1/*` with `503` and the frontend shows a maintenance page.
- No inbound control endpoint exists on the deployment, so **nothing is added to `openapi.yaml`** and `pnpm api:generate` sees nothing new. The deployment dials **out** to the author's control plane.
- No customer data ever crosses the channel — only license status and infrastructure health.
- Local development is recognised automatically (`APP_ENV=development`): no real license needed.

## 2. Decisions

| Question | Decision |
|---|---|
| Control channel direction | **Outbound.** The deployment opens a persistent TLS WebSocket to the author's control plane. No inbound port, works behind NAT/firewall, and leaves nothing to hide from OpenAPI. |
| Suspended behaviour | API refuses all `/v1/*` with `503 LICENSE_INACTIVE`; worker pauses job processing; the **frontend shows a maintenance page** ("service temporarily unavailable, contact your administrator"). Data untouched. |
| Signing key | A **dedicated Ed25519 license keypair**, separate from the author's GitHub SSH key. Private key lives only on the author's side; public key is embedded in the boilerplate at build time. |

## 3. Two core packages

### `platform/license` — the verifier (pure, no I/O)
- Reads `LICENSE_KEY` (the signed token) from env.
- Holds the **embedded Ed25519 public key** (`go:embed license_public_key.pem`).
- Verifies the token's signature offline and parses claims (§4). Pure function; no network, no DB.
- Exposes `State()`: `{Valid bool, Reason string, AppID string, Features []string, ExpiresAt *time.Time}`.
- `APP_ENV=development` → returns a synthetic valid dev license; `LICENSE_KEY` optional locally.

### `platform/licenseguard` — the control agent (the "encrypted" one)
- The only package that talks to the outside, and only **outbound**.
- Derives its own local secret from the existing `APP_ENCRYPTION_KEY` (via `platform/crypto`) to encrypt any state it caches on disk/Redis — so it "takes its own password locally" and config already refuses a default key outside development (`config.go` `APP_ENCRYPTION_KEY` guard).
- Opens a TLS WebSocket to `LICENSE_CONTROL_URL`, presents the license token as identity, and then only **receives signed commands** and **replies with health**.
- Verifies every command's Ed25519 signature with the same embedded public key, plus a nonce + timestamp window, so a captured command cannot be replayed.
- Writes the suspend flag to the DB (§5) so it survives restarts; never performs a destructive action — the protocol has no "delete" verb to begin with (§6).

## 4. License token

A compact signed envelope (PASETO v4.public or a signed JSON token — both Ed25519):

```
claims = {
  app:       "otopoly",            // which app this key is for
  deployment:"acme-prod",          // optional human label, shown in health
  iat:       <issued at>,
  exp:       <optional expiry>,    // NULL = no end
  features:  ["billing","ai"],     // optional per-deployment feature gates
  key_id:    "lic-2026-01"         // for rotation
}
signature = Ed25519(author_private_key, claims)
```

Verification is signature + `app` match + (if present) `exp` not passed. The author signs tokens with a small local CLI (§8); the private key never leaves the author's machine.

## 5. Runtime state (migration 000031)

```sql
-- 000031_license_runtime: single-row table, author-controlled suspend flag
license_runtime (
  id            SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  suspended     BOOLEAN NOT NULL DEFAULT false,
  reason        TEXT NULL,
  suspended_at  TIMESTAMPTZ NULL,
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
)
```

Why DB and not Redis: a restart must not silently un-suspend. Redis holds a short-TTL mirror for the hot path; the DB row is the source of truth. sqlc queries read `suspended`; the guard writes it on a verified `suspend`/`resume` command.

Effective state = `license.State().Valid` **AND NOT** `license_runtime.suspended` **AND** license not expired.

## 6. Control protocol (outbound WebSocket, author → deployment)

Three verbs, all signed by the author's key, all non-destructive:

| Verb | Deployment does | Returns |
|---|---|---|
| `health` | nothing | license status, version, uptime, DB/Redis up?, queue depth — **no customer data** |
| `suspend` | sets `license_runtime.suspended = true`, bumps Redis mirror | ack |
| `resume` | sets `suspended = false` | ack |

There is deliberately **no** verb that reads business data, runs SQL, deletes rows, or executes shell — the agent cannot be turned into a data tap even if the channel is compromised, because those code paths do not exist. The channel is opened only when `LICENSE_CONTROL_URL` is set; with it unset the deployment runs fully offline and can only be suspended by shipping a new (e.g. expired) `LICENSE_KEY`.

## 7. Enforcement

```
Browser (/t/acme or /)      BFF (/api/v1/*)              Go API
  request ───────────────▶ proxyToUpstream ───────────▶ license.Middleware
                             sees 503 LICENSE_INACTIVE    • effective state (§5), cached
                             → renders maintenance page    • refuse all /v1/* with 503 if inactive
                                                           handler… (only when active)
```

- `license.Middleware` sits at the top of the `/v1` chain in `internal/httpserver/server.go`, before auth. Inactive → `503 {code: "LICENSE_INACTIVE"}`, nothing else runs.
- Worker: a guard in `internal/queue/worker.go` skips/parks jobs while inactive (no data loss; jobs resume on resume).
- BFF: `bff-proxy.ts` maps upstream `503 LICENSE_INACTIVE` to a static maintenance page, so the browser never sees a raw error.
- Health/liveness probes used by the orchestrator (`/healthz`) stay up so the container is not killed while suspended.

## 8. Author side (separate, not shipped to clients)

- A **signing CLI**: `license sign --app otopoly --exp 2027-01-01 --key lic.ed25519` → prints a `LICENSE_KEY`.
- A minimal **control plane** service the author runs: accepts the outbound connections, lists deployments and their health, and sends signed `suspend`/`resume`. This lives in its own repo; the boilerplate ships only the client agent and the embedded public key.

## 9. Configuration

| Key | Default | Meaning |
|---|---|---|
| `LICENSE_KEY` | empty | Signed license token. Empty + non-dev = app runs but logs "unlicensed" (grace, see §10). |
| `LICENSE_CONTROL_URL` | empty | Author control plane WSS URL. Empty = no remote channel at all. |
| `LICENSE_ENFORCE` | `false` | Master switch: off = verify + log only (observe mode); on = actually gate `/v1`. |
| (build) `license_public_key.pem` | author's | Embedded at compile time; not an env var. |

`APP_ENCRYPTION_KEY` is reused by the guard and already must be non-default outside development.

## 10. Rollout stance

- Ships **off**. `LICENSE_ENFORCE=false` by default, so existing apps built from the boilerplate are unaffected until the author opts in per deployment.
- Observe mode first (verify + log, no gating) to prove a deployment's key and channel before enforcement is switched on — avoids locking out a paying customer by mistake.
- A short local grace on transient verify failures (clock skew, missing key at first boot) logs rather than suspends; only an explicit author `suspend` or a clearly invalid/expired key gates traffic.

## 11. Delivery plan (each phase one PR behind `LICENSE_ENFORCE`, main stays shippable)

| Phase | Scope | Done when |
|---|---|---|
| 1. Verifier | `platform/license`, embedded pubkey, token parse + Ed25519 verify, dev mode, `State()`, config keys, signing CLI, docs | Go tests cover valid/forged/expired/wrong-app/dev; no enforcement yet |
| 2. Enforcement | migration 000031 + sqlc, effective-state, `license.Middleware`, worker guard, BFF maintenance page | Suspended deployment refuses `/v1` with 503, shows maintenance page, data intact, resume restores |
| 3. Control agent | `platform/licenseguard` outbound WSS client, signed `health`/`suspend`/`resume`, nonce/replay guard, encrypted local state via `APP_ENCRYPTION_KEY` | Author suspends/resumes a running deployment and reads health over the channel; no customer data on the wire |
| 4. Control plane | Author-side server + deployment list/health UI (separate repo) | Author sees all deployments and acts on them |
| 5. Docs + setup | `boilerplate-first-setup` additions (how to get a key), operator-agreement clause template, key-rotation note | A new clone documents its license setup |

## 12. Open questions

1. Token format: PASETO v4.public (typed, hard to misuse) vs a minimal signed JSON — proposal: PASETO.
2. Should `LICENSE_KEY` absence in production be a hard stop or a logged grace? — proposal: logged grace until the author flips `LICENSE_ENFORCE`, then hard.
3. Key rotation: embed one current public key, or a small set so an old key keeps verifying during rollover? — proposal: a set keyed by `key_id`.
