#!/usr/bin/env bash
# Schema-driven production env: deploy/env/schema -> .env.server.
# Design: docs/design/env-core.md. Needs bash 4.3+ and openssl.
#
#   scripts/prod-env.sh init     [--set KEY=VALUE]... [--yes]   create .env.server
#   scripts/prod-env.sh upgrade  [--rotate K1,K2] [--yes]       migrate to the current schema
#   scripts/prod-env.sh check                                   exit 1 when the file drifts
#   scripts/prod-env.sh example                                 print .env.server.example
#
# Flags:
#   --file PATH      env file (default: .env.server at the repo root)
#   --schema PATH    schema (default: deploy/env/schema)
#   --set KEY=VALUE  answer an `ask:` key without prompting (init, or upgrade for a new key)
#   --rotate K1,K2   upgrade: regenerate these secrets (refuses @norotate keys)
#   --yes            no prompts: accept defaults, confirm --rotate
#
# init asks for APP_URL and IMAGE_REPO (and offers defaults for the rest), derives
# every URL and image and generates every secret. upgrade keeps every existing value: it applies schema
# renames/removals, fills only missing keys and re-stamps ENV_SCHEMA_VERSION,
# backing the old file up to <file>.bak. Secret values are never printed.
set -euo pipefail
shopt -s extglob

if ((BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 3))); then
  echo "prod-env: bash 4.3+ required (macOS: brew install bash)" >&2
  exit 2
fi
command -v openssl >/dev/null 2>&1 || { echo "prod-env: openssl not found" >&2; exit 2; }

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SCHEMA=$ROOT/deploy/env/schema
FILE=$ROOT/.env.server
DEV_EXAMPLE=$ROOT/.env.example
EXAMPLE_APP_URL=https://app.example.com
EXAMPLE_IMAGE_REPO=ghcr.io/example/app

die() { echo "prod-env: $*" >&2; exit 1; }
QUIET=0
info() { ((QUIET)) || echo "prod-env: $*" >&2; }

usage() { sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p}' "${BASH_SOURCE[0]}"; }

# --- arguments ---------------------------------------------------------------

CMD=${1:-}
(($#)) && shift
declare -A SETS=()
ASSUME_YES=0
ROTATE=""
while (($#)); do
  case $1 in
    --file) FILE=${2:?--file needs a path}; shift 2 ;;
    --schema) SCHEMA=${2:?--schema needs a path}; shift 2 ;;
    --set)
      [[ ${2:-} == *=* ]] || die "--set needs KEY=VALUE"
      SETS[${2%%=*}]=${2#*=}
      shift 2 ;;
    --rotate) ROTATE=${2:?--rotate needs KEY[,KEY]}; shift 2 ;;
    -y | --yes) ASSUME_YES=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

# --- schema ------------------------------------------------------------------

VERSION=""
KEYS=()       # managed keys, schema order
ITEMS=()      # render order: "k:KEY" or "c:<comment or blank line>"
MIGRATIONS=() # "<version> rename OLD NEW" / "<version> remove KEY -"
declare -A RULE=() NOROTATE=()

is_pseudo() { [[ $1 == APP_HOST || $1 == APP_SCHEME || $1 == APP_LABEL ]]; }

