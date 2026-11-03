#!/usr/bin/env bash
# Tests for scripts/prod-env.sh. Run: scripts/prod-env.test.sh
# Uses a temp dir; docker compose and node checks run only when those tools exist.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PE=$ROOT/scripts/prod-env.sh
SCHEMA=$ROOT/deploy/env/schema
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

pass=0
ok() { pass=$((pass + 1)); echo "ok  - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
val() { grep -E "^$1=" "$2" | tail -n1 | cut -d= -f2-; }
# Every KEY=VALUE except the given keys (to prove nothing else moved).
values_except() { local f=$1; shift; grep -E '^[A-Z_]+=' "$f" | grep -vE "^($(IFS='|'; echo "$*"))=" | sort; }

init() { "$PE" init --file "$1" --yes --set APP_URL=https://demo.example.com "${@:2}" </dev/null 2>/dev/null; }

# --- init --------------------------------------------------------------------
E=$T/env
init "$E"
[[ $(stat -c %a "$E" 2>/dev/null || stat -f %Lp "$E") == 600 ]] || fail "init: file mode is not 600"
ok "init writes a mode-600 file"

while IFS= read -r key; do
  grep -qE "^$key=" "$E" || fail "init: $key missing"
done < <(sed -nE 's/^([A-Z][A-Z0-9_]*)[[:space:]]*\|.*/\1/p' "$SCHEMA")
[[ $(val ENV_SCHEMA_VERSION "$E") == "$(sed -nE 's/^@version ([0-9]+)/\1/p' "$SCHEMA")" ]] || fail "init: version stamp"
ok "init writes every schema key and stamps ENV_SCHEMA_VERSION"

[[ $(val NAME_PREFIX "$E") == demo ]] || fail "NAME_PREFIX default from the host"
[[ $(val CENTRIFUGO_WS_URL "$E") == wss://realtime.demo.example.com/connection/websocket ]] || fail "CENTRIFUGO_WS_URL default"
[[ $(val CORS_ALLOWED_ORIGINS "$E") == https://demo.example.com ]] || fail "derive from APP_URL"
init "$T/short" --set APP_URL=https://x.io || fail "init with a one-letter host label"
[[ $(val NAME_PREFIX "$T/short") == app ]] || fail "NAME_PREFIX falls back to app"
ok "init derives NAME_PREFIX, URLs and origins from APP_URL (falls back to 'app')"

[[ $(val JWT_ACCESS_SECRET "$E") =~ ^[0-9a-f]{64}$ ]] || fail "hex:32 secret"
[[ $(val DB_PASSWORD "$E") =~ ^[A-Za-z0-9]{32}$ ]] || fail "alnum:32 secret"
[[ $(val APP_ENCRYPTION_KEY "$E" | openssl base64 -d -A | wc -c | tr -d ' ') == 32 ]] || fail "b64:32 decodes to 32 bytes"
[[ $(val JWT_ACCESS_SECRET "$E") != "$(val JWT_REFRESH_SECRET "$E")" ]] || fail "secrets are distinct"
ok "secrets have the requested format and length"

pub=$(val VAPID_PUBLIC_KEY "$E")
priv=$(val VAPID_PRIVATE_KEY "$E")
[[ ${#pub} -eq 87 && ${#priv} -eq 43 ]] || fail "VAPID key lengths ${#pub}/${#priv}"
if command -v node >/dev/null 2>&1; then
  node -e '
    const [pub, priv] = process.argv.slice(1);
    const e = require("crypto").createECDH("prime256v1");
    e.setPrivateKey(Buffer.from(priv, "base64url"));
    if (e.getPublicKey().toString("base64url") !== pub) process.exit(1);
  ' "$pub" "$priv" || fail "VAPID public key does not match the private key"
  ok "VAPID keys are a matching P-256 pair (web-push format)"
else
  ok "VAPID key lengths match the web-push format (node not found: pair check skipped)"
fi

if docker compose version >/dev/null 2>&1; then
  docker compose --env-file "$E" -f "$ROOT/compose.prod.yml" config -q ||
    fail "compose.prod.yml rejects the generated file"
  ok "compose.prod.yml accepts the generated file (docker compose config)"
fi

"$PE" check --file "$E" 2>/dev/null || fail "check fails on a fresh file"
ok "check passes on a fresh file"

if "$PE" init --file "$E" --yes --set APP_URL=https://x.example.com </dev/null 2>/dev/null; then
  fail "init overwrote an existing file"
fi
ok "init refuses to overwrite"

if "$PE" init --file "$T/nourl" --yes </dev/null 2>/dev/null; then fail "init without APP_URL"; fi
if init "$T/badurl" --set APP_URL=https://demo.example.com/path; then fail "APP_URL with a path"; fi
if init "$T/badprefix" --set NAME_PREFIX=Bad_Name; then fail "invalid NAME_PREFIX"; fi
ok "init rejects a missing/invalid APP_URL and an invalid NAME_PREFIX"

# --- upgrade: no-op --------------------------------------------------------------
cp "$E" "$T/before"
"$PE" upgrade --file "$E" --yes </dev/null 2>/dev/null
cmp -s "$E" "$T/before" || fail "no-op upgrade changed the file"
[[ ! -e $E.bak ]] || fail "no-op upgrade wrote a backup"
ok "upgrade on a current file is a no-op"

# --- upgrade across a schema bump ------------------------------------------------
S2=$T/schema.v2
sed -e 's/^@version 1$/@version 2/' \
  -e 's/^JWT_ACCESS_SECRET |/JWT_ACCESS_KEY |/' \
  -e '/^SMTP_PORT |/d' "$SCHEMA" >"$S2"
cat >>"$S2" <<'EOF'
@rename JWT_ACCESS_SECRET JWT_ACCESS_KEY 2
@remove SMTP_PORT 2
NEW_SECRET | secret:hex:16
NEW_URL | derive:{APP_URL}/new
EOF

if "$PE" check --file "$E" --schema "$S2" >"$T/check.out" 2>&1; then fail "check passed against a newer schema"; fi
grep -q 'file is v1, schema is v2' "$T/check.out" || fail "check: version gap not reported"
grep -q 'missing: NEW_SECRET' "$T/check.out" || fail "check: missing key not reported"
grep -q 'renamed: JWT_ACCESS_SECRET is now JWT_ACCESS_KEY' "$T/check.out" || fail "check: rename not reported"
grep -q 'removed: SMTP_PORT' "$T/check.out" || fail "check: removal not reported"
ok "check reports version gap, missing, renamed and removed keys (exit 1)"

old_jwt=$(val JWT_ACCESS_SECRET "$E")
"$PE" upgrade --file "$E" --schema "$S2" --yes </dev/null 2>/dev/null
[[ $(val ENV_SCHEMA_VERSION "$E") == 2 ]] || fail "upgrade: version not re-stamped"
[[ $(val JWT_ACCESS_KEY "$E") == "$old_jwt" ]] || fail "upgrade: rename lost the value"
! grep -qE '^JWT_ACCESS_SECRET=' "$E" || fail "upgrade: old key still active"
grep -qE '^# removed@2: SMTP_PORT=587$' "$E" || fail "upgrade: removed key not commented out"
[[ $(val NEW_SECRET "$E") =~ ^[0-9a-f]{32}$ ]] || fail "upgrade: new secret not generated"
[[ $(val NEW_URL "$E") == https://demo.example.com/new ]] || fail "upgrade: new derive"
cmp -s "$E.bak" "$T/before" || fail "upgrade: backup is not the previous file"
diff <(values_except "$T/before" JWT_ACCESS_SECRET SMTP_PORT ENV_SCHEMA_VERSION) \
  <(values_except "$E" JWT_ACCESS_KEY NEW_SECRET NEW_URL ENV_SCHEMA_VERSION) >/dev/null ||
  fail "upgrade: an existing value changed"
"$PE" check --file "$E" --schema "$S2" 2>/dev/null || fail "check fails after upgrade"
ok "upgrade moves renamed values, comments out removed keys, adds only new keys, keeps every secret, backs up"

cp "$E" "$T/v2"
"$PE" upgrade --file "$E" --schema "$S2" --yes </dev/null 2>/dev/null
cmp -s "$E" "$T/v2" || fail "second upgrade changed the file"
ok "upgrade is idempotent"

# --- rotation --------------------------------------------------------------------
R=$T/rot
init "$R"
cp "$R" "$T/rot.before"
for key in APP_ENCRYPTION_KEY DB_PASSWORD NAME_PREFIX NOPE; do
  if "$PE" upgrade --file "$R" --rotate "$key" --yes </dev/null 2>/dev/null; then fail "--rotate $key was allowed"; fi
done
cmp -s "$R" "$T/rot.before" || fail "refused rotation changed the file"
ok "--rotate refuses @norotate keys (APP_ENCRYPTION_KEY, DB_PASSWORD), non-secrets and unknown keys"

if "$PE" upgrade --file "$R" --rotate JWT_REFRESH_SECRET </dev/null 2>/dev/null; then fail "rotation without --yes and no tty"; fi
"$PE" upgrade --file "$R" --rotate JWT_REFRESH_SECRET,VAPID_PUBLIC_KEY --yes </dev/null 2>/dev/null
[[ $(val JWT_REFRESH_SECRET "$R") != "$(val JWT_REFRESH_SECRET "$T/rot.before")" ]] || fail "rotation did not change the secret"
[[ $(val VAPID_PRIVATE_KEY "$R") != "$(val VAPID_PRIVATE_KEY "$T/rot.before")" ]] || fail "VAPID rotation must rotate the pair"
diff <(values_except "$T/rot.before" JWT_REFRESH_SECRET VAPID_PUBLIC_KEY VAPID_PRIVATE_KEY) \
  <(values_except "$R" JWT_REFRESH_SECRET VAPID_PUBLIC_KEY VAPID_PRIVATE_KEY) >/dev/null ||
  fail "rotation touched other keys"
ok "--rotate needs confirmation, changes only the named secrets (VAPID as a pair)"

# --- legacy file (hand-made from .env.example, no version) ------------------------
L=$T/legacy
cp "$ROOT/.env.example" "$L"
sed -i.orig -e 's#^AUTH_URL=.*#AUTH_URL=https://legacy.example.com#' "$L"
if "$PE" check --file "$L" >"$T/legacy.out" 2>&1; then fail "check passed on a legacy file"; fi
grep -q 'file is v0' "$T/legacy.out" || fail "legacy: version gap"
grep -q 'insecure: JWT_ACCESS_SECRET' "$T/legacy.out" || fail "legacy: dev secret not flagged"
ok "check flags an unversioned file and .env.example development secrets"

"$PE" upgrade --file "$L" --yes </dev/null 2>/dev/null
[[ $(val APP_URL "$L") == https://legacy.example.com ]] || fail "legacy: APP_URL should default to AUTH_URL"
[[ -n $(val VAPID_PUBLIC_KEY "$L") ]] || fail "legacy: blank VAPID keys should be generated"
[[ $(val JWT_ACCESS_SECRET "$L") == "$(val JWT_ACCESS_SECRET "$ROOT/.env.example")" ]] || fail "legacy: upgrade must not touch existing secrets"
if "$PE" check --file "$L" >"$T/legacy2.out" 2>&1; then fail "legacy: dev secrets should still fail check"; fi
! grep -qE 'missing|empty|schema version' "$T/legacy2.out" || fail "legacy: upgrade left gaps"
ok "legacy upgrade fills gaps (APP_URL from AUTH_URL), keeps existing secrets; check still flags dev values"

# --- example -----------------------------------------------------------------------
"$PE" example >"$T/ex1"
"$PE" example >"$T/ex2"
cmp -s "$T/ex1" "$T/ex2" || fail "example is not deterministic"
grep -qE '^JWT_ACCESS_SECRET=$' "$T/ex1" || fail "example must leave secrets blank"
grep -qE '^APP_URL=https://app.example.com$' "$T/ex1" || fail "example APP_URL"
! grep -qE '^##' "$T/ex1" || fail "example leaks schema-only notes"
ok "example is deterministic, leaves secrets blank, drops ## notes"

# --- schema validation -------------------------------------------------------------
bad() { printf '%s\n' "$@" >"$T/bad"; ! "$PE" example --schema "$T/bad" >/dev/null 2>&1; }
bad 'APP_URL | ask:' || fail "schema without @version accepted"
bad '@version 1' 'A | derive:{B}' || fail "derive of an unknown key accepted"
bad '@version 1' 'A | const:x' 'A | const:y' || fail "duplicate key accepted"
bad '@version 1' 'A | secret:hex:0' || fail "bad secret length accepted"
bad '@version 1' 'A | vapid:public' || fail "half a VAPID pair accepted"
bad '@version 1' '@remove A 2' || fail "migration newer than @version accepted"
bad '@version 1' '@norotate A' || fail "@norotate of an unknown key accepted"
ok "invalid schemas are rejected"

echo "all $pass checks passed"
