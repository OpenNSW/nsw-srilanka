#!/usr/bin/env bash

set -e

# ============================================================================
# Tear down (delete) the NSW sample resources from a ThunderID deployment.
#
# This is the inverse of sample-resources.sh. It deletes ONLY the project's own
# entities — applications, users, groups, roles, user types, the NSW_API /
# AGENCY_API resource servers (and their resources/actions), and the
# private-sector / government organization units — in reverse-dependency order.
# It NEVER touches image defaults (the `default` OU, the `Person` user type, the
# admin user, the system resource server, default flows/themes).
#
# "Delete if exists": entities that are already gone are skipped, so the script
# is idempotent and safe to re-run.
#
# Usage:
#   API_BASE=https://idp.example.com AUTH_TOKEN=<bearer> ./idp/sample-resources.down.sh --yes
#
# See ./idp/sample-resources.down.sh --help for the full environment reference.
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

usage() {
    cat >&2 <<'USAGE'
Delete (tear down) the NSW sample resources from a running ThunderID deployment.
Inverse of sample-resources.sh. Deletes ONLY the project's own entities in
reverse-dependency order; never touches image defaults (default OU, Person type,
admin user, system resource server).

Idempotent: entities that don't exist are skipped.

Usage:
  API_BASE=https://idp.example.com AUTH_TOKEN=<bearer> ./idp/sample-resources.down.sh --yes

Environment (same as sample-resources.sh):
  API_BASE      Base URL of the ThunderID server (default: https://localhost:8090).
  AUTH_TOKEN    Bearer token for the management API. REQUIRED for non-localhost
                targets; sent as "Authorization: Bearer <token>".
  ALLOW_NO_AUTH Set to 1 to skip the AUTH_TOKEN requirement (security-disabled targets).
  INSECURE      "1" (default) skips TLS verification for self-signed localhost certs.

Flags:
  -y, --yes     Skip the interactive confirmation (REQUIRED when non-interactive).
  -h, --help    Show this help and exit.

WARNING: destructive. Deleted entities cannot be restored; re-seeding mints NEW
IDs, and live OAuth clients referencing the deleted apps / resource servers will
break until sample-resources.sh is run again.
USAGE
}

YES=0
case "${1:-}" in
    -h|--help) usage; exit 0 ;;
    -y|--yes)  YES=1 ;;
    "")        ;;
    *)         printf '[ERROR] Unknown argument: %s\n' "$1" >&2; usage; exit 1 ;;
esac

# Load .env values when available (.env wins, matching sample-resources.sh).
ENV_FILE="${SCRIPT_DIR}/.env"
if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
fi

API_BASE="${API_BASE:-https://localhost:8090}"
API_BASE="${API_BASE%/}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
INSECURE="${INSECURE:-1}"
ALLOW_NO_AUTH="${ALLOW_NO_AUTH:-0}"

# ============================================================================
# Shared engine library — logging, is_localhost, api_call, JSON lookups
# (get_*/list_*/extract_first_id), config load/merge, agency expansion. Sourced
# after SCRIPT_DIR and the API_BASE/AUTH_TOKEN/INSECURE config vars are set above
# (api_call reads them). The delete_* helpers below build on these.
# shellcheck source=resources-lib.sh
# ============================================================================
source "${SCRIPT_DIR}/resources-lib.sh"

# ============================================================================
# Auth guard + one-time auth/connectivity probe
# ============================================================================
if [[ -z "$AUTH_TOKEN" && "$ALLOW_NO_AUTH" != "1" ]]; then
    if is_localhost; then
        log_warning "No AUTH_TOKEN set and API_BASE is localhost; assuming security-disabled mode. Set ALLOW_NO_AUTH=1 to silence, or export AUTH_TOKEN."
    else
        log_error "AUTH_TOKEN is required for non-localhost targets (API_BASE=$API_BASE)."
        exit 1
    fi
fi

