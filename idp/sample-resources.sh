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
  ALLOW_DEFAULT_SECRETS
                Set to 1 to allow the insecure "1234" fallback for unset secrets
                on a NON-localhost target. Default 0: an unset SAMPLE_USER_PASSWORD
                or M2M_CLIENT_SECRET fails fast (exit non-zero) rather than seeding
                a default credential. Localhost targets always allow the fallback.

  SAMPLE_USER_PASSWORD, M2M_CLIENT_SECRET, and the per-entity overrides documented
  in idp/.env.example tune the seeded secrets. Values in idp/.env are loaded
  automatically and take precedence. When unset, they fall back to "1234" only for
  localhost or ALLOW_DEFAULT_SECRETS=1 runs; otherwise the script exits.

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
ALLOW_DEFAULT_SECRETS="${ALLOW_DEFAULT_SECRETS:-0}"

# ============================================================================
# Shared engine library — logging, is_localhost, api_call, JSON lookups
# (get_*/list_*/extract_first_id), key->ID registry, config load/merge, agency
# expansion, secret + scope-set resolution. Sourced after SCRIPT_DIR and the
# API_BASE / AUTH_TOKEN / INSECURE config vars are set above (api_call reads them).
# shellcheck source=resources-lib.sh
# ============================================================================
source "${SCRIPT_DIR}/resources-lib.sh"

# ============================================================================
# Auth guard + one-time auth/connectivity probe
# ============================================================================
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

# ============================================================================
# Shared fallback secrets (overridable via env / .env). Per-entity passwords,
# M2M client secrets, and per-SPA redirect URIs are referenced BY ENV-VAR NAME
# from the resources/ config and resolved at provisioning time (resolve_secret);
# only these two shared fallbacks live in the script. Guarded here — BEFORE the
# network probe — so a misconfigured non-localhost run fails fast (naming the
# missing var) without falling back to the insecure default "1234". Localhost /
# ALLOW_DEFAULT_SECRETS=1 keep the dev convenience default (see resources-lib.sh).
# ============================================================================
require_secret_or_default SAMPLE_USER_PASSWORD "sample user password"
require_secret_or_default M2M_CLIENT_SECRET   "M2M client secret"

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
# Entity create/assign helpers (idempotent). api_call, the get_*/list_* lookups,
# and extract_first_id are provided by resources-lib.sh (sourced above).
# ============================================================================

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
#   permissions-fragment is the comma-separated quoted scope list produced by
#   scopeset_fragment (e.g. '"nsw:task:read", "nsw:task:write"').
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

    local RESPONSE HTTP_CODE BODY USER_ID USER_PAYLOAD

    # Build with jq --arg so any special characters in the attribute values are
    # safely JSON-encoded (the create_user_in_ou values are the most free-form).
    USER_PAYLOAD=$(jq -n \
        --arg type "$USER_TYPE" --arg ou "$OU_ID" \
        --arg u "$USERNAME" --arg p "$PASSWORD" --arg e "$EMAIL" \
        --arg gn "$GIVEN_NAME" --arg fn "$FAMILY_NAME" --arg ph "$PHONE_NUMBER" \
        '{type: $type, ouId: $ou, attributes: {username: $u, password: $p, email: $e, given_name: $gn, family_name: $fn, phone_number: $ph}}')

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
        echo "Response: $BODY" >&2
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
        APP_CLIENT_ID=$(printf '%s' "$BODY" | jq -r '[.. | objects | .clientId?] | map(select(. != null)) | .[0] // empty')
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
        echo "Response: $BODY" >&2
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
        APP_CLIENT_ID=$(printf '%s' "$BODY" | jq -r '[.. | objects | .clientId?] | map(select(. != null)) | .[0] // empty')
    elif [[ "$HTTP_CODE" == "409" ]] || ([[ "$HTTP_CODE" == "400" ]] && [[ "$BODY" =~ (Application\ already\ exists|APP-1022) ]]); then
        log_warning "${APP_NAME} M2M application already exists, retrieving ID..."
        APP_ID=$(get_application_id_by_client_id "$CLIENT_ID")
        APP_CLIENT_ID="$CLIENT_ID"
    else
        log_error "Failed to create ${APP_NAME} M2M application (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
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
        echo "Response: $BODY" >&2
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
    if role_has_assignment "$ROLE_ID" group "$GROUP_ID"; then
        log_warning "Role ${ROLE_NAME} is already assigned to group ${GROUP_NAME}, skipping"
        return
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
            echo "Response: $BODY" >&2
            exit 1
        fi
    else
        log_error "Failed to assign role ${ROLE_NAME} to group ${GROUP_NAME} (HTTP $HTTP_CODE)"
        echo "Response: $BODY" >&2
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
    if role_has_assignment "$ROLE_ID" app "$APP_ID"; then
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
        echo "Response: $BODY" >&2
        exit 1
    fi
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
        RID=$(get_resource_id_by_handle "$RS_ID" "$R_HANDLE" "$PARENT")
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

