# shellcheck shell=bash
# ============================================================================
# resources-lib.sh — shared engine for the data-driven seed / teardown scripts.
#
# Sourced by BOTH idp/sample-resources.sh (create) and
# idp/sample-resources.down.sh (delete) so the idp/resources/ JSON config is the
# single source of truth, the two scripts can never drift, and the shared
# infrastructure (logging, api_call, JSON lookups) lives in exactly one place.
#
# Requires (set by the sourcing script BEFORE this file is sourced):
#   - SCRIPT_DIR                    dir containing the resources/ tree
#   - API_BASE / AUTH_TOKEN / INSECURE   consumed by api_call
#
# Provides:
#   log_info/success/warning/error  stderr loggers
#   is_localhost / api_call         HTTP against the ThunderID management API
#   extract_first_id + get_*/list_* jq-based response parsing (see note below)
#   role_has_assignment             assignment-existence check
#   reg_set / reg_get / reg_require logical-key -> runtime-ID registry
#   resolve_secret                  {override,default} env-name -> value
#   scopeset_fragment               scopeSet name -> '"a", "b", ...' fragment
#   expand_agencies / load_config   config expansion + glob/merge -> $MERGED
#
# JSON parsing uses `jq` (a hard dependency of these scripts, checked in
# load_config) rather than sed/grep/cut. Every list endpoint returns a WRAPPED
# object (`{"users":[...]}`, `{"applications":[...]}`, ...), not a bare array,
# so the filters below target the specific wrapper key. `extract_first_id` uses
# a recursive "first id in document order" match — equivalent to the previous
# `grep -o '"id":"…"' | head -1` — so it is robust to bare vs. wrapped create
# responses. (NOTE: idp/bootstrap/02-admin-cli.sh runs inside the setup
# container, which has NO jq, so it keeps sed/grep/cut — it does not use this.)
#
# bash-3.2 safe: NO associative arrays (the dev machine runs bash 3.2.57). The
# registry uses dynamic scalar variables; jq arrays are iterated with
# here-strings (never pipes — a `... | while read` body runs in a subshell on
# bash 4+ and would lose registry writes, a bug that does NOT reproduce on 3.2).
# ============================================================================

# ----------------------------------------------------------------------------
# Logging — ALWAYS to stderr so helpers can echo IDs on stdout inside $( ... ).
# ----------------------------------------------------------------------------
log_info()    { printf '[INFO] %s\n'    "$*" >&2; }
log_success() { printf '[SUCCESS] %s\n' "$*" >&2; }
log_warning() { printf '[WARNING] %s\n' "$*" >&2; }
log_error()   { printf '[ERROR] %s\n'   "$*" >&2; }