_PROBE=$(api_call GET "/organization-units/tree/default") || true
_PROBE_CODE="${_PROBE: -3}"
if [[ "$_PROBE_CODE" == "401" || "$_PROBE_CODE" == "403" ]]; then
    log_error "Authorization failed (HTTP $_PROBE_CODE) against $API_BASE. Check AUTH_TOKEN."
    exit 1
elif [[ "$_PROBE_CODE" == "000" || -z "$_PROBE_CODE" ]]; then
    log_error "Could not reach the IdP at $API_BASE (connection failed). Check API_BASE / network / TLS (INSECURE=$INSECURE)."
    exit 1
fi

# ============================================================================
# Delete helpers — "delete if exists" (missing -> skip)
# ============================================================================
delete_by_id() {  # delete_by_id <path> <label>
    local path="$1" label="$2" RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call DELETE "$path")
    HTTP_CODE="${RESPONSE: -3}"; BODY="${RESPONSE%???}"
    case "$HTTP_CODE" in
        200|204) log_success "Deleted ${label}" ;;
        404)     log_warning "${label} not found (already deleted), skipping" ;;
        *)       log_error "Failed to delete ${label} (HTTP $HTTP_CODE)"; echo "Response: $BODY" >&2; exit 1 ;;
    esac
}

delete_application() {
    local client_id="$1" id
    id=$(get_application_id_by_client_id "$client_id")
    if [[ -z "$id" ]]; then log_warning "Application '${client_id}' not found, skipping"; return; fi
    delete_by_id "/applications/${id}" "application ${client_id}"
}

delete_user() {
    local username="$1" id
    id=$(get_user_id_by_username "$username")
    if [[ -z "$id" ]]; then log_warning "User '${username}' not found, skipping"; return; fi
    delete_by_id "/users/${id}" "user ${username}"
}

delete_group() {
    local name="$1" ou_id="$2" id
    if [[ -z "$ou_id" ]]; then log_warning "Group '${name}' (OU already gone), skipping"; return; fi
    id=$(get_group_id_by_name "$name" "$ou_id")
    if [[ -z "$id" ]]; then log_warning "Group '${name}' not found, skipping"; return; fi
    delete_by_id "/groups/${id}" "group ${name}"
}

delete_role() {
    local name="$1" ou_id="$2" id
    if [[ -z "$ou_id" ]]; then log_warning "Role '${name}' (OU already gone), skipping"; return; fi
    id=$(get_role_id_by_name "$name" "$ou_id")
    if [[ -z "$id" ]]; then log_warning "Role '${name}' not found, skipping"; return; fi
    delete_by_id "/roles/${id}" "role ${name}"
}

delete_user_type() {
    local name="$1" id
    id=$(get_user_type_id_by_name "$name")
    if [[ -z "$id" ]]; then log_warning "User type '${name}' not found, skipping"; return; fi
    delete_by_id "/user-types/${id}" "user type ${name}"
}

delete_ou() {
    local tree_path="$1" label="$2" id
    id=$(get_ou_id_by_handle "$tree_path")
    if [[ -z "$id" ]]; then log_warning "OU '${label}' not found, skipping"; return; fi
    delete_by_id "/organization-units/${id}" "OU ${label}"
}

# Delete a resource and everything under it: recurse into child resources first,
# then delete this resource's actions, then the resource itself (the API refuses
# to delete a resource that still has sub-resources or actions).
delete_resource_subtree() {
    local rs_id="$1" res_id="$2" kid aid
    for kid in $(list_resource_ids "$rs_id" "$res_id"); do
        delete_resource_subtree "$rs_id" "$kid"
    done
    for aid in $(list_action_ids "$rs_id" "$res_id"); do
        delete_by_id "/resource-servers/${rs_id}/resources/${res_id}/actions/${aid}" "action ${aid}"
    done
    delete_by_id "/resource-servers/${rs_id}/resources/${res_id}" "resource ${res_id}"
}