# ============================================================================
# Engine — bootstrap + one provisioning pass per entity type. All data comes
# from idp/resources/ (merged into $MERGED by load_config). Each pass resolves
# cross-entity references via reg_require and reuses the create_* helpers above.
# bash-3.2 rules: iterate jq arrays with here-strings (never pipes), and
# declare-then-assign every $(create_*) so `set -e` still aborts on failure.
# ============================================================================

# Resolve image-provided defaults the config references but does not create:
# the `default` OU (registered as ou:default) plus the Classic theme + default
# auth/registration flow IDs (globals consumed by create_spa_application).
bootstrap_registry() {
    local id RESPONSE HTTP_CODE BODY
    id="$(get_ou_id_by_handle "default")"
    [[ -n "$id" ]] || { log_error "Could not determine default organization unit ID"; exit 1; }
    reg_set "ou:default" "$id"
    log_info "Default OU ID: $id"

    CLASSIC_THEME_ID=""
    RESPONSE=$(api_call GET "/design/themes")
    HTTP_CODE="${RESPONSE: -3}"; BODY="${RESPONSE%???}"
    if [[ "$HTTP_CODE" == "200" ]]; then
        CLASSIC_THEME_ID=$(printf '%s' "$BODY" | jq -r '[.themes[]? | select(.displayName == "Classic") | .id] | .[0] // empty')
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
}

# (1) Resource servers + their nested resources -> actions (depth-first).
_process_resource_tree() {
    local rs_id="$1" parent_id="$2" node="$3"
    local handle name res_id act a_handle a_name kid
    handle="$(jq -r '.handle' <<< "$node")"
    name="$(jq -r '.name' <<< "$node")"
    res_id="$(create_resource "$rs_id" "$handle" "$name" "$parent_id")"
    while IFS= read -r act; do
        [[ -z "$act" ]] && continue
        a_handle="$(jq -r '.handle' <<< "$act")"
        a_name="$(jq -r '.name' <<< "$act")"
        create_action "$rs_id" "$res_id" "$a_handle" "$a_name"
    done <<< "$(jq -c '.actions // [] | .[]' <<< "$node")"
    while IFS= read -r kid; do
        [[ -z "$kid" ]] && continue
        _process_resource_tree "$rs_id" "$res_id" "$kid"
    done <<< "$(jq -c '.resources // [] | .[]' <<< "$node")"
}

process_resource_servers() {
    local obj name identifier ou_key ou_id rs_id res
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        name="$(jq -r '.name' <<< "$obj")"
        identifier="$(jq -r '.identifier' <<< "$obj")"
        ou_key="$(jq -r '.ou' <<< "$obj")"
        ou_id="$(reg_require "ou:${ou_key}")"
        rs_id="$(create_resource_server "$name" "$identifier" "$ou_id")"
        reg_set "rs:${identifier}" "$rs_id"
        log_info "${identifier} resource server ID: $rs_id"
        while IFS= read -r res; do
            [[ -z "$res" ]] && continue
            _process_resource_tree "$rs_id" "" "$res"
        done <<< "$(jq -c '.resources // [] | .[]' <<< "$obj")"
    done <<< "$(jq -c '.resourceServers // [] | .[]' <<< "$MERGED")"
}

# (2) Organization units, parents before children (sort by treePath '/'-depth).
process_ous() {
    local line obj key handle name desc tree parent_key parent_id ou_id
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        obj="${line#*$'\t'}"
        key="$(jq -r '.key' <<< "$obj")"
        handle="$(jq -r '.handle' <<< "$obj")"
        name="$(jq -r '.name' <<< "$obj")"
        desc="$(jq -r '.description // .name' <<< "$obj")"
        tree="$(jq -r '.treePath // .handle' <<< "$obj")"
        parent_key="$(jq -r '.parent // empty' <<< "$obj")"
        parent_id=""
        [[ -n "$parent_key" ]] && parent_id="$(reg_require "ou:${parent_key}")"
        ou_id="$(create_ou "$handle" "$name" "$desc" "$parent_id" "$tree")"
        reg_set "ou:${key}" "$ou_id"
    done <<< "$(jq -r '.organizationUnits // [] | .[] | [ ((.treePath // .handle) | [scan("/")] | length), tojson ] | "\(.[0])\t\(.[1])"' <<< "$MERGED" | sort -n -k1,1 -s)"
}