# check_refs <template> <where>: every {X} must be an earlier key or a pseudo key.
check_refs() {
  local tpl=$1 where=$2 name
  while [[ $tpl =~ \{([A-Za-z_][A-Za-z0-9_]*)\} ]]; do
    name=${BASH_REMATCH[1]}
    if is_pseudo "$name"; then
      [[ -n ${RULE[APP_URL]+x} ]] || die "$where: {$name} needs APP_URL earlier in the schema"
    else
      [[ -n ${RULE[$name]+x} ]] || die "$where: {$name} is not an earlier key"
    fi
    tpl=${tpl#*"{$name}"}
  done
}

parse_schema() {
  [[ -f $SCHEMA ]] || die "schema not found: $SCHEMA"
  local line n=0 key rule where parts vapid_pub=0 vapid_priv=0 m
  while IFS= read -r line || [[ -n $line ]]; do
    n=$((n + 1))
    line=${line%$'\r'}
    where="$SCHEMA:$n"
    case $line in
      '##'*) continue ;;
      '#'* | '') ITEMS+=("c:$line"); continue ;;
      @*)
        read -r -a parts <<<"$line"
        case ${parts[0]} in
          @version)
            [[ ${#parts[@]} -eq 2 && ${parts[1]} =~ ^[1-9][0-9]*$ ]] || die "$where: @version N"
            [[ -z $VERSION ]] || die "$where: duplicate @version"
            VERSION=${parts[1]} ;;
          @rename)
            [[ ${#parts[@]} -eq 4 && ${parts[3]} =~ ^[1-9][0-9]*$ ]] || die "$where: @rename OLD NEW VERSION"
            MIGRATIONS+=("${parts[3]} rename ${parts[1]} ${parts[2]}") ;;
          @remove)
            [[ ${#parts[@]} -eq 3 && ${parts[2]} =~ ^[1-9][0-9]*$ ]] || die "$where: @remove KEY VERSION"
            MIGRATIONS+=("${parts[2]} remove ${parts[1]} -") ;;
          @norotate)
            [[ ${#parts[@]} -eq 2 ]] || die "$where: @norotate KEY"
            NOROTATE[${parts[1]}]=1 ;;
          *) die "$where: unknown directive ${parts[0]}" ;;
        esac
        continue ;;
    esac
    [[ $line =~ ^([A-Z][A-Z0-9_]*)[[:space:]]*\|[[:space:]]*(.*)$ ]] || die "$where: expected 'KEY | rule'"
    key=${BASH_REMATCH[1]}
    rule=${BASH_REMATCH[2]}
    rule=${rule%"${rule##*[![:space:]]}"}
    [[ -z ${RULE[$key]+x} ]] || die "$where: duplicate key $key"
    [[ $key != ENV_SCHEMA_VERSION ]] || die "$where: ENV_SCHEMA_VERSION is reserved"
    case $rule in
      ask:*) ;; # defaults resolve leniently (may name later keys)
      derive:*) check_refs "${rule#derive:}" "$where" ;;
      const:*) ;;
      secret:*) [[ $rule =~ ^secret:(hex|b64|alnum):[1-9][0-9]*$ ]] || die "$where: secret:hex|b64|alnum:<n>" ;;
      vapid:public) vapid_pub=$((vapid_pub + 1)) ;;
      vapid:private) vapid_priv=$((vapid_priv + 1)) ;;
      empty) ;;
      *) die "$where: unknown rule '$rule'" ;;
    esac
    RULE[$key]=$rule
    KEYS+=("$key")
    ITEMS+=("k:$key")
  done <"$SCHEMA"

  [[ -n $VERSION ]] || die "$SCHEMA: missing @version"
  ((vapid_pub == vapid_priv && vapid_pub <= 1)) || die "$SCHEMA: need exactly one vapid:public and one vapid:private (or neither)"
  for key in "${!NOROTATE[@]}"; do
    [[ -n ${RULE[$key]+x} ]] || die "$SCHEMA: @norotate $key is not a managed key"
  done
  if ((${#MIGRATIONS[@]})); then
    for m in "${MIGRATIONS[@]}"; do
      ((${m%% *} <= VERSION)) || die "$SCHEMA: migration '$m' is newer than @version $VERSION"
    done
  fi
}

vapid_key() { # vapid_key public|private -> REPLY = key with that rule
  local key
  for key in "${KEYS[@]}"; do
    [[ ${RULE[$key]} == "vapid:$1" ]] && { REPLY=$key; return 0; }
  done
  return 1
}

# --- env files ---------------------------------------------------------------

LINES=()          # file lines, verbatim
declare -A VAL=() # KEY -> unquoted value (last occurrence wins, like compose)

env_key() { # env_key <line> -> REPLY_KEY, REPLY_VAL (raw)
  [[ $1 =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || return 1
  REPLY_KEY=${BASH_REMATCH[2]}
  REPLY_VAL=${BASH_REMATCH[3]}
}

unquote() { # unquote <raw> -> REPLY
  REPLY=$1
  if [[ $REPLY =~ ^\"(.*)\"$ || $REPLY =~ ^\'(.*)\'$ ]]; then REPLY=${BASH_REMATCH[1]}; fi
}

read_env() {
  LINES=()
  VAL=()
  local line
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%$'\r'}
    LINES+=("$line")
    if env_key "$line"; then
      unquote "$REPLY_VAL"
      VAL[$REPLY_KEY]=$REPLY
    fi
  done <"$1"
}

declare -A IDX=() # KEY -> index of its last active line in LINES
index_lines() {
  IDX=()
  local i
  for i in "${!LINES[@]}"; do
    env_key "${LINES[$i]}" && IDX[$REPLY_KEY]=$i
  done
  return 0
}

# --- values ------------------------------------------------------------------

split_url() { # split_url <url> -> URL_SCHEME URL_HOST URL_LABEL
  [[ $1 =~ ^(https?)://([A-Za-z0-9.-]+)(:[0-9]+)?$ ]] || return 1
  URL_SCHEME=${BASH_REMATCH[1]}
  URL_HOST=${BASH_REMATCH[2],,}
  local host=$URL_HOST label
  [[ $host == www.* ]] && host=${host#www.}
  label=${host%%.*}
  # Same rule as NAME_PREFIX (check_value), so the default always validates.
  [[ $host =~ ^[0-9.]+$ || ! $label =~ ^[a-z][a-z0-9-]{1,39}$ ]] && label=app
  URL_LABEL=$label
}

lookup() { # lookup <name> -> REPLY; 1 when unknown
  case $1 in
    APP_HOST | APP_SCHEME | APP_LABEL)
      [[ -n ${VAL[APP_URL]:-} ]] && split_url "${VAL[APP_URL]}" || return 1
      case $1 in
        APP_HOST) REPLY=$URL_HOST ;;
        APP_SCHEME) REPLY=$URL_SCHEME ;;
        APP_LABEL) REPLY=$URL_LABEL ;;
      esac ;;
    *)
      [[ -n ${VAL[$1]:-} ]] || return 1
      REPLY=${VAL[$1]} ;;
  esac
}

render() { # render <template> -> RENDERED; 1 when a reference has no value
  local tpl=$1 out="" name
  while [[ $tpl =~ \{([A-Za-z_][A-Za-z0-9_]*)\} ]]; do
    name=${BASH_REMATCH[1]}
    lookup "$name" || return 1
    out+=${tpl%%"{$name}"*}$REPLY
    tpl=${tpl#*"{$name}"}
  done
  RENDERED=$out$tpl
}

check_value() { # check_value <key> <value> -> ERR; 1 when invalid
  ERR=""
  if [[ $2 =~ [[:space:]\$\"\'\`\\] ]]; then
    ERR="no spaces, quotes, backslashes or \$ (compose would mangle them)"
    return 1
  fi
  case $1 in
    APP_URL) split_url "$2" || { ERR="must look like https://app.example.com (scheme + host, no path)"; return 1; } ;;
    NAME_PREFIX) [[ $2 =~ ^[a-z][a-z0-9-]{1,39}$ ]] || { ERR="lowercase letters, digits and '-', 2-40 chars, starting with a letter"; return 1; } ;;
    IMAGE_REPO) [[ $2 =~ ^[a-z0-9.-]+(:[0-9]+)?(/[a-z0-9._-]+)+$ ]] || { ERR="must look like ghcr.io/<owner>/<repo> (lowercase, no tag)"; return 1; } ;;
  esac
}

ask_value() { # ask_value <key> -> REPLY
  local key=$1 def="" val
  render "${RULE[$key]#ask:}" && def=$RENDERED
  if [[ -n ${SETS[$key]+x} ]]; then
    val=${SETS[$key]}
  elif ((!ASSUME_YES)) && [[ -t 0 ]]; then
    while :; do
      read -r -p "$key${def:+ [$def]}: " val
      val=${val:-$def}
      [[ $key == APP_URL ]] && val=${val%/}
      [[ -n $val ]] && check_value "$key" "$val" && break
      echo "  ${ERR:-a value is required}" >&2
    done
  elif [[ -n $def ]]; then
    val=$def
    info "$key = $def (default)"
  else
    die "$key is required: pass --set $key=VALUE (or run interactively)"
  fi
  [[ $key == APP_URL ]] && val=${val%/}
  [[ -n $val ]] || die "$key must not be empty"
  check_value "$key" "$val" || die "$key: $ERR"
  [[ $key != APP_URL || $URL_SCHEME == https ]] || info "warning: APP_URL is not https"
  REPLY=$val
}

gen_secret() { # gen_secret secret:<kind>:<n> -> REPLY
  local kind n out=""
  kind=${1#secret:}
  n=${kind#*:}
  kind=${kind%%:*}
  case $kind in
    hex) out=$(openssl rand -hex "$n") ;;
    b64) out=$(openssl rand -base64 "$n" | tr -d '\n') ;;
    alnum)
      while ((${#out} < n)); do
        out+=$(openssl rand -base64 $((n * 2)) | tr -dc 'A-Za-z0-9')
      done
      out=${out:0:n} ;;
  esac
  [[ -n $out ]] || die "openssl failed to generate a secret"
  REPLY=$out
}

hex_b64url() { # hex_b64url <hex> -> stdout
  printf '%b' "$(sed 's/../\\x&/g' <<<"$1")" | openssl base64 -A | tr '+/' '-_' | tr -d '='
}

VAPID_PUB=""
VAPID_PRIV=""
gen_vapid() { # P-256 key pair in the web-push format (base64url, no padding)
  local text priv pub
  text=$(openssl ecparam -name prime256v1 -genkey -noout 2>/dev/null | openssl ec -text -noout 2>/dev/null) ||
    die "openssl failed to generate a VAPID key"
  priv=$(awk '/^priv:/{f=1;next} /^[^[:space:]]/{f=0} f' <<<"$text" | tr -d ' :\t\n')
  pub=$(awk '/^pub:/{f=1;next} /^[^[:space:]]/{f=0} f' <<<"$text" | tr -d ' :\t\n')
  while ((${#priv} < 64)); do priv=0$priv; done
  priv=${priv: -64}
  [[ ${#pub} -eq 130 && $pub == 04* ]] || die "unexpected VAPID public key from openssl"
  VAPID_PRIV=$(hex_b64url "$priv")
  VAPID_PUB=$(hex_b64url "$pub")
}

vapid_half() { # vapid_half <key> public|private -> REPLY
  local other=private
  [[ $2 == private ]] && other=public
  vapid_key "$other"
  if [[ -z $VAPID_PUB && -n ${VAL[$REPLY]:-} ]]; then
    die "$1 is missing but $REPLY is set: VAPID keys are a pair, set both or clear both"
  fi
  [[ -n $VAPID_PUB ]] || gen_vapid
  if [[ $2 == public ]]; then REPLY=$VAPID_PUB; else REPLY=$VAPID_PRIV; fi
}

resolve() { # resolve <key>: compute a missing value into VAL and REPLY
  local key=$1 rule=${RULE[$1]}
  case $rule in
    ask:*) ask_value "$key" ;;
    derive:*)
      render "${rule#derive:}" || die "$key: cannot derive '${rule#derive:}' (a referenced key is empty)"
      REPLY=$RENDERED ;;
    const:*) REPLY=${rule#const:} ;;
    secret:*) gen_secret "$rule" ;;
    vapid:*) vapid_half "$key" "${rule#vapid:}" ;;
    empty) REPLY="" ;;
  esac
  VAL[$key]=$REPLY
}

# Absent, or empty for any rule but `empty` (a blank secret/URL is never valid,
# and legacy files copied from .env.example carry blank VAPID keys).
is_missing() {
  [[ -n ${VAL[$1]+x} ]] || return 0
  [[ ${RULE[$1]} != empty && -z ${VAL[$1]} ]]
}

# --- output ------------------------------------------------------------------

TMP=""
trap '[[ -z $TMP ]] || rm -f "$TMP"' EXIT

write_lines() { # write_lines <path>: LINES -> path, atomically, mode 600
  TMP=$(mktemp "$1.tmp.XXXXXX")
  chmod 600 "$TMP"
  printf '%s\n' "${LINES[@]}" >"$TMP"
  mv -f "$TMP" "$1"
  TMP=""
}

secret_note() { # secret_note <rule> -> REPLY
  case $1 in
    secret:*)
      local kind=${1#secret:} unit=bytes
      [[ ${kind%%:*} == alnum ]] && unit=chars
      REPLY="# secret: generated by init (${kind%%:*}, ${kind#*:} $unit)" ;;
    vapid:*) REPLY="# VAPID ${1#vapid:} key: generated by init with its pair" ;;
  esac
}

# build_lines real|example: header + schema items -> LINES
build_lines() {
  local mode=$1 item key blank=1
  LINES=()
  if [[ $mode == real ]]; then
    LINES+=("# Production env, generated by scripts/prod-env.sh from deploy/env/schema."
      "# Secret: never commit it. Keep values stable; migrate with: scripts/prod-env.sh upgrade")
  else
    LINES+=("# GENERATED from deploy/env/schema by \`scripts/prod-env.sh example\`. Do not edit by hand."
      "# Production key list. Create the real file (asks for APP_URL and IMAGE_REPO, generates every secret):"
      "#   scripts/prod-env.sh init      then: make prod-config && make prod-up"
      "# Existing deployment: scripts/prod-env.sh upgrade   verify: scripts/prod-env.sh check"
      "# Values are examples for $EXAMPLE_APP_URL and $EXAMPLE_IMAGE_REPO; secrets stay blank on purpose."
      "# Keys not listed keep their compose.prod.yml defaults (all documented in .env.example).")
  fi
  LINES+=("ENV_SCHEMA_VERSION=$VERSION" "")
  for item in "${ITEMS[@]}"; do
    case $item in
      c:)
        ((blank)) || LINES+=("")
        blank=1 ;;
      c:*)
        LINES+=("${item#c:}")
        blank=0 ;;
      k:*)
        key=${item#k:}
        if [[ $mode == example && ${RULE[$key]} == @(secret|vapid):* ]]; then
          secret_note "${RULE[$key]}"
          LINES+=("$REPLY")
        fi
        LINES+=("$key=${VAL[$key]}")
        blank=0 ;;
    esac
  done
  ((blank)) && unset 'LINES[${#LINES[@]}-1]'
  return 0
}

# --- commands ----------------------------------------------------------------

cmd_init() {
  [[ ! -e $FILE ]] || die "$FILE already exists; use upgrade (it keeps every value)"
  local key
  VAL=()
  for key in "${KEYS[@]}"; do resolve "$key"; done
  build_lines real
  write_lines "$FILE"
  info "wrote $FILE (schema v$VERSION, ${#KEYS[@]} keys, secrets generated)"
  info "next: fill SMTP_* if you send mail, then make prod-config && make prod-up"
}

cmd_example() {
  local key
  QUIET=1
  ASSUME_YES=1
  [[ -n ${SETS[APP_URL]+x} ]] || SETS[APP_URL]=$EXAMPLE_APP_URL
  [[ -n ${SETS[IMAGE_REPO]+x} ]] || SETS[IMAGE_REPO]=$EXAMPLE_IMAGE_REPO
  VAL=()
  for key in "${KEYS[@]}"; do
    case ${RULE[$key]} in
      secret:* | vapid:*) VAL[$key]="" ;;
      *) resolve "$key" ;;
    esac
  done
  build_lines example
  printf '%s\n' "${LINES[@]}"
}

CHANGES=()
APPEND=()

set_line() { # set_line <key> <value>: replace the key's line in place, else append
  if [[ -n ${IDX[$1]+x} ]]; then
    LINES[${IDX[$1]}]="$1=$2"
  else
    APPEND+=("$1=$2")
  fi
  VAL[$1]=$2
}

apply_migrations() { # apply_migrations <from version>
  ((${#MIGRATIONS[@]})) || return 0
  local from=$1 ver op a b i
  while read -r ver op a b; do
    ((ver > from)) || continue
    case $op in
      rename)
        [[ -n ${VAL[$a]+x} ]] || continue
        for i in "${!LINES[@]}"; do
          env_key "${LINES[$i]}" && [[ $REPLY_KEY == "$a" ]] || continue
          if [[ -n ${VAL[$b]+x} ]]; then
            LINES[$i]="# renamed@$ver to $b (already set, kept that): ${LINES[$i]}"
          else
            LINES[$i]="$b=$REPLY_VAL"
          fi
        done
        [[ -n ${VAL[$b]+x} ]] || VAL[$b]=${VAL[$a]}
        unset 'VAL[$a]'
        CHANGES+=("renamed $a -> $b") ;;
      remove)
        [[ -n ${VAL[$a]+x} ]] || continue
        for i in "${!LINES[@]}"; do
          env_key "${LINES[$i]}" && [[ $REPLY_KEY == "$a" ]] || continue
          LINES[$i]="# removed@$ver: ${LINES[$i]}"
        done
        unset 'VAL[$a]'
        CHANGES+=("removed $a") ;;
    esac
  done < <(printf '%s\n' "${MIGRATIONS[@]}" | sort -s -n -k1,1)
}

confirm_rotate() {
  local keys=$1 answer
  cat >&2 <<EOF
prod-env: about to ROTATE: $keys
  Everything made with the old value stops working: JWT/AUTH secrets sign every
  user out, CENTRIFUGO keys drop realtime connections, VAPID keys drop every push
  subscription, REDIS/S3/MEILI credentials must match the restarted services.
  Restart the whole stack afterwards (make prod-up). The old file is kept as .bak.
EOF
  ((ASSUME_YES)) && return 0
  [[ -t 0 ]] || die "pass --yes to rotate non-interactively"
  read -r -p "Type ROTATE to continue: " answer
  [[ $answer == ROTATE ]] || die "aborted, nothing changed"
}

do_rotate() {
  [[ -n $ROTATE ]] || return 0
  local key keys vapid=0
  IFS=, read -r -a keys <<<"$ROTATE"
  for key in "${keys[@]}"; do
    [[ -n ${RULE[$key]+x} ]] || die "--rotate $key: not a key in $SCHEMA"
    [[ -z ${NOROTATE[$key]+x} ]] ||
      die "--rotate $key: refused, the schema marks it @norotate (rotating it destroys data; see the note above it in $SCHEMA)"
    [[ ${RULE[$key]} == @(secret|vapid):* ]] || die "--rotate $key: only secret and vapid keys can be rotated"
    [[ -n ${VAL[$key]:-} ]] || die "--rotate $key: not set yet (upgrade fills it without --rotate)"
  done
  confirm_rotate "${keys[*]}"
  for key in "${keys[@]}"; do
    if [[ ${RULE[$key]} == vapid:* ]]; then
      ((vapid)) && continue
      vapid=1
      gen_vapid
      vapid_key public && set_line "$REPLY" "$VAPID_PUB" && CHANGES+=("rotated $REPLY")
      vapid_key private && set_line "$REPLY" "$VAPID_PRIV" && CHANGES+=("rotated $REPLY")
    else
      gen_secret "${RULE[$key]}"
      set_line "$key" "$REPLY"
      CHANGES+=("rotated $key")
    fi
  done
}

file_version() { # -> REPLY
  REPLY=${VAL[ENV_SCHEMA_VERSION]:-0}
  [[ $REPLY =~ ^[0-9]+$ ]] || die "$FILE: ENV_SCHEMA_VERSION=$REPLY is not a number"
}

cmd_upgrade() {
  [[ -f $FILE ]] || die "$FILE not found; run init first"
  read_env "$FILE"
  file_version
  local from=$REPLY key i stamped=0
  ((from <= VERSION)) || die "$FILE is at schema v$from, newer than $SCHEMA (v$VERSION): wrong checkout?"

  apply_migrations "$from"
  index_lines
  do_rotate
  for key in "${KEYS[@]}"; do
    is_missing "$key" || continue
    resolve "$key"
    set_line "$key" "$REPLY"
    CHANGES+=("added $key")
  done

  if ((${#CHANGES[@]} == 0 && from == VERSION)); then
    info "$FILE is up to date (schema v$VERSION)"
    return 0
  fi
  if ((${#APPEND[@]})); then
    LINES+=("" "# --- added by scripts/prod-env.sh upgrade (schema v$VERSION) ---" "${APPEND[@]}")
  fi
  for i in "${!LINES[@]}"; do
    env_key "${LINES[$i]}" && [[ $REPLY_KEY == ENV_SCHEMA_VERSION ]] || continue
    LINES[$i]="ENV_SCHEMA_VERSION=$VERSION"
    stamped=1
  done
  ((stamped)) || LINES=("ENV_SCHEMA_VERSION=$VERSION" "${LINES[@]}")

  cp -p "$FILE" "$FILE.bak"
  chmod 600 "$FILE.bak"
  write_lines "$FILE"
  info "upgraded $FILE: schema v$from -> v$VERSION (backup: $FILE.bak)"
  if ((${#CHANGES[@]})); then
    for key in "${CHANGES[@]}"; do info "  $key"; done
  fi
}

cmd_check() {
  [[ -f $FILE ]] || die "$FILE not found"
  read_env "$FILE"
  file_version
  local from=$REPLY key ver op a b problems=()
  declare -A dev=()

  ((from == VERSION)) || problems+=("schema version: file is v$from, schema is v$VERSION (run: scripts/prod-env.sh upgrade)")
  for key in "${KEYS[@]}"; do
    if [[ -z ${VAL[$key]+x} ]]; then
      problems+=("missing: $key")
    elif is_missing "$key"; then
      problems+=("empty: $key")
    elif [[ ${RULE[$key]} == ask:* ]] && ! check_value "$key" "${VAL[$key]}"; then
      problems+=("invalid: $key ($ERR)")
    fi
  done
  if ((${#MIGRATIONS[@]})); then
    while read -r ver op a b; do
      [[ -n ${VAL[$a]+x} ]] || continue
      [[ $op == rename ]] && problems+=("renamed: $a is now $b (schema v$ver)")
      [[ $op == remove ]] && problems+=("removed: $a is no longer used (schema v$ver)")
    done < <(printf '%s\n' "${MIGRATIONS[@]}" | sort -s -n -k1,1)
  fi
  # Pre-v2 files carry locally built tags (<prefix>-backend:latest); compose
  # pulls them with pull_policy: always and fails on the first deploy.
  if [[ ${VAL[IMAGE_PULL_POLICY]:-always} == always ]]; then
    local svc
    for key in BACKEND_IMAGE FRONTEND_IMAGE; do
      svc=${key%%_*}
      [[ -n ${VAL[$key]:-} && ${VAL[$key]} != */* ]] &&
        problems+=("unpullable: $key=${VAL[$key]} has no registry path and IMAGE_PULL_POLICY=always (use ${VAL[IMAGE_REPO]:-ghcr.io/<owner>/<repo>}-${svc,,}:main)")
    done
  fi
  if [[ -f $DEV_EXAMPLE && $FILE != "$DEV_EXAMPLE" ]]; then
    local line
    while IFS= read -r line || [[ -n $line ]]; do
      env_key "$line" || continue
      unquote "$REPLY_VAL"
      dev[$REPLY_KEY]=$REPLY
    done <"$DEV_EXAMPLE"
    for key in "${KEYS[@]}"; do
      [[ ${RULE[$key]} == @(secret|vapid):* && -n ${VAL[$key]:-} && ${VAL[$key]} == "${dev[$key]:-}" ]] &&
        problems+=("insecure: $key still has the .env.example development value")
    done
  fi

  if ((${#problems[@]})); then
    echo "prod-env: $FILE does not match $SCHEMA (v$VERSION):" >&2
    printf '  %s\n' "${problems[@]}" >&2
    exit 1
  fi
  info "ok: $FILE matches schema v$VERSION (${#KEYS[@]} keys)"
}

case $CMD in
  init) parse_schema; cmd_init ;;
  upgrade) parse_schema; cmd_upgrade ;;
  check) parse_schema; cmd_check ;;
  example) parse_schema; cmd_example ;;
  -h | --help | help | "") usage; [[ -n $CMD ]] ;;
  *) die "unknown command: $CMD (init | upgrade | check | example)" ;;
esac