delete_resource_server() {
    local identifier="$1" rs_id root
    rs_id=$(get_resource_server_id_by_identifier "$identifier")
    if [[ -z "$rs_id" ]]; then log_warning "Resource server '${identifier}' not found, skipping"; return; fi
    log_info "Tearing down resource server '${identifier}' (${rs_id})..."
    # Delete all resource trees (top-level resources and their descendants/actions)
    for root in $(list_resource_ids "$rs_id" ""); do
        delete_resource_subtree "$rs_id" "$root"
    done
    delete_by_id "/resource-servers/${rs_id}" "resource server ${identifier}"
}

confirm_or_abort() {
    [[ "$YES" == "1" ]] && return
    if [[ -t 0 ]]; then
        printf 'About to DELETE all NSW sample resources from %s. This is destructive and irreversible. Type "yes" to continue: ' "$API_BASE" >&2
        local ans=""
        read -r ans || true
        [[ "$ans" == "yes" ]] || { log_error "Aborted (no confirmation)."; exit 1; }
    else
        log_error "Refusing to run non-interactively without confirmation. Re-run with --yes."
        exit 1
    fi
}

# ============================================================================
# Main — delete in reverse-dependency order, all lists derived from the merged
# idp/resources/ config (no hardcoded entity names).
# ============================================================================
log_info "Tearing down NSW sample resources from ${API_BASE}..."
confirm_or_abort
load_config

echo "" >&2
log_info "### (1) Applications ###"
while IFS= read -r c; do
    [[ -z "$c" ]] && continue
    delete_application "$c"
done <<< "$(jq -r '.applications // [] | .[].clientId' <<< "$MERGED")"

echo "" >&2
log_info "### (2) Users ###"
while IFS= read -r u; do
    [[ -z "$u" ]] && continue
    delete_user "$u"
done <<< "$(jq -r '.users // [] | .[].username' <<< "$MERGED")"

echo "" >&2
log_info "### (3) Groups ###"
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    g_name="${line%%$'\t'*}"; g_ou="${line#*$'\t'}"
    g_ou_id="$(get_ou_id_by_handle "$g_ou")"
    delete_group "$g_name" "$g_ou_id"
done <<< "$(jq -r '.groups // [] | .[] | "\(.name)\t\(.ou)"' <<< "$MERGED")"

echo "" >&2
log_info "### (4) Roles ###"
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    r_name="${line%%$'\t'*}"; r_ou="${line#*$'\t'}"
    r_ou_id="$(get_ou_id_by_handle "$r_ou")"
    delete_role "$r_name" "$r_ou_id"
done <<< "$(jq -r '.roles // [] | .[] | "\(.name)\t\(.ou)"' <<< "$MERGED")"

echo "" >&2
log_info "### (5) User types ###"
while IFS= read -r t; do
    [[ -z "$t" ]] && continue
    delete_user_type "$t"
done <<< "$(jq -r '.userTypes // [] | .[].name' <<< "$MERGED")"

echo "" >&2
log_info "### (6) Resource servers (resources + actions first) ###"
while IFS= read -r rs; do
    [[ -z "$rs" ]] && continue
    delete_resource_server "$rs"
done <<< "$(jq -r '.resourceServers // [] | .[].identifier' <<< "$MERGED")"

echo "" >&2
log_info "### (7) Organization units (children before parents) ###"
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    ou_tree="${line#*$'\t'}"
    delete_ou "$ou_tree" "${ou_tree##*/}"
done <<< "$(jq -r '.organizationUnits // [] | .[] | [ ((.treePath // .handle) | [scan("/")] | length), (.treePath // .handle) ] | "\(.[0])\t\(.[1])"' <<< "$MERGED" | sort -rn -k1,1 -s)"

echo "" >&2
log_success "Sample resources teardown completed."
log_info "Note: image defaults (default OU, Person type, admin, system resource server) were left untouched."
echo "" >&2
