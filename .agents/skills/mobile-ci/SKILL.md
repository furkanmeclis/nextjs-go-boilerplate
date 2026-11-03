---
name: mobile-ci
description: >-
  CI and EAS release rules for an Expo app under mobile/: committed, pinned
  OpenAPI client types, the "api:generate is up to date" check, and why a
  backend/OpenAPI change never ships a mobile release. Use when adding mobile/,
  touching its API client generation, or debugging a mobile release trigger.
---

# Mobile CI / release

The boilerplate has no `mobile/` yet. The `mobile` and `mobile-release` jobs in
`.github/workflows/ci.yml` skip green until `mobile/package.json` exists.

## Contract a mobile/ app must follow

`mobile/package.json` needs these scripts: `lint`, `typecheck`, optionally `test`, and:

```json
"scripts": {
  "api:generate": "openapi-typescript ../backend/docs/openapi.yaml -o src/core/api/schema.d.ts"
},
"devDependencies": {
  "openapi-typescript": "7.9.1"
}
```

(`7.9.1` above is only an example. Pin whatever exact release you install: `npm i -D -E openapi-typescript`.)

- **Pin `openapi-typescript` to an exact version** (no `^`/`~`). CI fails otherwise. Bump it on purpose, in its own commit, with the regenerated file.
- **Commit `mobile/src/core/api/schema.d.ts`**. CI runs `npm run api:generate`, then `git diff --exit-code`. Stale types fail the `mobile` job.
- Commit `mobile/package-lock.json` (CI uses `npm ci`).
- For EAS: set `expo.extra.eas.projectId` in `mobile/app.json` (`eas init`) and the `EXPO_TOKEN` repo secret. Without either one, `mobile-release` skips green with a summary note.

## Why backend changes don't release the app

Forks used to map `backend/docs/openapi.yaml → mobile`. Any backend edit that regenerated the spec then shipped a mobile release, even when nothing the app consumes changed.

Now the release trigger is **`mobile/**` only** (the `mobile_release` output of the `changes` job). `openapi.yaml` never feeds it, and neither does the "full" run that a `ci.yml` edit triggers. `openapi-typescript` output is deterministic, so:

| Backend change | `schema.d.ts` after `api:generate` | Mobile release? |
|---|---|---|
| Internal or non-API edit, spec reformatted | identical, no diff | no |
| Real API surface change, types regenerated and committed | diff under `mobile/**` | yes |
| Real API change, types **not** regenerated | — | `mobile` job fails ("api:generate is up to date") |

The `mobile` **check** also runs on backend changes, so drift is caught. Only a committed `mobile/**` diff can trigger the **release**. Doc-only `mobile/**/*.md` edits don't release either.

## Jobs

- `mobile`: lint, typecheck, test, generator-pinned check, api:generate up-to-date check. Runs on a `mobile/**` or `backend/**` change, or a full run.
- `mobile-release`: runs on a green `main` push with a real `mobile/**` change. Does `eas build --platform all --profile production --non-interactive --no-wait`. Change the profile or command there if the app uses `eas update` (OTA).