# (3) User types (the schema object is passed through to the API verbatim).
process_user_types() {
    local obj name ou_key ou_id payload
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        name="$(jq -r '.name' <<< "$obj")"
        ou_key="$(jq -r '.ou' <<< "$obj")"
        ou_id="$(reg_require "ou:${ou_key}")"
        payload="$(jq -c --arg ou "$ou_id" '{name, ouId: $ou, allowSelfRegistration, schema, systemAttributes}' <<< "$obj")"
        create_user_type "$name" "$payload"
    done <<< "$(jq -c '.userTypes // [] | .[]' <<< "$MERGED")"
}

# (4) Groups.
process_groups() {
    local obj key name desc ou_key ou_id gid
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        key="$(jq -r '.key' <<< "$obj")"
        name="$(jq -r '.name' <<< "$obj")"
        desc="$(jq -r '.description // (.name + " group")' <<< "$obj")"
        ou_key="$(jq -r '.ou' <<< "$obj")"
        ou_id="$(reg_require "ou:${ou_key}")"
        gid="$(create_group "$name" "$desc" "$ou_id")"
        reg_set "group:${key}" "$gid"
    done <<< "$(jq -c '.groups // [] | .[]' <<< "$MERGED")"
}

# (5) Roles (resolve resource server + scope set).
process_roles() {
    local obj key name desc ou_key ou_id rs_key rs_id scopeset perms rid
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        key="$(jq -r '.key' <<< "$obj")"
        name="$(jq -r '.name' <<< "$obj")"
        desc="$(jq -r '.description // .name' <<< "$obj")"
        ou_key="$(jq -r '.ou' <<< "$obj")"
        ou_id="$(reg_require "ou:${ou_key}")"
        rs_key="$(jq -r '.resourceServer' <<< "$obj")"
        rs_id="$(reg_require "rs:${rs_key}")"
        scopeset="$(jq -r '.scopeSet' <<< "$obj")"
        perms="$(scopeset_fragment "$scopeset")"
        rid="$(create_role "$name" "$desc" "$ou_id" "$rs_id" "$perms")"
        reg_set "role:${key}" "$rid"
    done <<< "$(jq -c '.roles // [] | .[]' <<< "$MERGED")"
}

# (6) Role -> group assignments.
process_role_group_assignments() {
    local obj role_key group_key role_id group_id
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        role_key="$(jq -r '.role' <<< "$obj")"
        group_key="$(jq -r '.group' <<< "$obj")"
        role_id="$(reg_require "role:${role_key}")"
        group_id="$(reg_require "group:${group_key}")"
        assign_role_to_group "$role_id" "$group_id" "$role_key" "$group_key"
    done <<< "$(jq -c '.roleAssignments // [] | .[]' <<< "$MERGED")"
}

# (7) Users + inline group memberships.
process_users() {
    local obj key utype ou_key ou_id username email given family phone password uid g gid
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        key="$(jq -r '.key' <<< "$obj")"
        utype="$(jq -r '.type' <<< "$obj")"
        ou_key="$(jq -r '.ou' <<< "$obj")"
        ou_id="$(reg_require "ou:${ou_key}")"
        username="$(jq -r '.username' <<< "$obj")"
        email="$(jq -r '.email' <<< "$obj")"
        given="$(jq -r '.givenName' <<< "$obj")"
        family="$(jq -r '.familyName' <<< "$obj")"
        phone="$(jq -r '.phoneNumber' <<< "$obj")"
        password="$(resolve_secret "$(jq -c '.passwordEnv' <<< "$obj")")"
        create_user_in_ou "$utype" "$ou_id" "$username" "$email" "$given" "$family" "$password" "$phone"
        uid="$CREATED_USER_ID"
        reg_set "user:${key}" "$uid"
        while IFS= read -r g; do
            [[ -z "$g" ]] && continue
            gid="$(reg_require "group:${g}")"
            ensure_user_in_group "$gid" "$uid" "$g" "$username"
        done <<< "$(jq -r '.groups // [] | .[]' <<< "$obj")"
    done <<< "$(jq -c '.users // [] | .[]' <<< "$MERGED")"
}

