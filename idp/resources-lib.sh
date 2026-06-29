# shellcheck shell=bash
# ============================================================================
# resources-lib.sh — shared engine for the data-driven seed / teardown scripts.
#
# Sourced by BOTH idp/sample-resources.sh (create) and
# idp/sample-resources.down.sh (delete) so the idp/resources/ JSON config is the
# single source of truth and the two scripts can never drift.
#
# Requires (defined by the sourcing script BEFORE this file is sourced):
#   - log_info / log_warning / log_error  (stderr loggers)
#   - SCRIPT_DIR                           (dir containing the resources/ tree)
#
# Provides:
#   reg_set / reg_get / reg_require   logical-key -> runtime-ID registry
#   resolve_secret                    {override,default} env-name -> value
#   scopeset_fragment                 scopeSet name  -> '"a", "b", ...' fragment
#   expand_agencies                   agencies[] -> primitive entity arrays (jq)
#   load_config                       glob + merge resources/ -> $MERGED, $SCOPESETS
#
# bash-3.2 safe: NO associative arrays (the dev machine runs bash 3.2.57). The
# registry uses dynamic scalar variables; jq arrays are iterated with
# here-strings (never pipes — a `... | while read` body runs in a subshell on
# bash 4+ and would lose registry writes, a bug that does NOT reproduce on 3.2).
# ============================================================================

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
