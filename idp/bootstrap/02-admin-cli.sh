#!/bin/bash

set -e

# ============================================================================
# Bootstrap script: create the `admin-cli` machine-to-machine application.
#
# Runs inside the `thunderid-setup` container (mounted to
# /opt/thunderid/bootstrap/02-admin-cli.sh), after 01-default-resources.sh, with
# security disabled — so no auth token is needed. It sources the image's
# common.sh for api_call / log_*.
#
# It creates one M2M client in the default OU:
#   name=admin-cli  client_id=ADMIN_CLI  client_secret=${ADMIN_CLI_SECRET:-1234}
# and assigns the existing `Administrator` role (created by 01-default-resources)
# to it. That role grant is what makes the client's client_credentials token
# carry management ("system") scope + audience, e.g.:
#
#   curl -k -X POST https://localhost:8090/oauth2/token \
#     -u "ADMIN_CLI:1234" -H "Content-Type: application/x-www-form-urlencoded" \
#     -d "grant_type=client_credentials" -d "scope=system"
#
# The resulting access_token is used as AUTH_TOKEN for idp/sample-resources.sh.
# ============================================================================

# Source common functions from the same directory as this script
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]:-$0}")"
source "${SCRIPT_DIR}/common.sh"

ADMIN_CLI_SECRET="${ADMIN_CLI_SECRET:-1234}"

# ============================================================================
# Helpers (api_call / log_* come from common.sh)
# ============================================================================

extract_first_id() {
    echo "$1" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
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

# ============================================================================
# Main — create admin-cli and grant it the Administrator role
# ============================================================================
log_info "Creating admin-cli M2M application in the default OU..."

DEFAULT_OU_ID=$(get_ou_id_by_handle "default")
if [[ -z "$DEFAULT_OU_ID" ]]; then
    log_error "Could not determine default organization unit ID"
    exit 1
fi
log_info "Default OU ID: $DEFAULT_OU_ID"

ADMIN_ROLE_ID=$(get_role_id_by_name "Administrator" "$DEFAULT_OU_ID")
if [[ -z "$ADMIN_ROLE_ID" ]]; then
    log_error "Administrator role not found in the default OU (expected from 01-default-resources.sh)"
    exit 1
fi
log_info "Administrator role ID: $ADMIN_ROLE_ID"

# Create the M2M client (no requestable scopes — the Administrator role grant
# is what emits the token's scopes and audience).
create_m2m_application "admin-cli" "Admin CLI machine client for management API access" \
    "ADMIN_CLI" "$ADMIN_CLI_SECRET" "$DEFAULT_OU_ID" ""

assign_role_to_app "$ADMIN_ROLE_ID" "$CREATED_M2M_APP_ID" "Administrator" "admin-cli"

log_success "admin-cli application ready (client_id=ADMIN_CLI). Obtain a management token via:"
log_info "  curl -k -X POST <API_BASE>/oauth2/token -u 'ADMIN_CLI:<secret>' -d grant_type=client_credentials -d scope=system"
