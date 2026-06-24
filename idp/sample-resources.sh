#!/usr/bin/env bash

set -e

# ============================================================================
# Seed NSW sample resources into a running ThunderID deployment.
#
# This script is self-contained: it defines its own api_call / log_* helpers
# (it does NOT rely on the ThunderID image's common.sh) so it can be run by
# hand against any deployment. It used to run only inside the compose bootstrap
# container with security disabled; now it is a manual step.
#
# Usage:
#   API_BASE=https://idp.example.com AUTH_TOKEN=<bearer> ./idp/sample-resources.sh
#
# See ./idp/sample-resources.sh --help for the full environment reference.
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

usage() {
    cat >&2 <<'USAGE'
Seed NSW sample resources (OUs, users, groups, roles, SPA + M2M apps) into a
running ThunderID deployment.

Usage:
  API_BASE=https://idp.example.com AUTH_TOKEN=<bearer> ./idp/sample-resources.sh

Environment:
  API_BASE      Base URL of the ThunderID server (default: https://localhost:8090).
  AUTH_TOKEN    Bearer token for the management API. REQUIRED for non-localhost
                targets; sent as "Authorization: Bearer <token>".
  ALLOW_NO_AUTH Set to 1 to skip the AUTH_TOKEN requirement (only safe when the
                target runs with security disabled, e.g. the compose bootstrap).
  INSECURE      "1" (default) skips TLS verification for self-signed localhost
                certs. Set INSECURE=0 to enforce certificate validation.

  SAMPLE_USER_PASSWORD, M2M_CLIENT_SECRET, and the per-entity overrides documented
  in idp/.env.example tune the seeded secrets. Values in idp/.env are loaded
  automatically and take precedence.

Flags:
  -h, --help    Show this help and exit.

The script is idempotent: re-running against a partially-seeded deployment
detects existing entities (HTTP 409) and reuses them.
USAGE
}

case "${1:-}" in
    -h|--help) usage; exit 0 ;;
esac

# Load .env values when available (useful for local execution). .env wins over
# the caller's environment (preserves the original `set -a; source` behavior).
ENV_FILE="${SCRIPT_DIR}/.env"
if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
fi

API_BASE="${API_BASE:-https://localhost:8090}"
API_BASE="${API_BASE%/}"            # strip a single trailing slash
AUTH_TOKEN="${AUTH_TOKEN:-}"
INSECURE="${INSECURE:-1}"
ALLOW_NO_AUTH="${ALLOW_NO_AUTH:-0}"

# ============================================================================
# Logging — ALWAYS to stderr so helpers can echo IDs on stdout inside $( ... ).
# ============================================================================
log_info()    { printf '[INFO] %s\n'    "$*" >&2; }
log_success() { printf '[SUCCESS] %s\n' "$*" >&2; }
log_warning() { printf '[WARNING] %s\n' "$*" >&2; }
log_error()   { printf '[ERROR] %s\n'   "$*" >&2; }

# ============================================================================
# api_call METHOD PATH [BODY]
# Echoes "<response-body><3-digit-http-code>" to stdout. Callers split it via:
#   HTTP_CODE="${RESPONSE: -3}"; BODY="${RESPONSE%???}"
# ============================================================================
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

