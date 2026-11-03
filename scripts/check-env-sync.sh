#!/usr/bin/env bash
# Keeps the env files in line with deploy/env/schema (docs/design/env-core.md):
#   1. .env.server.example is exactly `scripts/prod-env.sh example` output
#      (fix: scripts/prod-env.sh example > .env.server.example)
#   2. every schema key also has a local default in .env.example
#   3. scripts/prod-env.test.sh passes (skip with --no-tests)
# Run: make env-check
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
run_tests=1
[[ ${1:-} == --no-tests ]] && run_tests=0
status=0

generated=$(mktemp)
trap 'rm -f "$generated"' EXIT
scripts/prod-env.sh example >"$generated"
if ! diff -u --label .env.server.example --label "prod-env.sh example" .env.server.example "$generated"; then
  echo "check-env-sync: .env.server.example is out of date with deploy/env/schema." >&2
  echo "  fix: scripts/prod-env.sh example > .env.server.example" >&2
  status=1
fi

missing=$(comm -23 \
  <(sed -nE 's/^([A-Z][A-Z0-9_]*)[[:space:]]*\|.*/\1/p' deploy/env/schema | sort -u) \
  <(sed -nE 's/^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=.*/\2/p' .env.example | sort -u))
if [[ -n $missing ]]; then
  echo "check-env-sync: schema keys without a local default in .env.example:" >&2
  sed 's/^/  /' <<<"$missing" >&2
  status=1
fi

if ((run_tests)); then
  scripts/prod-env.test.sh >/dev/null || { echo "check-env-sync: scripts/prod-env.test.sh failed" >&2; status=1; }
fi

((status)) || echo "check-env-sync: ok"
exit "$status"
