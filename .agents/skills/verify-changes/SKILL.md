---
name: verify-changes
description: >-
  Run the same checks CI runs (backend vet/test/lint, OpenAPI lint, frontend
  typecheck/lint/build) before committing or opening a PR. Use before saying a
  change is done.
---

# Verify changes

CI (`.github/workflows/ci.yml`) runs on every branch push. Run the matching subset locally first.

## Backend (any `backend/**` change)

```bash
cd backend
gofmt -l .            # must print nothing
go build ./...
go vet ./...
go test ./...
make openapi-lint     # when backend/docs/openapi.yaml changed
```

`make lint` runs golangci-lint (needs `make tools`).

## Frontend (any `frontend/**` change)

```bash
cd frontend
pnpm api:generate     # after OpenAPI changes; commit src/generated/api.d.ts
pnpm typecheck
pnpm lint
pnpm format:check
pnpm build            # slow; run before a PR
```

Stale `.next/types` after deleting a route: `rm -rf .next/types` and typecheck again.

## Database

New migration → round-trip it (see the `db-migration` skill).

## Prod compose

```bash
docker compose --env-file <filled env> -f compose.prod.yml config >/dev/null
```

`NAME_PREFIX` must be set; internal hosts stay `<service>.<app network>`.

## CI workflow changes

```bash
actionlint .github/workflows/ci.yml   # go install github.com/rhysd/actionlint/cmd/actionlint@latest
jq -e '.images | length > 0' deploy/images.json
```

Jobs skip by path (the `changes` job). `ci` is the single gate: a job passes when it succeeds or is skipped.

## Shipping

Push a feature branch: `ci` runs the path-gated jobs. When `auto-pr` is enabled (Actions "create PRs" setting or an `AUTO_PR_TOKEN`), it opens a PR into `main` and merges the tested commit. Put `[no-automerge]` in the head commit message to keep the PR open. Otherwise, merge by hand. A green `main` push publishes the GHCR images and cuts `v0.N`, and Dokploy deploys that tag.