# ============================================================================
# Auth guard + one-time auth/connectivity probe
# ============================================================================
is_localhost() { [[ "$API_BASE" =~ ^https?://(localhost|127\.0\.0\.1|0\.0\.0\.0)(:[0-9]+)?$ ]]; }

if [[ -z "$AUTH_TOKEN" && "$ALLOW_NO_AUTH" != "1" ]]; then
    if is_localhost; then
        log_warning "No AUTH_TOKEN set and API_BASE is localhost; assuming security-disabled bootstrap mode. Set ALLOW_NO_AUTH=1 to silence, or export AUTH_TOKEN."
    else
        log_error "AUTH_TOKEN is required for non-localhost targets (API_BASE=$API_BASE)."
        log_error "Obtain a bearer token for the ThunderID management API and re-run:"
        log_error "  API_BASE=$API_BASE AUTH_TOKEN=<token> $0"
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
# Sample secrets / passwords (overridable via env / .env)
# ============================================================================
SAMPLE_USER_PASSWORD="${SAMPLE_USER_PASSWORD:-1234}"
SURESH_PASSWORD="${SAMPLE_SURESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
RAMESH_PASSWORD="${SAMPLE_RAMESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
GOMESH_PASSWORD="${SAMPLE_GOMESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
NARESH_PASSWORD="${SAMPLE_NARESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
NPQS_OFFICER_PASSWORD="${SAMPLE_NPQS_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
FCAU_OFFICER_PASSWORD="${SAMPLE_FCAU_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
CDA_OFFICER_PASSWORD="${SAMPLE_CDA_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
SLPA_OFFICER_PASSWORD="${SAMPLE_SLPA_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
CUSTOMS_OFFICER_PASSWORD="${SAMPLE_CUSTOMS_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
SLTB_OFFICER_PASSWORD="${SAMPLE_SLTB_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
M2M_CLIENT_SECRET="${M2M_CLIENT_SECRET:-1234}"
# Outbound (Agency -> NSW) M2M client secrets — one per *_TO_NSW client.
NPQS_M2M_CLIENT_SECRET="${M2M_NPQS_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
FCAU_M2M_CLIENT_SECRET="${M2M_FCAU_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
CDA_M2M_CLIENT_SECRET="${M2M_CDA_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
SLPA_M2M_CLIENT_SECRET="${M2M_SLPA_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
CUSTOMS_M2M_CLIENT_SECRET="${M2M_CUSTOMS_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
SLTB_M2M_CLIENT_SECRET="${M2M_SLTB_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
# Inbound (NSW -> Agency) M2M client secrets — one per NSW_TO_* client.
NSW_TO_NPQS_M2M_CLIENT_SECRET="${M2M_NSW_TO_NPQS_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_FCAU_M2M_CLIENT_SECRET="${M2M_NSW_TO_FCAU_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_CDA_M2M_CLIENT_SECRET="${M2M_NSW_TO_CDA_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_SLPA_M2M_CLIENT_SECRET="${M2M_NSW_TO_SLPA_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_CUSTOMS_M2M_CLIENT_SECRET="${M2M_NSW_TO_CUSTOMS_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_SLTB_M2M_CLIENT_SECRET="${M2M_NSW_TO_SLTB_SECRET:-${M2M_CLIENT_SECRET}}"

# ----------------------------------------------------------------------------
# Per-SPA OAuth redirect URIs (space- or comma-separated; multiple allowed).
# When unset, each SPA defaults to the local dev pair http(s)://localhost:<port>.
# Override per app for real deployments, e.g.:
#   TRADER_REDIRECT_URIS="https://trader.example.lk https://trader.example.lk/callback"
# ----------------------------------------------------------------------------
TRADER_REDIRECT_URIS="${TRADER_REDIRECT_URIS:-}"
NPQS_REDIRECT_URIS="${NPQS_REDIRECT_URIS:-}"
FCAU_REDIRECT_URIS="${FCAU_REDIRECT_URIS:-}"
CDA_REDIRECT_URIS="${CDA_REDIRECT_URIS:-}"
SLPA_REDIRECT_URIS="${SLPA_REDIRECT_URIS:-}"
CUSTOMS_REDIRECT_URIS="${CUSTOMS_REDIRECT_URIS:-}"
SLTB_REDIRECT_URIS="${SLTB_REDIRECT_URIS:-}"
# ----------------------------------------------------------------------------
# OAuth2 resource servers & scope sets
# ----------------------------------------------------------------------------
# Two backends are OAuth2-protected resources. Each gets a resource server
# whose identifier becomes the access-token audience (aud) its backend
# validates:
#   NSW_API    -> OpenNSW/nsw backend        (AUTH_AUDIENCE=NSW_API)
#   AGENCY_API -> OpenNSW/nsw-agency backend  (AUTH_AUDIENCE=AGENCY_API)
#
# Scopes derive from "<resource>:<action>" handles. Because both APIs expose
# consignment/storage resources, scopes are namespaced per server ("nsw:*" /
# "agency:*") via a wrapper resource so each scope is globally unique and maps
# unambiguously to a single audience. The fragments below are reused for BOTH
# role permissions (what a role grants) and application scopes (what a client
# may request); keep them in sync with the create_resource/create_action calls.
NSW_API_IDENTIFIER="NSW_API"
AGENCY_API_IDENTIFIER="AGENCY_API"

# Traders/CHA (private-sector SPA users via TraderApp -> NSW_API): manage
# consignments, drive their task steps, read reference data, upload documents.
TRADER_NSW_SCOPES='"nsw:consignment:read", "nsw:consignment:write", "nsw:task:read", "nsw:task:write", "nsw:hscode:read", "nsw:company:read", "nsw:cha:read", "nsw:storage:read", "nsw:storage:write"'

# External OGA systems (M2M client_credentials -> NSW_API): push task outcomes
# and read the consignment context for their processing,
# and read/write storage for document exchange.
M2M_NSW_SCOPES='"nsw:task:write", "nsw:consignment:read", "nsw:storage:read", "nsw:storage:write"'

# Government reviewers (OGA portal SPA users via *PortalApp -> AGENCY_API):
# review trader applications and read/write supporting documents.
AGENCY_REVIEWER_SCOPES='"agency:application:read", "agency:application:review", "agency:application:feedback", "agency:consignment:read", "agency:storage:read", "agency:storage:write"'

# NSW core (M2M client_credentials -> AGENCY_API): inject consignment/task
# data into an agency's review queue.
M2M_AGENCY_SCOPES='"agency:application:inject"'

# ============================================================================
# Helpers
# ============================================================================

extract_first_id() {
    echo "$1" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

get_user_id_by_username() {
    local USERNAME="$1"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/users?limit=100&offset=0")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    # Parse one user object per line and locate matching username inside attributes.
    echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"username\":\"${USERNAME}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

get_group_id_by_name() {
    local GROUP_NAME="$1"
    local OU_ID="$2"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/groups?limit=100&offset=0")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"name\":\"${GROUP_NAME}\"" | grep "\"ouId\":\"${OU_ID}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

get_role_id_by_name() {
    local ROLE_NAME="$1"
    local OU_ID="$2"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/roles?limit=100&offset=0")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"name\":\"${ROLE_NAME}\"" | grep "\"ouId\":\"${OU_ID}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

get_flow_id_by_handle() {
    local FLOW_TYPE="$1"
    local FLOW_HANDLE="$2"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/flows?limit=30&offset=0&flowType=${FLOW_TYPE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    echo "$BODY" | grep -o '{[^}]*"handle":"'"${FLOW_HANDLE}"'"[^}]*}' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

get_application_id_by_client_id() {
    local CLIENT_ID="$1"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/applications?limit=100&offset=0")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"clientId\":\"${CLIENT_ID}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

get_ou_id_by_handle() {
    local OU_HANDLE="$1"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/organization-units/tree/${OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    extract_first_id "$BODY"
}

# Create an organization unit (idempotent). Echoes the OU ID on stdout.
# Usage: create_ou <handle> <name> <description> [parent_ou_id] [tree_path]
#   - root OU: omit parent_ou_id; tree_path defaults to <handle>
#   - child OU: pass parent_ou_id and tree_path "<parent-handle>/<handle>"
create_ou() {
    local HANDLE="$1" NAME="$2" DESCRIPTION="$3" PARENT_ID="${4:-}" TREE_PATH="${5:-$1}"
    local RESPONSE HTTP_CODE BODY OU_ID PARENT_FIELD=""

    if [[ -n "$PARENT_ID" ]]; then
        PARENT_FIELD=",
    \"parent\": \"${PARENT_ID}\""
    fi

    read -r -d '' OU_PAYLOAD <<JSON || true
{
    "handle": "${HANDLE}",
    "name": "${NAME}",
    "description": "${DESCRIPTION}"${PARENT_FIELD}
}
JSON

    RESPONSE=$(api_call POST "/organization-units" "${OU_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "${NAME} organization unit created successfully"
        OU_ID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} organization unit already exists, retrieving ID..."
        RESPONSE=$(api_call GET "/organization-units/tree/${TREE_PATH}")
        HTTP_CODE="${RESPONSE: -3}"
        BODY="${RESPONSE%???}"
        if [[ "$HTTP_CODE" == "200" ]]; then
            OU_ID=$(extract_first_id "$BODY")
        else
            log_error "Failed to fetch organization unit by path '${TREE_PATH}' (HTTP $HTTP_CODE)"
            echo "Response: $BODY" >&2
            exit 1
        fi
    else
        log_error "Failed to create ${NAME} organization unit (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
        exit 1
    fi

    if [[ -z "$OU_ID" ]]; then
        log_error "Could not determine ${NAME} organization unit ID"
        exit 1
    fi
    log_info "${NAME} OU ID: $OU_ID"
    echo "$OU_ID"
}

# Create a user type (idempotent). Takes a pre-built JSON payload (the schema
# differs per type). 409 -> skip. Usage: create_user_type <name> <payload-json>
create_user_type() {
    local NAME="$1" PAYLOAD="$2"
    local RESPONSE HTTP_CODE
    RESPONSE=$(api_call POST "/user-types" "${PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "${NAME} user type created successfully"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} user type already exists, skipping"
    else
        log_error "Failed to create ${NAME} user type (HTTP $HTTP_CODE)"
        exit 1
    fi
}

# Create a group (idempotent). Echoes the group ID on stdout.
# Usage: create_group <name> <description> <ou_id>
create_group() {
    local NAME="$1" DESCRIPTION="$2" OU_ID="$3"
    local RESPONSE HTTP_CODE BODY GID

    read -r -d '' GROUP_PAYLOAD <<JSON || true
{
    "name": "${NAME}",
    "description": "${DESCRIPTION}",
    "ouId": "${OU_ID}"
}
JSON

    RESPONSE=$(api_call POST "/groups" "${GROUP_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "${NAME} group created successfully"
        GID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} group already exists, retrieving ID..."
        GID=$(get_group_id_by_name "$NAME" "$OU_ID")
    else
        log_error "Failed to create ${NAME} group (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
        exit 1
    fi

    if [[ -z "$GID" ]]; then
        log_error "Could not determine ${NAME} group ID"
        exit 1
    fi
    log_info "${NAME} group ID: $GID"
    echo "$GID"
}

# Create a role granting a resource server's scopes (idempotent). Echoes role ID.
# Usage: create_role <name> <description> <ou_id> <resource_server_id> <permissions-fragment>
#   permissions-fragment is the comma-separated quoted scope list, e.g. "$TRADER_NSW_SCOPES"
create_role() {
    local NAME="$1" DESCRIPTION="$2" OU_ID="$3" RS_ID="$4" PERMS="$5"
    local RESPONSE HTTP_CODE BODY RID

    read -r -d '' ROLE_PAYLOAD <<JSON || true
{
    "name": "${NAME}",
    "description": "${DESCRIPTION}",
    "ouId": "${OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${RS_ID}",
            "permissions": [ ${PERMS} ]
        }
    ]
}
JSON

    RESPONSE=$(api_call POST "/roles" "${ROLE_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "${NAME} role created successfully"
        RID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} role already exists, retrieving ID..."
        RID=$(get_role_id_by_name "$NAME" "$OU_ID")
    else
        log_error "Failed to create ${NAME} role (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
        exit 1
    fi

    if [[ -z "$RID" ]]; then
        log_error "Could not determine ${NAME} role ID"
        exit 1
    fi
    log_info "${NAME} role ID: $RID"
    echo "$RID"
}

create_user_in_ou() {
    local USER_TYPE="$1"
    local OU_ID="$2"
    local USERNAME="$3"
    local EMAIL="$4"
    local GIVEN_NAME="$5"
    local FAMILY_NAME="$6"
    local PASSWORD="$7"
    local PHONE_NUMBER="$8"

    local RESPONSE HTTP_CODE BODY USER_ID

    read -r -d '' USER_PAYLOAD <<JSON || true
{
    "type": "${USER_TYPE}",
    "ouId": "${OU_ID}",
    "attributes": {
        "username": "${USERNAME}",
        "password": "${PASSWORD}",
        "email": "${EMAIL}",
        "given_name": "${GIVEN_NAME}",
        "family_name": "${FAMILY_NAME}",
        "phone_number": "${PHONE_NUMBER}"
    }
}
JSON

    RESPONSE=$(api_call POST "/users" "${USER_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "User ${USERNAME} created successfully"
        USER_ID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "User ${USERNAME} already exists, retrieving ID..."
        USER_ID=$(get_user_id_by_username "$USERNAME")
    else
        log_error "Failed to create user ${USERNAME} (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi

    if [[ -z "$USER_ID" ]]; then
        log_error "Could not determine user ID for ${USERNAME}"
        exit 1
    fi

    log_info "${USERNAME} user ID: $USER_ID"
    CREATED_USER_ID="$USER_ID"
}

# Convert a space/comma-separated list into a JSON array body: "a", "b", "c"
json_str_list() {
    local raw="${1//,/ }" item out=""
    for item in $raw; do
        [[ -z "$item" ]] && continue
        if [[ -z "$out" ]]; then out="\"${item}\""; else out="${out}, \"${item}\""; fi
    done
    printf '%s' "$out"
}

create_spa_application() {
    local APP_NAME="$1"
    local APP_DESCRIPTION="$2"
    local CLIENT_ID="$3"
    local PORT="$4"
    local ALLOWED_USER_TYPE="$5"
    local OU_ID="$6"
    local API_SCOPES="${7:-}"
    local REDIRECT_URIS="${8:-}"
    local RESPONSE HTTP_CODE BODY
    local APP_ID APP_CLIENT_ID

    # Resource-server scopes this app may request (sets the token audience).
    local API_SCOPES_FRAGMENT=""
    [[ -n "$API_SCOPES" ]] && API_SCOPES_FRAGMENT=",
                    ${API_SCOPES}"

    # OAuth redirect URIs: use the caller-provided list (space/comma-separated)
    # when set, otherwise default to the local dev pair derived from PORT.
    local REDIRECT_URIS_JSON
    if [[ -n "$REDIRECT_URIS" ]]; then
        REDIRECT_URIS_JSON="$(json_str_list "$REDIRECT_URIS")"
    else
        REDIRECT_URIS_JSON="\"http://localhost:${PORT}\", \"https://localhost:${PORT}\""
    fi

    log_info "Creating ${APP_NAME} application..."

    ADDITIONAL_FIELDS=""
    if [[ -n "$CLASSIC_THEME_ID" ]]; then
        ADDITIONAL_FIELDS="${ADDITIONAL_FIELDS}
    \"themeId\": \"${CLASSIC_THEME_ID}\","
    fi
    if [[ -n "$AUTH_FLOW_ID" ]]; then
        ADDITIONAL_FIELDS="${ADDITIONAL_FIELDS}
    \"authFlowId\": \"${AUTH_FLOW_ID}\","
    fi
    if [[ -n "$REG_FLOW_ID" ]]; then
        ADDITIONAL_FIELDS="${ADDITIONAL_FIELDS}
    \"registrationFlowId\": \"${REG_FLOW_ID}\","
    fi

    read -r -d '' APP_PAYLOAD <<JSON || true
{
    "name": "${APP_NAME}",
    "description": "${APP_DESCRIPTION}",${ADDITIONAL_FIELDS}
    "ouId": "${OU_ID}",
    "isRegistrationFlowEnabled": false,
    "template": "react",
    "logoUrl": "https://ssl.gstatic.com/docs/common/profile/kiwi_lg.png",
    "assertion": {
        "validityPeriod": 3600
    },
    "inboundAuthConfig": [
        {
            "type": "oauth2",
            "config": {
                "clientId": "${CLIENT_ID}",
                "redirectUris": [
                    ${REDIRECT_URIS_JSON}
                ],
                "grantTypes": [
                    "authorization_code",
                    "refresh_token"
                ],
                "responseTypes": [
                    "code"
                ],
                "tokenEndpointAuthMethod": "none",
                "pkceRequired": true,
                "publicClient": true,
                "token": {
                    "accessToken": {
                        "validityPeriod": 3600,
                        "userAttributes": [
                            "email",
                            "phone_number",
                            "family_name",
                            "given_name",
                            "groups",
                            "roles",
                            "ouHandle",
                            "ouId",
                            "ouName",
                            "username"
                        ]
                    },
                    "idToken": {
                        "validityPeriod": 3600,
                        "userAttributes": [
                            "email",
                            "family_name",
                            "given_name",
                            "groups",
                            "roles",
                            "ouHandle",
                            "ouId",
                            "ouName",
                            "username"
                        ]
                    }
                },
                "scopes": [
                    "openid",
                    "profile",
                    "email",
                    "group",
                    "role",
                    "ou"${API_SCOPES_FRAGMENT}
                ],
                "userInfo": {
                    "userAttributes": [
                        "family_name",
                        "given_name",
                        "email"
                    ]
                },
                "scopeClaims": {
                    "profile": [
                        "name",
                        "given_name",
                        "family_name"
                    ],
                    "email": [
                        "email"
                    ],
                    "phone": [
                        "phone_number"
                    ],
                    "group": [
                        "groups"
                    ],
                    "ou": [
                        "ouId",
                        "ouHandle"
                    ],
                    "role": [
                        "roles"
                    ]
                }
            }
        }
    ],
    "userAttributes": [
        "given_name",
        "family_name",
        "email",
        "groups",
        "ouId",
        "ouHandle",
        "ouName",
        "username"
    ],
    "allowedUserTypes": [
        "${ALLOWED_USER_TYPE}"
    ]
}
JSON

    RESPONSE=$(api_call POST "/applications" "${APP_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]] || [[ "$HTTP_CODE" == "202" ]]; then
        log_success "${APP_NAME} application created successfully"
        APP_ID=$(extract_first_id "$BODY")
        APP_CLIENT_ID=$(echo "$BODY" | grep -o '"clientId":"[^"]*"' | head -1 | cut -d'"' -f4)
        if [[ -n "$APP_ID" ]]; then
            log_info "${APP_NAME} app ID: ${APP_ID}"
        fi
        if [[ -n "$APP_CLIENT_ID" ]]; then
            log_info "${APP_NAME} client ID: ${APP_CLIENT_ID}"
        fi
    elif [[ "$HTTP_CODE" == "409" ]] || ([[ "$HTTP_CODE" == "400" ]] && [[ "$BODY" =~ (Application\ already\ exists|APP-1022) ]]); then
        log_warning "${APP_NAME} application already exists, skipping"
    else
        log_error "Failed to create ${APP_NAME} application (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
}

create_m2m_application() {
    local APP_NAME="$1"
    local APP_DESCRIPTION="$2"
    local CLIENT_ID="$3"
    local CLIENT_SECRET="$4"
    local OU_ID="$5"
    local API_SCOPES="${6:-}"
    local RESPONSE HTTP_CODE BODY
    local APP_ID APP_CLIENT_ID

    # Resource-server scopes granted to this client (client_credentials takes
    # its scopes directly from the app; this also sets the token audience).
    local M2M_SCOPES_FRAGMENT=""
    [[ -n "$API_SCOPES" ]] && M2M_SCOPES_FRAGMENT="
                \"scopes\": [ ${API_SCOPES} ],"

    log_info "Creating ${APP_NAME} M2M application..."

    read -r -d '' APP_PAYLOAD <<JSON || true
{
    "name": "${APP_NAME}",
    "description": "${APP_DESCRIPTION}",
    "ouId": "${OU_ID}",
    "isRegistrationFlowEnabled": false,
    "assertion": {
        "validityPeriod": 3600
    },
    "inboundAuthConfig": [
        {
            "type": "oauth2",
            "config": {
                "clientId": "${CLIENT_ID}",
                "clientSecret": "${CLIENT_SECRET}",
                "grantTypes": [
                    "client_credentials"
                ],
                "tokenEndpointAuthMethod": "client_secret_basic",
                "pkceRequired": false,
                "publicClient": false,${M2M_SCOPES_FRAGMENT}
                "token": {
                    "accessToken": {
                        "validityPeriod": 3600
                    }
                }
            }
        }
    ],
    "allowedUserTypes": []
}
JSON

    RESPONSE=$(api_call POST "/applications" "${APP_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]] || [[ "$HTTP_CODE" == "202" ]]; then
        log_success "${APP_NAME} M2M application created successfully"
        APP_ID=$(extract_first_id "$BODY")
        APP_CLIENT_ID=$(echo "$BODY" | grep -o '"clientId":"[^"]*"' | head -1 | cut -d'"' -f4)
    elif [[ "$HTTP_CODE" == "409" ]] || ([[ "$HTTP_CODE" == "400" ]] && [[ "$BODY" =~ (Application\ already\ exists|APP-1022) ]]); then
        log_warning "${APP_NAME} M2M application already exists, retrieving ID..."
        APP_ID=$(get_application_id_by_client_id "$CLIENT_ID")
        APP_CLIENT_ID="$CLIENT_ID"
    else
        log_error "Failed to create ${APP_NAME} M2M application (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi

    if [[ -n "$APP_ID" ]]; then
        log_info "${APP_NAME} M2M app ID: ${APP_ID}"
    fi
    if [[ -n "$APP_CLIENT_ID" ]]; then
        log_info "${APP_NAME} M2M client ID: ${APP_CLIENT_ID}"
    fi

    CREATED_M2M_APP_ID="$APP_ID"
}

ensure_user_in_group() {
    local GROUP_ID="$1"
    local USER_ID="$2"
    local GROUP_NAME="$3"
    local USERNAME="$4"
    local RESPONSE HTTP_CODE BODY

    read -r -d '' MEMBERS_ADD_PAYLOAD <<JSON || true
{
    "members": [
        {
            "id": "${USER_ID}",
            "type": "user"
        }
    ]
}
JSON

    RESPONSE=$(api_call POST "/groups/${GROUP_ID}/members/add" "${MEMBERS_ADD_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]] || [[ "$HTTP_CODE" == "204" ]]; then
        log_success "Added ${USERNAME} to group ${GROUP_NAME}"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${USERNAME} is already a member of group ${GROUP_NAME}, skipping"
    else
        log_error "Failed to add ${USERNAME} to group ${GROUP_NAME} (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
}

assign_role_to_group() {
    local ROLE_ID="$1"
    local GROUP_ID="$2"
    local ROLE_NAME="$3"
    local GROUP_NAME="$4"
    local RESPONSE HTTP_CODE BODY

    # Check existing assignments first to avoid server-side unique constraint errors
    RESPONSE=$(api_call GET "/roles/${ROLE_ID}/assignments?type=group")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        if echo "$BODY" | grep -q "\"id\":\"${GROUP_ID}\""; then
            log_warning "Role ${ROLE_NAME} is already assigned to group ${GROUP_NAME}, skipping"
            return
        fi
    fi

    read -r -d '' ROLE_ASSIGNMENT_PAYLOAD <<JSON || true
{
    "assignments": [
        {
            "id": "${GROUP_ID}",
            "type": "group"
        }
    ]
}
JSON

    RESPONSE=$(api_call POST "/roles/${ROLE_ID}/assignments/add" "${ROLE_ASSIGNMENT_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]] || [[ "$HTTP_CODE" == "204" ]]; then
        log_success "Assigned role ${ROLE_NAME} to group ${GROUP_NAME}"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Role ${ROLE_NAME} is already assigned to group ${GROUP_NAME}, skipping"
    elif [[ "$HTTP_CODE" == "500" ]]; then
        if echo "$BODY" | grep -qi "UNIQUE constraint failed"; then
            log_warning "Role ${ROLE_NAME} appears already assigned to group ${GROUP_NAME} (unique constraint), skipping"
        else
            log_error "Failed to assign role ${ROLE_NAME} to group ${GROUP_NAME} (HTTP $HTTP_CODE)"
            echo "Response: $BODY"
            exit 1
        fi
    else
        log_error "Failed to assign role ${ROLE_NAME} to group ${GROUP_NAME} (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
}

assign_role_to_app() {
    local ROLE_ID="$1"
    local APP_ID="$2"
    local ROLE_NAME="$3"
    local APP_NAME="$4"
    local RESPONSE HTTP_CODE BODY

    # A role assigned to an application (type "app") is what makes a
    # client_credentials token carry the role's resource-server permissions as
    # scopes and sets the token audience (aud) to that resource server.
    # Check existing assignments first to avoid server-side unique constraint errors.
    RESPONSE=$(api_call GET "/roles/${ROLE_ID}/assignments?type=app")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"
    if [[ "$HTTP_CODE" == "200" ]] && echo "$BODY" | grep -q "\"id\":\"${APP_ID}\""; then
        log_warning "Role ${ROLE_NAME} is already assigned to app ${APP_NAME}, skipping"
        return
    fi

    read -r -d '' ROLE_APP_ASSIGNMENT_PAYLOAD <<JSON || true
{
    "assignments": [
        {
            "id": "${APP_ID}",
            "type": "app"
        }
    ]
}
JSON

    RESPONSE=$(api_call POST "/roles/${ROLE_ID}/assignments/add" "${ROLE_APP_ASSIGNMENT_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]] || [[ "$HTTP_CODE" == "204" ]]; then
        log_success "Assigned role ${ROLE_NAME} to app ${APP_NAME}"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Role ${ROLE_NAME} is already assigned to app ${APP_NAME}, skipping"
    elif [[ "$HTTP_CODE" == "500" ]] && echo "$BODY" | grep -qi "UNIQUE constraint failed"; then
        log_warning "Role ${ROLE_NAME} appears already assigned to app ${APP_NAME} (unique constraint), skipping"
    else
        log_error "Failed to assign role ${ROLE_NAME} to app ${APP_NAME} (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
}

get_resource_server_id_by_identifier() {
    local IDENTIFIER="$1"
    local RESPONSE HTTP_CODE BODY
    RESPONSE=$(api_call GET "/resource-servers?limit=100&offset=0")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"identifier\":\"${IDENTIFIER}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

# Create a resource server (idempotent). Echoes the resource server ID on stdout
# (log lines go to stderr, so command substitution is safe).
# Usage: create_resource_server <name> <identifier> <ou_id>
create_resource_server() {
    local RS_NAME="$1" RS_IDENTIFIER="$2" RS_OU="$3"
    local RESPONSE HTTP_CODE BODY RID

    RESPONSE=$(api_call POST "/resource-servers" "{\"name\":\"${RS_NAME}\",\"description\":\"${RS_NAME} resource server\",\"identifier\":\"${RS_IDENTIFIER}\",\"ouId\":\"${RS_OU}\"}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        RID=$(extract_first_id "$BODY")
        log_success "Resource server '${RS_IDENTIFIER}' created successfully"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Resource server '${RS_IDENTIFIER}' already exists, retrieving ID..."
        RID=$(get_resource_server_id_by_identifier "${RS_IDENTIFIER}")
    else
        log_error "Failed to create resource server '${RS_IDENTIFIER}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
        exit 1
    fi

    if [[ -z "$RID" ]]; then
        log_error "Could not determine resource server ID for '${RS_IDENTIFIER}'"
        exit 1
    fi
    echo "$RID"
}

# Create a resource under a resource server (idempotent). Echoes the resource ID.
# Usage: create_resource <rs_id> <handle> <name> [parent_id]
create_resource() {
    local RS_ID="$1" R_HANDLE="$2" R_NAME="$3" PARENT="${4:-}"
    local RESPONSE HTTP_CODE BODY RID PARENT_FIELD=""

    [[ -n "$PARENT" ]] && PARENT_FIELD=",\"parent\":\"${PARENT}\""

    RESPONSE=$(api_call POST "/resource-servers/${RS_ID}/resources" "{\"name\":\"${R_NAME}\",\"description\":\"${R_NAME} resource\",\"handle\":\"${R_HANDLE}\"${PARENT_FIELD}}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        RID=$(extract_first_id "$BODY")
        log_success "Resource '${R_HANDLE}' created"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Resource '${R_HANDLE}' already exists, retrieving ID..."
        local Q="/resource-servers/${RS_ID}/resources?limit=100&offset=0"
        [[ -n "$PARENT" ]] && Q="${Q}&parentId=${PARENT}"
        RESPONSE=$(api_call GET "$Q")
        BODY="${RESPONSE%???}"
        RID=$(echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"handle\":\"${R_HANDLE}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    else
        log_error "Failed to create resource '${R_HANDLE}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
        exit 1
    fi

    if [[ -z "$RID" ]]; then
        log_error "Could not determine resource ID for '${R_HANDLE}'"
        exit 1
    fi
    echo "$RID"
}

# Create an action under a resource (idempotent). Derives permission
# "<resource-path>:<handle>". Usage: create_action <rs_id> <resource_id> <handle> <name>
create_action() {
    local RS_ID="$1" RES_ID="$2" A_HANDLE="$3" A_NAME="$4"
    local RESPONSE HTTP_CODE BODY

    RESPONSE=$(api_call POST "/resource-servers/${RS_ID}/resources/${RES_ID}/actions" "{\"name\":\"${A_NAME}\",\"description\":\"${A_NAME} action\",\"handle\":\"${A_HANDLE}\"}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "  action '${A_HANDLE}' created"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "  action '${A_HANDLE}' already exists, skipping"
    else
        log_error "Failed to create action '${A_HANDLE}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
        exit 1
    fi
}

# Provision one government agency end-to-end: child OU + officer user (joined to
# the shared OGA Reviewers group) + portal SPA + outbound (<AGENCY>_TO_NSW) and
# inbound (NSW_TO_<AGENCY>) M2M clients with their role assignments.
#
# Requires globals: GOVERNMENT_ORG_OU_ID, DEFAULT_OU_ID, OGA_REVIEWERS_GROUP_ID,
#   AGENCY_M2M_ROLE_ID, NSW_M2M_ROLE_ID, CLASSIC_THEME_ID/AUTH_FLOW_ID/REG_FLOW_ID,
#   AGENCY_REVIEWER_SCOPES, M2M_NSW_SCOPES, M2M_AGENCY_SCOPES.
#
# Usage: setup_agency <handle> <name> <ou_desc> \
#          <off_user> <off_email> <off_given> <off_family> <off_pass> <off_phone> \
#          <portal_name> <portal_client> <portal_port> \
#          <to_nsw_client> <to_nsw_secret> <nsw_to_client> <nsw_to_secret>
setup_agency() {
    local HANDLE="$1" NAME="$2" OU_DESC="$3"
    local OFF_USER="$4" OFF_EMAIL="$5" OFF_GIVEN="$6" OFF_FAMILY="$7" OFF_PASS="$8" OFF_PHONE="$9"
    local PORTAL_NAME="${10}" PORTAL_CLIENT="${11}" PORTAL_PORT="${12}"
    local TO_NSW_CLIENT="${13}" TO_NSW_SECRET="${14}"
    local NSW_TO_CLIENT="${15}" NSW_TO_SECRET="${16}"
    local PORTAL_REDIRECT_URIS="${17:-}"
    # NOTE: declare then assign on separate lines so a failing $(create_ou ...)
    # subshell (exit 1) still aborts under `set -e` (a `local X=$(...)` would mask it).
    local ou_id officer_id app_id

    echo "" >&2
    log_info "=== Setting up agency: ${NAME} ==="

    # 1. Agency child OU under government-organization
    ou_id=$(create_ou "$HANDLE" "$NAME" "$OU_DESC" "$GOVERNMENT_ORG_OU_ID" "government-organization/${HANDLE}")

    # 2. Officer user (Government_User) in the agency OU
    create_user_in_ou "Government_User" "$ou_id" "$OFF_USER" "$OFF_EMAIL" "$OFF_GIVEN" "$OFF_FAMILY" "$OFF_PASS" "$OFF_PHONE"
    officer_id="$CREATED_USER_ID"

    # 3. Officer joins the shared OGA Reviewers group (grants AGENCY_API via OGA Reviewer role)
    ensure_user_in_group "$OGA_REVIEWERS_GROUP_ID" "$officer_id" "OGA Reviewers" "$OFF_USER"

    # 4. Portal SPA (lives in the agency OU; allowedUserTypes = Government_User)
    create_spa_application "$PORTAL_NAME" "Application for ${NAME} portal built with React" \
        "$PORTAL_CLIENT" "$PORTAL_PORT" "Government_User" "$ou_id" "${AGENCY_REVIEWER_SCOPES}" "${PORTAL_REDIRECT_URIS}"

    # 5. Outbound M2M (<AGENCY>_TO_NSW): client_credentials -> NSW_API, in default OU,
    #    AgencyM2M role assigned to the app (so its token carries aud=NSW_API + scopes).
    create_m2m_application "${TO_NSW_CLIENT}_M2M" "Machine-to-machine integration for ${NAME} to NSW" \
        "$TO_NSW_CLIENT" "$TO_NSW_SECRET" "$DEFAULT_OU_ID" "${M2M_NSW_SCOPES}"
    app_id="$CREATED_M2M_APP_ID"
    assign_role_to_app "$AGENCY_M2M_ROLE_ID" "$app_id" "AgencyM2M" "${TO_NSW_CLIENT}_M2M"

    # 6. Inbound M2M (NSW_TO_<AGENCY>): client_credentials -> AGENCY_API, in government OU,
    #    NswM2M role assigned to the app (token carries aud=AGENCY_API + inject scope).
    create_m2m_application "${NSW_TO_CLIENT}_M2M" "Machine-to-machine integration for NSW to ${NAME}" \
        "$NSW_TO_CLIENT" "$NSW_TO_SECRET" "$GOVERNMENT_ORG_OU_ID" "${M2M_AGENCY_SCOPES}"
    app_id="$CREATED_M2M_APP_ID"
    assign_role_to_app "$NSW_M2M_ROLE_ID" "$app_id" "NswM2M" "${NSW_TO_CLIENT}_M2M"

    log_info "=== Agency ${NAME} complete (OU=${ou_id}) ==="
}

# ============================================================================
# Main
# ============================================================================

log_info "Creating sample Thunder resources (API_BASE=${API_BASE})..."

# ----------------------------------------------------------------------------
# (A) Global / shared resources
#     Resource servers + scopes and the AgencyM2M role must exist before any
#     role grant; theme/flow IDs must exist before any SPA creation.
# ----------------------------------------------------------------------------
echo "" >&2
log_info "Resolving default organization unit for resource servers..."
DEFAULT_OU_ID=$(get_ou_id_by_handle "default")
if [[ -z "$DEFAULT_OU_ID" ]]; then
    log_error "Could not determine default organization unit ID"
    exit 1
fi
log_info "Default OU ID: $DEFAULT_OU_ID"

echo "" >&2
log_info "Creating NSW_API resource server (audience for the OpenNSW/nsw backend)..."
NSW_RS_ID=$(create_resource_server "NSW API" "${NSW_API_IDENTIFIER}" "${DEFAULT_OU_ID}")
NSW_ROOT_RES_ID=$(create_resource "$NSW_RS_ID" "nsw" "NSW API")
RID=$(create_resource "$NSW_RS_ID" "consignment" "Consignment" "$NSW_ROOT_RES_ID")
create_action "$NSW_RS_ID" "$RID" "read" "Read"; create_action "$NSW_RS_ID" "$RID" "write" "Write"
RID=$(create_resource "$NSW_RS_ID" "task" "Task" "$NSW_ROOT_RES_ID")
create_action "$NSW_RS_ID" "$RID" "read" "Read"; create_action "$NSW_RS_ID" "$RID" "write" "Write"
RID=$(create_resource "$NSW_RS_ID" "hscode" "HS Code" "$NSW_ROOT_RES_ID")
create_action "$NSW_RS_ID" "$RID" "read" "Read"
RID=$(create_resource "$NSW_RS_ID" "company" "Company" "$NSW_ROOT_RES_ID")
create_action "$NSW_RS_ID" "$RID" "read" "Read"
RID=$(create_resource "$NSW_RS_ID" "cha" "CHA" "$NSW_ROOT_RES_ID")
create_action "$NSW_RS_ID" "$RID" "read" "Read"
RID=$(create_resource "$NSW_RS_ID" "storage" "Storage" "$NSW_ROOT_RES_ID")
create_action "$NSW_RS_ID" "$RID" "read" "Read"; create_action "$NSW_RS_ID" "$RID" "write" "Write"; create_action "$NSW_RS_ID" "$RID" "delete" "Delete"
log_info "NSW_API resource server ID: $NSW_RS_ID"

echo "" >&2
log_info "Creating AGENCY_API resource server (audience for the OpenNSW/nsw-agency backend)..."
AGENCY_RS_ID=$(create_resource_server "Agency API" "${AGENCY_API_IDENTIFIER}" "${DEFAULT_OU_ID}")
AGENCY_ROOT_RES_ID=$(create_resource "$AGENCY_RS_ID" "agency" "Agency API")
RID=$(create_resource "$AGENCY_RS_ID" "application" "Application" "$AGENCY_ROOT_RES_ID")
create_action "$AGENCY_RS_ID" "$RID" "read" "Read"; create_action "$AGENCY_RS_ID" "$RID" "review" "Review"; create_action "$AGENCY_RS_ID" "$RID" "feedback" "Feedback"; create_action "$AGENCY_RS_ID" "$RID" "inject" "Inject"
RID=$(create_resource "$AGENCY_RS_ID" "consignment" "Consignment" "$AGENCY_ROOT_RES_ID")
create_action "$AGENCY_RS_ID" "$RID" "read" "Read"
RID=$(create_resource "$AGENCY_RS_ID" "storage" "Storage" "$AGENCY_ROOT_RES_ID")
create_action "$AGENCY_RS_ID" "$RID" "read" "Read"; create_action "$AGENCY_RS_ID" "$RID" "write" "Write"
log_info "AGENCY_API resource server ID: $AGENCY_RS_ID"

# AgencyM2M role: granted to each *_TO_NSW client (type "app") so its
# client_credentials token carries aud=NSW_API + the NSW_API scopes.
echo "" >&2
log_info "Creating AgencyM2M role (NSW_API permissions for machine clients)..."
AGENCY_M2M_ROLE_ID=$(create_role "AgencyM2M" "Role for agency machine-to-machine clients calling the NSW API" "$DEFAULT_OU_ID" "$NSW_RS_ID" "${M2M_NSW_SCOPES}")
# (NswM2M role is created in block C — it lives in the government OU.)

# Theme + default flow IDs — consumed by every create_spa_application below
# (blocks B and D), so resolve them up-front in the shared section.
echo "" >&2
log_info "Fetching Classic theme and default flows..."
CLASSIC_THEME_ID=""
RESPONSE=$(api_call GET "/design/themes")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"
if [[ "$HTTP_CODE" == "200" ]]; then
    CLASSIC_THEME_ID=$(echo "$BODY" | grep -o '{[^}]*"displayName":"Classic"[^}]*}' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -n "$CLASSIC_THEME_ID" ]]; then
        log_success "Found Classic theme ID: $CLASSIC_THEME_ID"
    else
        log_warning "Classic theme not found; app creation will continue without theme_id"
    fi
else
    log_warning "Failed to fetch themes (HTTP $HTTP_CODE); app creation will continue without theme_id"
fi

AUTH_FLOW_ID=$(get_flow_id_by_handle "AUTHENTICATION" "default-basic-flow")
REG_FLOW_ID=$(get_flow_id_by_handle "REGISTRATION" "default-basic-flow")

if [[ -n "$AUTH_FLOW_ID" ]]; then
    log_success "Found default authentication flow ID: $AUTH_FLOW_ID"
else
    log_warning "Default authentication flow not found; app creation will continue without auth_flow_id"
fi

if [[ -n "$REG_FLOW_ID" ]]; then
    log_success "Found default registration flow ID: $REG_FLOW_ID"
else
    log_warning "Default registration flow not found; app creation will continue without registration_flow_id"
fi

# ----------------------------------------------------------------------------
# (B) Private-sector domain
#     OUs -> Private_User type -> groups -> roles -> role/group assignments ->
#     users -> group memberships -> TraderApp SPA.
# ----------------------------------------------------------------------------
echo "" >&2
log_info "### Private-sector domain ###"

PRIVATE_SECTOR_OU_ID=$(create_ou "private-sector" "Private Sector" "Organization unit for private sector entities")
ADAM_PVT_LTD_OU_ID=$(create_ou "adam-pvt-ltd" "ADAM PVT LTD" "Child organization unit for ADAM PVT LTD" "$PRIVATE_SECTOR_OU_ID" "private-sector/adam-pvt-ltd")
EDWARD_PVT_LTD_OU_ID=$(create_ou "edward-pvt-ltd" "EDWARD PVT LTD" "Child organization unit for EDWARD PVT LTD" "$PRIVATE_SECTOR_OU_ID" "private-sector/edward-pvt-ltd")

echo "" >&2
log_info "Creating Private_User user type..."
read -r -d '' PRIVATE_USER_TYPE_PAYLOAD <<JSON || true
{
    "name": "Private_User",
    "ouId": "${PRIVATE_SECTOR_OU_ID}",
    "allowSelfRegistration": false,
    "schema": {
        "username": {
            "type": "string",
            "required": true,
            "unique": true
        },
        "password": {
            "type": "string",
            "required": true,
            "credential": true
        },
        "email": {
            "type": "string",
            "required": true,
            "unique": true,
            "regex": "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\\\.[a-zA-Z]{2,}$"
        },
        "phone_number": {
            "type": "string",
            "required": false,
            "regex": "^\\\\+?[1-9]\\\\d{1,14}$"
        },
        "given_name": {
            "type": "string",
            "required": false
        },
        "family_name": {
            "type": "string",
            "required": false
        }
    },
    "systemAttributes": {
        "display": "username"
    }
}
JSON
create_user_type "Private_User" "$PRIVATE_USER_TYPE_PAYLOAD"

echo "" >&2
log_info "Creating private-sector groups and roles..."
TRADERS_GROUP_ID=$(create_group "Traders" "Trader members group" "$PRIVATE_SECTOR_OU_ID")
CHA_GROUP_ID=$(create_group "CHA" "CHA members group" "$PRIVATE_SECTOR_OU_ID")
TRADER_ROLE_ID=$(create_role "Trader" "Role for trader operations" "$PRIVATE_SECTOR_OU_ID" "$NSW_RS_ID" "${TRADER_NSW_SCOPES}")
CHA_ROLE_ID=$(create_role "CHA" "Role for CHA operations" "$PRIVATE_SECTOR_OU_ID" "$NSW_RS_ID" "${TRADER_NSW_SCOPES}")

log_info "Assigning roles to groups..."
assign_role_to_group "$TRADER_ROLE_ID" "$TRADERS_GROUP_ID" "Trader" "Traders"
assign_role_to_group "$CHA_ROLE_ID" "$CHA_GROUP_ID" "CHA" "CHA"

echo "" >&2
log_info "Creating private-sector sample users..."
create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "suresh" "suresh@adam-pvt-ltd.private-sector.dev" "Suresh" "Fernando" "$SURESH_PASSWORD" "+94771234567"
USER_SURESH="$CREATED_USER_ID"
create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "ramesh" "ramesh@adam-pvt-ltd.private-sector.dev" "Ramesh" "Fernando" "$RAMESH_PASSWORD" "+94771234568"
USER_RAMESH="$CREATED_USER_ID"
create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "gomesh" "gomesh@adam-pvt-ltd.private-sector.dev" "Gomesh" "Fernando" "$GOMESH_PASSWORD" "+94771234569"
USER_GOMESH="$CREATED_USER_ID"
create_user_in_ou "Private_User" "$EDWARD_PVT_LTD_OU_ID" "naresh" "naresh@edward-pvt-ltd.private-sector.dev" "Naresh" "Fernando" "$NARESH_PASSWORD" "+94771234570"
USER_NARESH="$CREATED_USER_ID"

log_info "Assigning private-sector users to groups..."
ensure_user_in_group "$TRADERS_GROUP_ID" "$USER_SURESH" "Traders" "suresh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_SURESH" "CHA" "suresh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_RAMESH" "CHA" "ramesh"
ensure_user_in_group "$TRADERS_GROUP_ID" "$USER_GOMESH" "Traders" "gomesh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_NARESH" "CHA" "naresh"

echo "" >&2
log_info "Creating TraderApp SPA (default OU)..."
create_spa_application "TraderApp" "Application for trader portal built with React" "TRADER_PORTAL_APP" "5173" "Private_User" "${DEFAULT_OU_ID}" "${TRADER_NSW_SCOPES}" "${TRADER_REDIRECT_URIS}"

# ----------------------------------------------------------------------------
# (C) Government-shared resources
#     The government root OU, Government_User type, shared OGA Reviewers
#     group/role, and the NswM2M role — all consumed by every agency below.
# ----------------------------------------------------------------------------
echo "" >&2
log_info "### Government-shared resources ###"

GOVERNMENT_ORG_OU_ID=$(create_ou "government-organization" "Government Organization" "Root organization unit for government entities")

echo "" >&2
log_info "Creating Government_User user type..."
read -r -d '' GOVERNMENT_USER_TYPE_PAYLOAD <<JSON || true
{
    "name": "Government_User",
    "ouId": "${GOVERNMENT_ORG_OU_ID}",
    "allowSelfRegistration": false,
    "schema": {
        "username": {
            "type": "string",
            "required": true,
            "unique": true
        },
        "password": {
            "type": "string",
            "required": true,
            "credential": true
        },
        "email": {
            "type": "string",
            "required": true,
            "unique": true,
            "regex": "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\\\.[a-zA-Z]{2,}$"
        },
        "phone_number": {
            "type": "string",
            "required": false
        },
        "given_name": {
            "type": "string",
            "required": false
        },
        "family_name": {
            "type": "string",
            "required": false
        }
    },
    "systemAttributes": {
        "display": "username"
    }
}
JSON
create_user_type "Government_User" "$GOVERNMENT_USER_TYPE_PAYLOAD"

# OGA portal reviewers: one shared group/role grants AGENCY_API. Per-agency
# isolation is enforced by the agency backend via the user's ouHandle, so a
# single shared group/role suffices; officers join it in setup_agency.
echo "" >&2
log_info "Creating shared OGA Reviewers group and role (AGENCY_API permissions)..."
OGA_REVIEWERS_GROUP_ID=$(create_group "OGA Reviewers" "Government agency reviewers group" "$GOVERNMENT_ORG_OU_ID")
OGA_REVIEWER_ROLE_ID=$(create_role "OGA Reviewer" "Role for government agency reviewers (AGENCY_API)" "$GOVERNMENT_ORG_OU_ID" "$AGENCY_RS_ID" "${AGENCY_REVIEWER_SCOPES}")
assign_role_to_group "$OGA_REVIEWER_ROLE_ID" "$OGA_REVIEWERS_GROUP_ID" "OGA Reviewer" "OGA Reviewers"

# NswM2M role: granted to each NSW_TO_* client (type "app") so its
# client_credentials token carries aud=AGENCY_API + agency:application:inject.
echo "" >&2
log_info "Creating NswM2M role (AGENCY_API permissions for machine clients)..."
NSW_M2M_ROLE_ID=$(create_role "NswM2M" "Role for NSW machine-to-machine clients calling the Agency API" "$GOVERNMENT_ORG_OU_ID" "$AGENCY_RS_ID" "${M2M_AGENCY_SCOPES}")

# ----------------------------------------------------------------------------
# (D) Per-agency domains — each call provisions the agency's OU, officer,
#     portal SPA, and both M2M clients (+ role assignments).
# ----------------------------------------------------------------------------
echo "" >&2
log_info "### Per-agency provisioning ###"

setup_agency "npqs" "NPQS" "National Plant Quarantine Service" \
    "npqs_officer" "npqs_officer@government.dev" "NPQS" "Officer" "$NPQS_OFFICER_PASSWORD" "+94771234560" \
    "NPQSPortalApp" "OGA_PORTAL_APP_NPQS" "5174" \
    "NPQS_TO_NSW" "$NPQS_M2M_CLIENT_SECRET" "NSW_TO_NPQS" "$NSW_TO_NPQS_M2M_CLIENT_SECRET" \
    "$NPQS_REDIRECT_URIS"

setup_agency "fcau" "FCAU" "Food Control Administration Unit" \
    "fcau_officer" "fcau_officer@government.dev" "FCAU" "Officer" "$FCAU_OFFICER_PASSWORD" "+94771234561" \
    "FCAUPortalApp" "OGA_PORTAL_APP_FCAU" "5175" \
    "FCAU_TO_NSW" "$FCAU_M2M_CLIENT_SECRET" "NSW_TO_FCAU" "$NSW_TO_FCAU_M2M_CLIENT_SECRET" \
    "$FCAU_REDIRECT_URIS"

setup_agency "cda" "CDA" "Coconut Development Authority" \
    "cda_officer" "cda_officer@government.dev" "CDA" "Officer" "$CDA_OFFICER_PASSWORD" "+94771234563" \
    "CDAPortalApp" "OGA_PORTAL_APP_CDA" "5176" \
    "CDA_TO_NSW" "$CDA_M2M_CLIENT_SECRET" "NSW_TO_CDA" "$NSW_TO_CDA_M2M_CLIENT_SECRET" \
    "$CDA_REDIRECT_URIS"

setup_agency "slpa" "SLPA" "Sri Lanka Ports Authority" \
    "slpa_officer" "slpa_officer@government.dev" "SLPA" "Officer" "$SLPA_OFFICER_PASSWORD" "+94771234564" \
    "SLPAPortalApp" "OGA_PORTAL_APP_SLPA" "5177" \
    "SLPA_TO_NSW" "$SLPA_M2M_CLIENT_SECRET" "NSW_TO_SLPA" "$NSW_TO_SLPA_M2M_CLIENT_SECRET" \
    "$SLPA_REDIRECT_URIS"

setup_agency "customs" "Customs" "Sri Lanka Customs" \
    "customs_officer" "customs_officer@government.dev" "Customs" "Officer" "$CUSTOMS_OFFICER_PASSWORD" "+94771234565" \
    "CustomsPortalApp" "OGA_PORTAL_APP_CUSTOMS" "5178" \
    "CUSTOMS_TO_NSW" "$CUSTOMS_M2M_CLIENT_SECRET" "NSW_TO_CUSTOMS" "$NSW_TO_CUSTOMS_M2M_CLIENT_SECRET" \
    "$CUSTOMS_REDIRECT_URIS"

setup_agency "sltb" "SLTB" "Sri Lanka Tea Board" \
    "sltb_officer" "sltb_officer@government.dev" "SLTB" "Officer" "$SLTB_OFFICER_PASSWORD" "+94771234566" \
    "SLTBPortalApp" "OGA_PORTAL_APP_SLTB" "5179" \
    "SLTB_TO_NSW" "$SLTB_M2M_CLIENT_SECRET" "NSW_TO_SLTB" "$NSW_TO_SLTB_M2M_CLIENT_SECRET" \
    "$SLTB_REDIRECT_URIS"

# ----------------------------------------------------------------------------
# (E) Summary
# ----------------------------------------------------------------------------
echo "" >&2
log_success "Sample resources setup completed successfully!"
log_info "Private Sector OU path: private-sector (children: adam-pvt-ltd, edward-pvt-ltd)"
log_info "Government Organization OU path: government-organization (children: npqs, fcau, cda, slpa, customs, sltb)"
log_info "Private user type: Private_User; Government user type: Government_User"
log_info "Traders group -> Trader role (NSW_API scopes)"
log_info "CHA group -> CHA role (NSW_API scopes)"
log_info "OGA Reviewers group -> OGA Reviewer role (AGENCY_API scopes)"
log_info "suresh in groups: Traders, CHA"
log_info "ramesh in groups: CHA"
log_info "gomesh in groups: Traders"
log_info "naresh (EDWARD PVT LTD) in groups: CHA"
log_info "Government officers: npqs_officer, fcau_officer, cda_officer, slpa_officer, customs_officer, sltb_officer - all in OGA Reviewers group"
log_info "SPA client IDs: TRADER_PORTAL_APP, OGA_PORTAL_APP_NPQS, OGA_PORTAL_APP_FCAU, OGA_PORTAL_APP_CDA, OGA_PORTAL_APP_SLPA, OGA_PORTAL_APP_CUSTOMS, OGA_PORTAL_APP_SLTB"
log_info "M2M client IDs (OGA -> NSW): NPQS_TO_NSW, FCAU_TO_NSW, CDA_TO_NSW, SLPA_TO_NSW, CUSTOMS_TO_NSW, SLTB_TO_NSW"
log_info "M2M client IDs (NSW -> OGA): NSW_TO_NPQS, NSW_TO_FCAU, NSW_TO_CDA, NSW_TO_SLPA, NSW_TO_CUSTOMS, NSW_TO_SLTB"
log_info "M2M roles: AgencyM2M (clients -> NSW_API), NswM2M (clients -> AGENCY_API)"
log_info "M2M auth method: client_secret_basic"
echo "" >&2
log_info "Resource servers (token audiences):"
log_info "  NSW_API    -> TraderApp users (Trader/CHA roles) + *_TO_NSW M2M clients (AgencyM2M role on app)"
log_info "  AGENCY_API -> OGA portal users (OGA Reviewers group / OGA Reviewer role) + NSW_TO_* M2M clients (NswM2M role on app)"
log_info "NSW_API scopes: nsw:{consignment,task,storage}:{read,write,delete}, nsw:{hscode,company,cha}:read"
log_info "AGENCY_API scopes: agency:application:{read,review,feedback,inject}, agency:consignment:read, agency:storage:{read,write}"
echo "" >&2
