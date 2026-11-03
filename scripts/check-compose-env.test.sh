#!/usr/bin/env bash
# Tests for scripts/check-compose-env.sh. Run: scripts/check-compose-env.test.sh
# Needs docker compose. Works on temp copies; the repo is not modified.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CHK=$ROOT/scripts/check-compose-env.sh
ALLOW=$ROOT/deploy/env/compose-env.allow
T=$(mktemp -d)
trap 'rm -rf "$T"; rm -f "$ROOT"/.compose-env-test.*' EXIT

if ! docker compose version >/dev/null 2>&1; then
  echo "skip - docker compose not available"
  exit 0
fi

pass=0
ok() { pass=$((pass + 1)); echo "ok  - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
# expect_fail <grep pattern> <args...>: the check must exit 1 and print the pattern.
expect_fail() {
  local pattern=$1 code=0
  shift
  "$CHK" "$@" >"$T/out" 2>&1 || code=$?
  [[ $code == 1 ]] || { cat "$T/out" >&2; fail "expected exit 1, got $code ($*)"; }
  grep -qE -- "$pattern" "$T/out" || { cat "$T/out" >&2; fail "output lacks /$pattern/"; }
}
# A compose.prod.yml copy beside the real deploy/ dir (compose resolves bind mounts).
compose_copy() { cp "$ROOT/compose.prod.yml" "$ROOT/.compose-env-test.$1.yml"; echo "$ROOT/.compose-env-test.$1.yml"; }

"$CHK" >"$T/out" 2>&1 || { cat "$T/out" >&2; fail "check fails on the repo"; }
ok "passes on the repository"

for key in MEILI_MASTER_KEY S3_REGION; do # S3_REGION: names with digits must be parsed
  C=$(compose_copy "drop-$key")
  sed -E "/^  $key: /d" "$ROOT/compose.prod.yml" >"$C"
  expect_fail "(backend|worker): $key is read by the backend but not delivered" --compose "$C"
done
ok "a read key dropped from x-backend-environment fails (incl. names with digits)"

mkdir -p "$T/src"
cp -R "$ROOT/backend/internal/config" "$T/src/"
printf 'package config\n\nvar _ = getInt("NEW_KNOB", 1)\n' >"$T/src/config/knob.go"
printf 'package config\n\nvar _ = getEnv("TEST_ONLY_KEY", "")\n' >"$T/src/config/knob_test.go"
expect_fail 'NEW_KNOB is read by the backend' --src "$T/src"
! grep -q TEST_ONLY_KEY "$T/out" || fail "reads in _test.go files must be ignored"
ok "a new backend env read without a decision fails; test files are ignored"

cp "$ALLOW" "$T/allow.stale"
echo 'GONE_KEY | omit | removed long ago' >>"$T/allow.stale"
expect_fail 'stale allowlist entry: GONE_KEY is no longer read' --allow "$T/allow.stale"
ok "an allowlist entry for a key the backend no longer reads fails"

# 'wire' keys pass with a warning carrying a KEY: ${KEY:-} snippet. Grafting
# that snippet makes them delivered (stale entries); dropping the entries passes
# with no warning. Built on a copy with two tunables removed, so it does not
# depend on the repo having 'wire' keys left.
C=$(compose_copy unwired)
sed -E '/^  (JWT_ACCESS_TTL|HTTP_READ_TIMEOUT): /d' "$ROOT/compose.prod.yml" >"$C"
cp "$ALLOW" "$T/allow.wire"
printf '%s\n' 'JWT_ACCESS_TTL | wire | test' 'HTTP_READ_TIMEOUT | wire | test' >>"$T/allow.wire"
"$CHK" --compose "$C" --allow "$T/allow.wire" 2>"$T/warn" >/dev/null || { cat "$T/warn" >&2; fail "'wire' keys should only warn"; }
grep -E '^    [A-Z0-9_]+: \$\{[A-Z0-9_]+:-\}$' "$T/warn" | sed 's/^  //' >"$T/snippet" || true
[[ $(wc -l <"$T/snippet") -eq 2 ]] || { cat "$T/warn" >&2; fail "expected a two-key snippet in the warning"; }
G=$(compose_copy graft)
awk -v snip="$T/snippet" '{ print } /^x-backend-environment: &backend-environment$/ { while ((getline l < snip) > 0) print l }' \
  "$C" >"$G"
expect_fail 'stale allowlist entry: .* is now delivered' --compose "$G" --allow "$T/allow.wire"
! grep -q 'not delivered by' "$T/out" || fail "graft introduced a missing key"
"$CHK" --compose "$G" >"$T/out" 2>&1 || { cat "$T/out" >&2; fail "grafted compose + trimmed allowlist should pass"; }
! grep -q warning "$T/out" || fail "no warning expected after the graft"
ok "'wire' keys warn with a KEY: \${KEY:-} snippet that grafts cleanly and clears the warning"

# Wired tunables are behaviour-neutral: empty values resolve to the Go defaults.
C=$ROOT/compose.prod.yml
"$ROOT/scripts/prod-env.sh" init --file "$T/env" --yes --set APP_URL=https://app.example.com </dev/null 2>/dev/null
docker compose --env-file "$T/env" -f "$C" config >"$T/resolved" 2>&1 || { cat "$T/resolved" >&2; fail "compose config on compose.prod.yml"; }
grep -q 'JWT_ACCESS_TTL: ""' "$T/resolved" || fail "wired tunables should resolve to an empty value when unset"
ok "wired tunables resolve to empty when unset (config.go then uses its default)"

expect_fail "service 'api' has no environment" --services "backend api"
ok "an unknown service fails instead of passing silently"

echo 'NOT A VALID LINE' >"$T/allow.bad"
code=0
"$CHK" --allow "$T/allow.bad" >/dev/null 2>&1 || code=$?
[[ $code == 2 ]] || fail "a malformed allowlist should exit 2 (got $code)"
ok "a malformed allowlist is rejected"

echo "all $pass checks passed"