# ----------------------------------------------------------------------------
# api_call METHOD PATH [BODY]  ->  echoes "<response-body><3-digit-http-code>".
# Callers split it via:  HTTP_CODE="${RESPONSE: -3}"; BODY="${RESPONSE%???}"
# ----------------------------------------------------------------------------
is_localhost() { [[ "$API_BASE" =~ ^https?://(localhost|127\.0\.0\.1|0\.0\.0\.0)(:[0-9]+)?$ ]]; }

api_call() {
    local method="$1" path="$2" body="${3:-}"
    local -a curl_args=(
        -s -S
        -X "$method"
        -H "Content-Type: application/json"
        -w '%{http_code}'
    )
    [[ "$INSECURE" == "1" ]] && curl_args+=(-k)
    [[ -n "$AUTH_TOKEN" ]] && curl_args+=(-H "Authorization: Bearer ${AUTH_TOKEN}")
    [[ -n "$body" ]] && curl_args+=(-d "$body")
    curl "${curl_args[@]}" "${API_BASE}${path}"
}

# ----------------------------------------------------------------------------
# JSON response parsing (jq). Every get_* echoes the matched id (or nothing) on
# stdout and never aborts, so callers can `X=$(get_… )` and test for empty.
# ----------------------------------------------------------------------------
# First "id" value in document order (root object first, then descendants) —
# equivalent to the old `grep -o '"id":"…"' | head -1`, robust to bare/wrapped.
extract_first_id() {
    printf '%s' "$1" | jq -r '[.. | objects | .id?] | map(select(. != null)) | .[0] // empty'
}

get_user_id_by_username() {
    local USERNAME="$1" RESPONSE
    RESPONSE=$(api_call GET "/users?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg u "$USERNAME" \
        '[.users[]? | select(.attributes.username == $u) | .id] | .[0] // empty'
}

get_group_id_by_name() {
    local GROUP_NAME="$1" OU_ID="$2" RESPONSE
    RESPONSE=$(api_call GET "/groups?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg n "$GROUP_NAME" --arg ou "$OU_ID" \
        '[.groups[]? | select(.name == $n and .ouId == $ou) | .id] | .[0] // empty'
}

get_role_id_by_name() {
    local ROLE_NAME="$1" OU_ID="$2" RESPONSE
    RESPONSE=$(api_call GET "/roles?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg n "$ROLE_NAME" --arg ou "$OU_ID" \
        '[.roles[]? | select(.name == $n and .ouId == $ou) | .id] | .[0] // empty'
}

get_application_id_by_client_id() {
    local CLIENT_ID="$1" RESPONSE
    RESPONSE=$(api_call GET "/applications?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg c "$CLIENT_ID" \
        '[.applications[]? | select(.clientId == $c) | .id] | .[0] // empty'
}

get_resource_server_id_by_identifier() {
    local IDENTIFIER="$1" RESPONSE
    RESPONSE=$(api_call GET "/resource-servers?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg i "$IDENTIFIER" \
        '[.resourceServers[]? | select(.identifier == $i) | .id] | .[0] // empty'
}

get_user_type_id_by_name() {
    local NAME="$1" RESPONSE
    RESPONSE=$(api_call GET "/user-types?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg n "$NAME" \
        '[.types[]? | select(.name == $n) | .id] | .[0] // empty'
}

get_resource_id_by_handle() {  # get_resource_id_by_handle <rs_id> <handle> [parent_id]
    local RS_ID="$1" HANDLE="$2" PARENT="${3:-}" Q RESPONSE
    Q="/resource-servers/${RS_ID}/resources?limit=100&offset=0"
    [[ -n "$PARENT" ]] && Q="${Q}&parentId=${PARENT}"
    RESPONSE=$(api_call GET "$Q")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg h "$HANDLE" \
        '[.resources[]? | select(.handle == $h) | .id] | .[0] // empty'
}

get_flow_id_by_handle() {
    local FLOW_TYPE="$1" FLOW_HANDLE="$2" RESPONSE
    RESPONSE=$(api_call GET "/flows?limit=30&offset=0&flowType=${FLOW_TYPE}")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r --arg h "$FLOW_HANDLE" \
        '[.flows[]? | select(.handle == $h) | .id] | .[0] // empty'
}

get_ou_id_by_handle() {
    local OU_HANDLE="$1" RESPONSE
    RESPONSE=$(api_call GET "/organization-units/tree/${OU_HANDLE}")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    extract_first_id "${RESPONSE%???}"
}

# List ids of the resources under a resource server (direct children of PARENT
# when given, else top-level resources). Echoes one id per line.
list_resource_ids() {
    local RS_ID="$1" PARENT="${2:-}" Q RESPONSE
    Q="/resource-servers/${RS_ID}/resources?limit=100&offset=0"
    [[ -n "$PARENT" ]] && Q="${Q}&parentId=${PARENT}"
    RESPONSE=$(api_call GET "$Q")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r '.resources[]?.id // empty'
}

list_action_ids() {
    local RS_ID="$1" RES_ID="$2" RESPONSE
    RESPONSE=$(api_call GET "/resource-servers/${RS_ID}/resources/${RES_ID}/actions?limit=100&offset=0")
    [[ "${RESPONSE: -3}" == "200" ]] || { echo ""; return; }
    printf '%s' "${RESPONSE%???}" | jq -r '.actions[]?.id // empty'
}

# role_has_assignment <role_id> <type: group|app> <target_id> -> exit 0 if the
# role is already assigned to that group/app (used to avoid duplicate-add errors).
role_has_assignment() {
    local ROLE_ID="$1" TYPE="$2" TARGET_ID="$3" RESPONSE
    RESPONSE=$(api_call GET "/roles/${ROLE_ID}/assignments?type=${TYPE}")
    [[ "${RESPONSE: -3}" == "200" ]] || return 1
    printf '%s' "${RESPONSE%???}" | jq -e --arg t "$TARGET_ID" \
        'any(.assignments[]?; .id == $t)' >/dev/null 2>&1
}

# ----------------------------------------------------------------------------
# Registry: logical key "<type>:<id>" -> server-assigned runtime ID.
# Stored as dynamic scalar vars REG_<sanitized-key> (no `declare -A` in 3.2).
# ----------------------------------------------------------------------------
_reg_san() { printf '%s' "$1" | LC_ALL=C tr -c 'A-Za-z0-9' '_'; }

reg_set() {  # reg_set <logicalKey> <runtimeId>
    local var
    var="REG_$(_reg_san "$1")"
    eval "$var=\$2"
}

reg_get() {  # echoes runtimeId, or "" if unset (never aborts)
    local var
    var="REG_$(_reg_san "$1")"
    eval "printf '%s' \"\${$var:-}\""
}

reg_require() {  # reg_get + hard fail if missing (use to resolve references)
    local val
    val="$(reg_get "$1")"
    if [[ -z "$val" ]]; then
        log_error "unresolved reference '$1' (entity not created yet — check provisioning order or a typo in config)"
        exit 1
    fi
    printf '%s' "$val"
}

# ----------------------------------------------------------------------------
# Secret resolution: config stores only env-var NAMES, never values. Preserves
# the original ${OVERRIDE:-${DEFAULT}} precedence via bash-3.2 indirect
# expansion (${!name}). Input is a compact JSON object {override,default}.
# ----------------------------------------------------------------------------
resolve_secret() {
    local spec="$1" ov def val
    ov="$(printf '%s' "$spec" | jq -r '.override // empty')"
    def="$(printf '%s' "$spec" | jq -r '.default // empty')"
    val=""
    [[ -n "$ov" ]] && val="${!ov:-}"
    if [[ -z "$val" && -n "$def" ]]; then val="${!def:-}"; fi
    printf '%s' "$val"
}

# ----------------------------------------------------------------------------
# scopeSet name -> the comma-separated quoted scope fragment the create_role /
# create_*_application helpers expect, e.g.  "nsw:task:read", "nsw:task:write"
# ----------------------------------------------------------------------------
scopeset_fragment() {
    printf '%s' "$SCOPESETS" | jq -r --arg n "$1" '
        .scopeSets[$n] // error("unknown scopeSet \($n)") | map("\"\(.)\"") | join(", ")'
}

# ----------------------------------------------------------------------------
# Expand the `agencies` shorthand into the primitive entity arrays the engine
# already provisions (mirrors the old setup_agency): one agency ->
#   1 OU (government-organization/<handle>)
#   1 officer user (Government_User, member of OGA Reviewers)
#   1 portal SPA + 2 M2M apps (<H>_TO_NSW, NSW_TO_<H>)
#   2 app-role assignments (AgencyM2M on _TO_NSW, NswM2M on NSW_TO_)
# Reads the merged base config (arg $1); echoes a JSON object of the new arrays.
# ----------------------------------------------------------------------------
expand_agencies() {
    printf '%s' "$1" | jq '
      (.agencies // []) as $ag
      | {
          organizationUnits: [ $ag[] | {
              key: ("government-organization/" + .handle),
              handle: .handle,
              name: .name,
              description: .description,
              parent: "government-organization",
              treePath: ("government-organization/" + .handle)
          } ],
          users: [ $ag[] | {
              key: .officer.username,
              type: "Government_User",
              ou: ("government-organization/" + .handle),
              username: .officer.username,
              email: .officer.email,
              givenName: .officer.givenName,
              familyName: .officer.familyName,
              phoneNumber: .officer.phoneNumber,
              passwordEnv: .officer.passwordEnv,
              groups: ["OGA Reviewers"]
          } ],
          applications: [ $ag[] |
              ( { key: .portal.clientId, kind: "spa", name: .portal.name,
                  description: ("Application for " + .name + " portal built with React"),
                  clientId: .portal.clientId, port: .portal.port,
                  allowedUserType: "Government_User",
                  ou: ("government-organization/" + .handle),
                  scopeSet: "agencyReviewer",
                  redirectUrisEnv: .portal.redirectUrisEnv } ),
              ( { key: .m2m.toNsw.clientId, kind: "m2m",
                  name: (.m2m.toNsw.clientId + "_M2M"),
                  description: ("Machine-to-machine integration for " + .name + " to NSW"),
                  clientId: .m2m.toNsw.clientId, secretEnv: .m2m.toNsw.secretEnv,
                  ou: "default", scopeSet: "m2mNsw" } ),
              ( { key: .m2m.nswTo.clientId, kind: "m2m",
                  name: (.m2m.nswTo.clientId + "_M2M"),
                  description: ("Machine-to-machine integration for NSW to " + .name),
                  clientId: .m2m.nswTo.clientId, secretEnv: .m2m.nswTo.secretEnv,
                  ou: "government-organization", scopeSet: "m2mAgency" } )
          ],
          appRoleAssignments: [ $ag[] |
              ( { role: "AgencyM2M", app: .m2m.toNsw.clientId } ),
              ( { role: "NswM2M",   app: .m2m.nswTo.clientId } )
          ]
      }'
}

# ----------------------------------------------------------------------------
# Load + merge every resources/*.json into $MERGED (top-level arrays keyed by
# entity type are concatenated across files), fold in the agency expansion, and
# load _scopesets.json into $SCOPESETS. Sets globals: MERGED, SCOPESETS.
# ----------------------------------------------------------------------------
# jq program: merge an array of config docs by concatenating same-named arrays.
_MERGE_PROG='reduce .[] as $f ({}; reduce ($f|to_entries[]) as $e (.; .[$e.key] = ((.[$e.key] // []) + $e.value)))'

load_config() {
    command -v jq >/dev/null 2>&1 || { log_error "jq is required but was not found in PATH."; exit 1; }

    local res_dir="${SCRIPT_DIR}/resources"
    [[ -d "$res_dir" ]] || { log_error "config directory not found: $res_dir"; exit 1; }

    local ss_file="${res_dir}/_scopesets.json"
    [[ -f "$ss_file" ]] || { log_error "scope-sets file not found: $ss_file"; exit 1; }
    jq -e . "$ss_file" >/dev/null 2>&1 || { log_error "invalid JSON: $ss_file"; exit 1; }
    SCOPESETS="$(cat "$ss_file")"

    local files=() f
    while IFS= read -r f; do files+=("$f"); done < <(find "$res_dir" -type f -name '*.json' ! -name '_scopesets.json' | sort)
    [[ ${#files[@]} -gt 0 ]] || { log_error "no config files found under $res_dir"; exit 1; }
    for f in "${files[@]}"; do
        jq -e . "$f" >/dev/null 2>&1 || { log_error "invalid JSON: $f"; exit 1; }
    done

    local base expanded
    base="$(jq -s "$_MERGE_PROG" "${files[@]}")"
    expanded="$(expand_agencies "$base")"
    # Fold expanded primitives onto base, then drop the consumed `agencies` key.
    MERGED="$(printf '%s\n%s\n' "$base" "$expanded" | jq -s "$_MERGE_PROG" | jq 'del(.agencies)')"
}