# (8) Applications (SPA + M2M). M2M app IDs are registered for pass (9).
process_applications() {
    local obj kind name desc client_id ou_key ou_id scopeset scopes
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        kind="$(jq -r '.kind' <<< "$obj")"
        name="$(jq -r '.name' <<< "$obj")"
        desc="$(jq -r '.description' <<< "$obj")"
        client_id="$(jq -r '.clientId' <<< "$obj")"
        ou_key="$(jq -r '.ou' <<< "$obj")"
        ou_id="$(reg_require "ou:${ou_key}")"
        scopeset="$(jq -r '.scopeSet // empty' <<< "$obj")"
        scopes=""
        [[ -n "$scopeset" ]] && scopes="$(scopeset_fragment "$scopeset")"
        if [[ "$kind" == "spa" ]]; then
            local port allowed_type redirect_env redirect_uris
            port="$(jq -r '.port' <<< "$obj")"
            allowed_type="$(jq -r '.allowedUserType' <<< "$obj")"
            redirect_env="$(jq -r '.redirectUrisEnv // empty' <<< "$obj")"
            redirect_uris=""
            [[ -n "$redirect_env" ]] && redirect_uris="${!redirect_env:-}"
            create_spa_application "$name" "$desc" "$client_id" "$port" "$allowed_type" "$ou_id" "$scopes" "$redirect_uris"
        elif [[ "$kind" == "m2m" ]]; then
            local secret
            secret="$(resolve_secret "$(jq -c '.secretEnv' <<< "$obj")")"
            create_m2m_application "$name" "$desc" "$client_id" "$secret" "$ou_id" "$scopes"
            reg_set "app:${client_id}" "$CREATED_M2M_APP_ID"
        else
            log_error "unknown application kind '$kind' for ${client_id}"
            exit 1
        fi
    done <<< "$(jq -c '.applications // [] | .[]' <<< "$MERGED")"
}

# (9) Role -> application assignments (makes an M2M token carry the role's
# resource-server scopes and sets its audience).
process_app_role_assignments() {
    local obj role_key app_key role_id app_id
    while IFS= read -r obj; do
        [[ -z "$obj" ]] && continue
        role_key="$(jq -r '.role' <<< "$obj")"
        app_key="$(jq -r '.app' <<< "$obj")"
        role_id="$(reg_require "role:${role_key}")"
        app_id="$(reg_require "app:${app_key}")"
        assign_role_to_app "$role_id" "$app_id" "$role_key" "${app_key}_M2M"
    done <<< "$(jq -c '.appRoleAssignments // [] | .[]' <<< "$MERGED")"
}

# ============================================================================
# Main
# ============================================================================
log_info "Creating sample Thunder resources (API_BASE=${API_BASE})..."

load_config

echo "" >&2; log_info "### Bootstrap (image defaults) ###"
bootstrap_registry

echo "" >&2; log_info "### Resource servers ###"
process_resource_servers

echo "" >&2; log_info "### Organization units ###"
process_ous

echo "" >&2; log_info "### User types ###"
process_user_types

echo "" >&2; log_info "### Groups ###"
process_groups

echo "" >&2; log_info "### Roles ###"
process_roles

echo "" >&2; log_info "### Role -> group assignments ###"
process_role_group_assignments

echo "" >&2; log_info "### Users + group memberships ###"
process_users

echo "" >&2; log_info "### Applications (SPA + M2M) ###"
process_applications

echo "" >&2; log_info "### Role -> application assignments ###"
process_app_role_assignments

# ============================================================================
# Summary (derived from config)
# ============================================================================
echo "" >&2
log_success "Sample resources setup completed successfully!"
log_info "Organization units: $(jq -r '[.organizationUnits[].treePath] | join(", ")' <<< "$MERGED")"
log_info "User types: $(jq -r '[.userTypes[].name] | join(", ")' <<< "$MERGED")"
log_info "Groups: $(jq -r '[.groups[].name] | join(", ")' <<< "$MERGED")"
log_info "Roles: $(jq -r '[.roles[].name] | join(", ")' <<< "$MERGED")"
log_info "Resource servers (token audiences): $(jq -r '[.resourceServers[].identifier] | join(", ")' <<< "$MERGED")"
log_info "SPA client IDs: $(jq -r '[.applications[] | select(.kind=="spa") | .clientId] | join(", ")' <<< "$MERGED")"
log_info "M2M client IDs: $(jq -r '[.applications[] | select(.kind=="m2m") | .clientId] | join(", ")' <<< "$MERGED")"
log_info "M2M role/app assignments: $(jq -r '[.appRoleAssignments[] | "\(.role)->\(.app)"] | join(", ")' <<< "$MERGED")"
echo "" >&2
