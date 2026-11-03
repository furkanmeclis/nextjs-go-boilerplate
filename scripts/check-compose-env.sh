#!/usr/bin/env bash
# Every env var the Go backend reads must reach its containers in production,
# or carry a recorded decision in deploy/env/compose-env.allow. Otherwise a
# value set in Dokploy is silently dropped. Design: docs/design/env-core.md §8.
#
#   scripts/check-compose-env.sh [--compose FILE] [--allow FILE] [--src DIR]
#                                [--services "backend worker"]
#
# Read set: string-literal getEnv/getInt/getBool/getDuration/getFloat/getInt64/
#   os.Getenv/os.LookupEnv calls in non-test Go files under --src.
# Delivered set: each service's environment in `docker compose config`, resolved
#   with an env generated from deploy/env/schema (anchors and overrides applied).
# Needs docker compose and bash 4.3+.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
COMPOSE=$ROOT/compose.prod.yml
ALLOW=$ROOT/deploy/env/compose-env.allow
SRC=$ROOT/backend
SERVICES="backend worker"
while (($#)); do
  case $1 in
    --compose) COMPOSE=${2:?}; shift 2 ;;
    --allow) ALLOW=${2:?}; shift 2 ;;
    --src) SRC=${2:?}; shift 2 ;;
    --services) SERVICES=${2:?}; shift 2 ;;
    -h | --help) sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p}' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "check-compose-env: unknown argument $1" >&2; exit 2 ;;
  esac
done

die() { echo "check-compose-env: $*" >&2; exit 2; }
docker compose version >/dev/null 2>&1 || die "docker compose is required"
[[ -f $ALLOW ]] || die "allowlist not found: $ALLOW"

T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

# --- read set ----------------------------------------------------------------
grep -rhoE --include='*.go' --exclude='*_test.go' \
  '\b(getEnv|getInt|getInt64|getBool|getDuration|getFloat|os\.Getenv|os\.LookupEnv)\("[A-Za-z0-9_]+"' "$SRC" |
  sed -E 's/.*\("//; s/"$//' | sort -u >"$T/read"
[[ -s $T/read ]] || die "found no env reads under $SRC (pattern out of date?)"

# --- allowlist ---------------------------------------------------------------
declare -A DECISION=() REASON=()
n=0
while IFS= read -r line || [[ -n $line ]]; do
  n=$((n + 1))
  [[ $line =~ ^[[:space:]]*(#|$) ]] && continue
  [[ $line =~ ^([A-Z][A-Z0-9_]*)[[:space:]]*\|[[:space:]]*(omit|wire)[[:space:]]*\|[[:space:]]*(.+)$ ]] ||
    die "$ALLOW:$n: expected 'KEY | omit|wire | reason'"
  [[ -z ${DECISION[${BASH_REMATCH[1]}]+x} ]] || die "$ALLOW:$n: duplicate ${BASH_REMATCH[1]}"
  DECISION[${BASH_REMATCH[1]}]=${BASH_REMATCH[2]}
  REASON[${BASH_REMATCH[1]}]=${BASH_REMATCH[3]}
done <"$ALLOW"

# --- delivered set -------------------------------------------------------------
"$ROOT/scripts/prod-env.sh" init --file "$T/env" --yes --set APP_URL=https://app.example.com </dev/null 2>"$T/init.err" ||
  { cat "$T/init.err" >&2; die "could not generate an env from deploy/env/schema"; }
if ! docker compose --env-file "$T/env" -f "$COMPOSE" config >"$T/resolved.yml" 2>"$T/config.err"; then
  cat "$T/config.err" >&2
  die "docker compose config failed with the schema-generated env: compose probably requires a variable deploy/env/schema does not manage"
fi

service_env() { # service_env <service>: keys of its resolved environment
  awk -v svc="$1" '
    $0 == "  " svc ":" { in_svc = 1; next }
    in_svc && /^  [^ ]/ { in_svc = 0 }
    in_svc && /^    environment:/ { in_env = 1; next }
    in_env && /^    [^ ]/ { in_env = 0 }
    in_svc && in_env && /^      [A-Za-z_][A-Za-z0-9_]*:/ { sub(/^ +/, ""); sub(/:.*/, ""); print }
  ' "$T/resolved.yml" | sort -u
}

status=0
fail() { echo "check-compose-env: $*" >&2; status=1; }

declare -A DELIVERED_ANY=()
pending=()
for svc in $SERVICES; do
  service_env "$svc" >"$T/wired.$svc"
  [[ -s $T/wired.$svc ]] || { fail "service '$svc' has no environment in $COMPOSE (renamed? pass --services)"; continue; }
  while IFS= read -r key; do DELIVERED_ANY[$key]=1; done <"$T/wired.$svc"
  while IFS= read -r key; do
    case ${DECISION[$key]:-} in
      omit) ;;
      wire) [[ " ${pending[*]-} " == *" $key "* ]] || pending+=("$key") ;;
      *) fail "$svc: $key is read by the backend but not delivered by $COMPOSE (wire it into x-backend-environment, or record a decision in $ALLOW)" ;;
    esac
  done < <(comm -23 "$T/read" "$T/wired.$svc")
done

for key in "${!DECISION[@]}"; do
  if ! grep -qxF "$key" "$T/read"; then
    fail "stale allowlist entry: $key is no longer read by the backend; drop it from $ALLOW"
  elif [[ -n ${DELIVERED_ANY[$key]+x} ]]; then
    fail "stale allowlist entry: $key is now delivered; drop it from $ALLOW"
  fi
done

if ((${#pending[@]})); then
  echo "check-compose-env: warning: ${#pending[@]} read keys are not delivered yet (marked 'wire' in $ALLOW)." >&2
  echo "  Behaviour-neutral fix for x-backend-environment (an empty value keeps the Go default):" >&2
  printf '%s\n' "${pending[@]}" | sort | sed 's/.*/    &: ${&:-}/' >&2
fi

if ((status == 0)); then
  echo "check-compose-env: ok ($(wc -l <"$T/read" | tr -d ' ') backend env reads; services: $SERVICES)"
fi
exit "$status"
