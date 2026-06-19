#!/bin/bash

set -e

# Source common functions from the same directory as this script
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]:-$0}")"
source "${SCRIPT_DIR}/common.sh"

# Load .env values when available (useful for local execution).
ENV_FILE="${SCRIPT_DIR}/.env"
if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

SAMPLE_USER_PASSWORD="${SAMPLE_USER_PASSWORD:-1234}"
SURESH_PASSWORD="${SAMPLE_SURESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
RAMESH_PASSWORD="${SAMPLE_RAMESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
GOMESH_PASSWORD="${SAMPLE_GOMESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
NARESH_PASSWORD="${SAMPLE_NARESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
NPQS_OFFICER_PASSWORD="${SAMPLE_NPQS_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
FCAU_OFFICER_PASSWORD="${SAMPLE_FCAU_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
CDA_OFFICER_PASSWORD="${SAMPLE_CDA_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
SLPA_OFFICER_PASSWORD="${SAMPLE_SLPA_OFFICER_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
M2M_CLIENT_SECRET="${M2M_CLIENT_SECRET:-1234}"
# Outbound (Agency -> NSW) M2M client secrets — one per *_TO_NSW client.
NPQS_M2M_CLIENT_SECRET="${M2M_NPQS_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
FCAU_M2M_CLIENT_SECRET="${M2M_FCAU_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
CDA_M2M_CLIENT_SECRET="${M2M_CDA_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
SLPA_M2M_CLIENT_SECRET="${M2M_SLPA_TO_NSW_SECRET:-${M2M_CLIENT_SECRET}}"
# Inbound (NSW -> Agency) M2M client secrets — one per NSW_TO_* client.
NSW_TO_NPQS_M2M_CLIENT_SECRET="${M2M_NSW_TO_NPQS_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_FCAU_M2M_CLIENT_SECRET="${M2M_NSW_TO_FCAU_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_CDA_M2M_CLIENT_SECRET="${M2M_NSW_TO_CDA_SECRET:-${M2M_CLIENT_SECRET}}"
NSW_TO_SLPA_M2M_CLIENT_SECRET="${M2M_NSW_TO_SLPA_SECRET:-${M2M_CLIENT_SECRET}}"

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

log_info "Creating sample Thunder resources..."
echo ""

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
    RESPONSE=$(api_call GET "/applications?limit=200&offset=0")
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

create_spa_application() {
    local APP_NAME="$1"
    local APP_DESCRIPTION="$2"
    local CLIENT_ID="$3"
    local PORT="$4"
    local ALLOWED_USER_TYPE="$5"
    local OU_ID="$6"
    local API_SCOPES="${7:-}"
    local RESPONSE HTTP_CODE BODY
    local APP_ID APP_CLIENT_ID

    # Resource-server scopes this app may request (sets the token audience).
    local API_SCOPES_FRAGMENT=""
    [[ -n "$API_SCOPES" ]] && API_SCOPES_FRAGMENT=",
                    ${API_SCOPES}"

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
                    "http://localhost:${PORT}",
                    "https://localhost:${PORT}"
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
    RESPONSE=$(api_call GET "/resource-servers?limit=200&offset=0")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" != "200" ]]; then
        echo ""
        return
    fi

    echo "$BODY" | sed 's/},{/}\n{/g' | grep "\"identifier\":\"${IDENTIFIER}\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

# Create a resource server (idempotent). Echoes the resource server ID on stdout
# (log lines go to stderr via common.sh, so command substitution is safe).
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
        local Q="/resource-servers/${RS_ID}/resources?limit=200&offset=0"
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

# ============================================================================
# Create OAuth2 Resource Servers (NSW_API, AGENCY_API)
# ============================================================================
# Created before roles/applications so their IDs and derived scopes are
# available when granting role permissions and configuring client scopes.

log_info "Resolving default organization unit for resource servers..."
DEFAULT_OU_ID=$(get_ou_id_by_handle "default")
if [[ -z "$DEFAULT_OU_ID" ]]; then
    log_error "Could not determine default organization unit ID"
    exit 1
fi
log_info "Default OU ID: $DEFAULT_OU_ID"
echo ""

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
echo ""

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
echo ""

# ============================================================================
# Create AgencyM2M Role (granted to M2M clients calling the NSW API)
# ============================================================================
# Machine-to-machine (client_credentials) clients have no user, so their token
# scopes and audience come from a role assigned to the APPLICATION (type "app").
# This role grants the NSW_API scopes; assigning it to each *_TO_NSW client
# (below) makes their tokens carry aud=NSW_API plus the granted scopes.

log_info "Creating AgencyM2M role (NSW_API permissions for machine clients)..."
read -r -d '' AGENCY_M2M_ROLE_PAYLOAD <<JSON || true
{
    "name": "AgencyM2M",
    "description": "Role for agency machine-to-machine clients calling the NSW API",
    "ouId": "${DEFAULT_OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${NSW_RS_ID}",
            "permissions": [ ${M2M_NSW_SCOPES} ]
        }
    ]
}
JSON
RESPONSE=$(api_call POST "/roles" "${AGENCY_M2M_ROLE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"
if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "AgencyM2M role created successfully"
    AGENCY_M2M_ROLE_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "AgencyM2M role already exists, retrieving ID..."
    AGENCY_M2M_ROLE_ID=$(get_role_id_by_name "AgencyM2M" "$DEFAULT_OU_ID")
else
    log_error "Failed to create AgencyM2M role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi
if [[ -z "$AGENCY_M2M_ROLE_ID" ]]; then
    log_error "Could not determine AgencyM2M role ID"
    exit 1
fi
log_info "AgencyM2M role ID: $AGENCY_M2M_ROLE_ID"
echo ""

# ============================================================================
# Create Private Sector Organization Unit
# ============================================================================

PRIVATE_SECTOR_OU_HANDLE="private-sector"

log_info "Creating Private Sector organization unit..."

read -r -d '' PRIVATE_SECTOR_OU_PAYLOAD <<JSON || true
{
    "handle": "${PRIVATE_SECTOR_OU_HANDLE}",
    "name": "Private Sector",
    "description": "Organization unit for private sector entities"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${PRIVATE_SECTOR_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Private Sector organization unit created successfully"
    PRIVATE_SECTOR_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Private Sector organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${PRIVATE_SECTOR_OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        PRIVATE_SECTOR_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch organization unit by handle '${PRIVATE_SECTOR_OU_HANDLE}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create Private Sector organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$PRIVATE_SECTOR_OU_ID" ]]; then
    log_error "Could not determine Private Sector organization unit ID"
    exit 1
fi

log_info "Private Sector OU ID: $PRIVATE_SECTOR_OU_ID"

echo ""

# ============================================================================
# Create ADAM PVT LTD Child Organization Unit
# ============================================================================

ADAM_PVT_LTD_OU_HANDLE="adam-pvt-ltd"
ADAM_PVT_LTD_OU_PATH="${PRIVATE_SECTOR_OU_HANDLE}/${ADAM_PVT_LTD_OU_HANDLE}"

log_info "Creating ADAM PVT LTD child organization unit..."

read -r -d '' ADAM_PVT_LTD_OU_PAYLOAD <<JSON || true
{
    "handle": "${ADAM_PVT_LTD_OU_HANDLE}",
    "name": "ADAM PVT LTD",
    "description": "Child organization unit for ADAM PVT LTD",
    "parent": "${PRIVATE_SECTOR_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${ADAM_PVT_LTD_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "ADAM PVT LTD organization unit created successfully"
    ADAM_PVT_LTD_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "ADAM PVT LTD organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${ADAM_PVT_LTD_OU_PATH}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        ADAM_PVT_LTD_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch organization unit by path '${ADAM_PVT_LTD_OU_PATH}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create ADAM PVT LTD organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$ADAM_PVT_LTD_OU_ID" ]]; then
    log_error "Could not determine ADAM PVT LTD organization unit ID"
    exit 1
fi

log_info "ADAM PVT LTD OU ID: $ADAM_PVT_LTD_OU_ID"

echo ""

# ============================================================================
# Create EDWARD PVT LTD Child Organization Unit
# ============================================================================

EDWARD_PVT_LTD_OU_HANDLE="edward-pvt-ltd"
EDWARD_PVT_LTD_OU_PATH="${PRIVATE_SECTOR_OU_HANDLE}/${EDWARD_PVT_LTD_OU_HANDLE}"

log_info "Creating EDWARD PVT LTD child organization unit..."

read -r -d '' EDWARD_PVT_LTD_OU_PAYLOAD <<JSON || true
{
    "handle": "${EDWARD_PVT_LTD_OU_HANDLE}",
    "name": "EDWARD PVT LTD",
    "description": "Child organization unit for EDWARD PVT LTD",
    "parent": "${PRIVATE_SECTOR_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${EDWARD_PVT_LTD_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "EDWARD PVT LTD organization unit created successfully"
    EDWARD_PVT_LTD_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "EDWARD PVT LTD organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${EDWARD_PVT_LTD_OU_PATH}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        EDWARD_PVT_LTD_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch organization unit by path '${EDWARD_PVT_LTD_OU_PATH}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create EDWARD PVT LTD organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$EDWARD_PVT_LTD_OU_ID" ]]; then
    log_error "Could not determine EDWARD PVT LTD organization unit ID"
    exit 1
fi

log_info "EDWARD PVT LTD OU ID: $EDWARD_PVT_LTD_OU_ID"

echo ""

# ============================================================================
# Create Government Organization and Child OUs
# ============================================================================

GOVERNMENT_ORG_OU_HANDLE="government-organization"
NPQS_OU_HANDLE="npqs"
FCAU_OU_HANDLE="fcau"
CDA_OU_HANDLE="cda"
SLPA_OU_HANDLE="slpa"

log_info "Creating Government Organization root organization unit..."

read -r -d '' GOVERNMENT_ORG_OU_PAYLOAD <<JSON || true
{
    "handle": "${GOVERNMENT_ORG_OU_HANDLE}",
    "name": "Government Organization",
    "description": "Root organization unit for government entities"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${GOVERNMENT_ORG_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Government Organization created successfully"
    GOVERNMENT_ORG_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Government Organization already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${GOVERNMENT_ORG_OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        GOVERNMENT_ORG_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch organization unit by path '${GOVERNMENT_ORG_OU_HANDLE}' (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create Government Organization (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$GOVERNMENT_ORG_OU_ID" ]]; then
    log_error "Could not determine Government Organization ID"
    exit 1
fi

log_info "Government Organization OU ID: $GOVERNMENT_ORG_OU_ID"

echo ""
log_info "Creating NPQS organization unit..."

read -r -d '' NPQS_OU_PAYLOAD <<JSON || true
{
    "handle": "${NPQS_OU_HANDLE}",
    "name": "NPQS",
    "description": "National Plant Quarantine Service",
    "parent": "${GOVERNMENT_ORG_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${NPQS_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "NPQS organization unit created successfully"
    NPQS_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "NPQS organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${GOVERNMENT_ORG_OU_HANDLE}/${NPQS_OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        NPQS_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch NPQS OU (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create NPQS organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$NPQS_OU_ID" ]]; then
    log_error "Could not determine NPQS organization unit ID"
    exit 1
fi

log_info "NPQS OU ID: $NPQS_OU_ID"

echo ""
log_info "Creating FCAU organization unit..."

read -r -d '' FCAU_OU_PAYLOAD <<JSON || true
{
    "handle": "${FCAU_OU_HANDLE}",
    "name": "FCAU",
    "description": "Food Control Administration Unit",
    "parent": "${GOVERNMENT_ORG_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${FCAU_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "FCAU organization unit created successfully"
    FCAU_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "FCAU organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${GOVERNMENT_ORG_OU_HANDLE}/${FCAU_OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        FCAU_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch FCAU OU (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create FCAU organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$FCAU_OU_ID" ]]; then
    log_error "Could not determine FCAU organization unit ID"
    exit 1
fi

log_info "FCAU OU ID: $FCAU_OU_ID"

echo ""
log_info "Creating CDA organization unit..."

read -r -d '' CDA_OU_PAYLOAD <<JSON || true
{
    "handle": "${CDA_OU_HANDLE}",
    "name": "CDA",
    "description": "Coconut Development Authority",
    "parent": "${GOVERNMENT_ORG_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${CDA_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "CDA organization unit created successfully"
    CDA_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "CDA organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${GOVERNMENT_ORG_OU_HANDLE}/${CDA_OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        CDA_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch CDA OU (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create CDA organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$CDA_OU_ID" ]]; then
    log_error "Could not determine CDA organization unit ID"
    exit 1
fi

log_info "CDA OU ID: $CDA_OU_ID"

echo ""
log_info "Creating SLPA organization unit..."

read -r -d '' SLPA_OU_PAYLOAD <<JSON || true
{
    "handle": "${SLPA_OU_HANDLE}",
    "name": "SLPA",
    "description": "Sri Lanka Ports Authority",
    "parent": "${GOVERNMENT_ORG_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/organization-units" "${SLPA_OU_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "SLPA organization unit created successfully"
    SLPA_OU_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "SLPA organization unit already exists, retrieving ID..."
    RESPONSE=$(api_call GET "/organization-units/tree/${GOVERNMENT_ORG_OU_HANDLE}/${SLPA_OU_HANDLE}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        SLPA_OU_ID=$(extract_first_id "$BODY")
    else
        log_error "Failed to fetch SLPA OU (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi
else
    log_error "Failed to create SLPA organization unit (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$SLPA_OU_ID" ]]; then
    log_error "Could not determine SLPA organization unit ID"
    exit 1
fi

log_info "SLPA OU ID: $SLPA_OU_ID"

echo ""

# ============================================================================
# Create Private_User User Type
# ============================================================================

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

RESPONSE=$(api_call POST "/user-types" "${PRIVATE_USER_TYPE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Private_User user type created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Private_User user type already exists, skipping"
else
    log_error "Failed to create Private_User user type (HTTP $HTTP_CODE)"
    exit 1
fi

echo ""

# ============================================================================
# Create Government_User User Type
# ============================================================================

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

RESPONSE=$(api_call POST "/user-types" "${GOVERNMENT_USER_TYPE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "Government_User user type created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Government_User user type already exists, skipping"
else
        log_error "Failed to create Government_User user type (HTTP $HTTP_CODE)"
        exit 1
fi

echo ""

# ============================================================================
# Create Groups (Traders, CHA)
# ============================================================================

log_info "Creating Traders group..."

read -r -d '' TRADERS_GROUP_PAYLOAD <<JSON || true
{
    "name": "Traders",
    "description": "Trader members group",
    "ouId": "${PRIVATE_SECTOR_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/groups" "${TRADERS_GROUP_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Traders group created successfully"
    TRADERS_GROUP_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Traders group already exists, retrieving ID..."
    TRADERS_GROUP_ID=$(get_group_id_by_name "Traders" "$PRIVATE_SECTOR_OU_ID")
else
    log_error "Failed to create Traders group (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$TRADERS_GROUP_ID" ]]; then
    log_error "Could not determine Traders group ID"
    exit 1
fi

log_info "Traders group ID: $TRADERS_GROUP_ID"

echo ""

# ============================================================================
# Create CHA Group
# ============================================================================

log_info "Creating CHA group..."

read -r -d '' CHA_GROUP_PAYLOAD <<JSON || true
{
    "name": "CHA",
    "description": "CHA members group",
    "ouId": "${PRIVATE_SECTOR_OU_ID}"
}
JSON

RESPONSE=$(api_call POST "/groups" "${CHA_GROUP_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "CHA group created successfully"
    CHA_GROUP_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "CHA group already exists, retrieving ID..."
    CHA_GROUP_ID=$(get_group_id_by_name "CHA" "$PRIVATE_SECTOR_OU_ID")
else
    log_error "Failed to create CHA group (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$CHA_GROUP_ID" ]]; then
    log_error "Could not determine CHA group ID"
    exit 1
fi

log_info "CHA group ID: $CHA_GROUP_ID"

echo ""

# ============================================================================
# Create Roles (Trader, CHA)
# ============================================================================

log_info "Creating Trader role..."

read -r -d '' TRADER_ROLE_PAYLOAD <<JSON || true
{
    "name": "Trader",
    "description": "Role for trader operations",
    "ouId": "${PRIVATE_SECTOR_OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${NSW_RS_ID}",
            "permissions": [ ${TRADER_NSW_SCOPES} ]
        }
    ]
}
JSON

RESPONSE=$(api_call POST "/roles" "${TRADER_ROLE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Trader role created successfully"
    TRADER_ROLE_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Trader role already exists, retrieving ID..."
    TRADER_ROLE_ID=$(get_role_id_by_name "Trader" "$PRIVATE_SECTOR_OU_ID")
else
    log_error "Failed to create Trader role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$TRADER_ROLE_ID" ]]; then
    log_error "Could not determine Trader role ID"
    exit 1
fi

log_info "Trader role ID: $TRADER_ROLE_ID"

echo ""
log_info "Creating CHA role..."

read -r -d '' CHA_ROLE_PAYLOAD <<JSON || true
{
    "name": "CHA",
    "description": "Role for CHA operations",
    "ouId": "${PRIVATE_SECTOR_OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${NSW_RS_ID}",
            "permissions": [ ${TRADER_NSW_SCOPES} ]
        }
    ]
}
JSON

RESPONSE=$(api_call POST "/roles" "${CHA_ROLE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "CHA role created successfully"
    CHA_ROLE_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "CHA role already exists, retrieving ID..."
    CHA_ROLE_ID=$(get_role_id_by_name "CHA" "$PRIVATE_SECTOR_OU_ID")
else
    log_error "Failed to create CHA role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

if [[ -z "$CHA_ROLE_ID" ]]; then
    log_error "Could not determine CHA role ID"
    exit 1
fi

log_info "CHA role ID: $CHA_ROLE_ID"

echo ""

# ============================================================================
# Assign Roles to Groups
# ============================================================================

log_info "Assigning roles to groups..."
assign_role_to_group "$TRADER_ROLE_ID" "$TRADERS_GROUP_ID" "Trader" "Traders"
assign_role_to_group "$CHA_ROLE_ID" "$CHA_GROUP_ID" "CHA" "CHA"

echo ""

# ============================================================================
# Create Government Reviewer Group and Role (AGENCY_API permissions)
# ============================================================================
# OGA portal users (NPQS/FCAU/CDA/SLPA) review trader applications via the
# nsw-agency backend. A single shared "OGA Reviewers" group carries the
# "OGA Reviewer" role, which grants the AGENCY_API scopes — so a reviewer's
# token carries aud=AGENCY_API. Government users gain access simply by joining
# the group (below). Per-agency isolation is enforced by the agency backend via
# the user's ouHandle, so one shared group/role suffices.

log_info "Creating OGA Reviewers group..."
read -r -d '' OGA_REVIEWERS_GROUP_PAYLOAD <<JSON || true
{
    "name": "OGA Reviewers",
    "description": "Government agency reviewers group",
    "ouId": "${GOVERNMENT_ORG_OU_ID}"
}
JSON
RESPONSE=$(api_call POST "/groups" "${OGA_REVIEWERS_GROUP_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"
if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "OGA Reviewers group created successfully"
    OGA_REVIEWERS_GROUP_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "OGA Reviewers group already exists, retrieving ID..."
    OGA_REVIEWERS_GROUP_ID=$(get_group_id_by_name "OGA Reviewers" "$GOVERNMENT_ORG_OU_ID")
else
    log_error "Failed to create OGA Reviewers group (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi
if [[ -z "$OGA_REVIEWERS_GROUP_ID" ]]; then
    log_error "Could not determine OGA Reviewers group ID"
    exit 1
fi
log_info "OGA Reviewers group ID: $OGA_REVIEWERS_GROUP_ID"

echo ""
log_info "Creating OGA Reviewer role (AGENCY_API permissions)..."
read -r -d '' OGA_REVIEWER_ROLE_PAYLOAD <<JSON || true
{
    "name": "OGA Reviewer",
    "description": "Role for government agency reviewers (AGENCY_API)",
    "ouId": "${GOVERNMENT_ORG_OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${AGENCY_RS_ID}",
            "permissions": [ ${AGENCY_REVIEWER_SCOPES} ]
        }
    ]
}
JSON
RESPONSE=$(api_call POST "/roles" "${OGA_REVIEWER_ROLE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"
if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "OGA Reviewer role created successfully"
    OGA_REVIEWER_ROLE_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "OGA Reviewer role already exists, retrieving ID..."
    OGA_REVIEWER_ROLE_ID=$(get_role_id_by_name "OGA Reviewer" "$GOVERNMENT_ORG_OU_ID")
else
    log_error "Failed to create OGA Reviewer role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi
if [[ -z "$OGA_REVIEWER_ROLE_ID" ]]; then
    log_error "Could not determine OGA Reviewer role ID"
    exit 1
fi
log_info "OGA Reviewer role ID: $OGA_REVIEWER_ROLE_ID"

assign_role_to_group "$OGA_REVIEWER_ROLE_ID" "$OGA_REVIEWERS_GROUP_ID" "OGA Reviewer" "OGA Reviewers"

echo ""

# ============================================================================
# Create NswM2M Role (granted to M2M clients calling the Agency API)
# ============================================================================
# Reverse of AgencyM2M: NSW core's machine clients (the NSW_TO_* apps below)
# call the nsw-agency backend's inject endpoint. Assigning this role to each app
# makes their client_credentials tokens carry aud=AGENCY_API plus the
# agency:application:inject scope. Lives in the government-sector OU alongside
# the OGA Reviewer role (same AGENCY_API audience).
log_info "Creating NswM2M role (AGENCY_API permissions for machine clients)..."
read -r -d '' NSW_M2M_ROLE_PAYLOAD <<JSON || true
{
    "name": "NswM2M",
    "description": "Role for NSW machine-to-machine clients calling the Agency API",
    "ouId": "${GOVERNMENT_ORG_OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${AGENCY_RS_ID}",
            "permissions": [ ${M2M_AGENCY_SCOPES} ]
        }
    ]
}
JSON
RESPONSE=$(api_call POST "/roles" "${NSW_M2M_ROLE_PAYLOAD}")
HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"
if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "NswM2M role created successfully"
    NSW_M2M_ROLE_ID=$(extract_first_id "$BODY")
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "NswM2M role already exists, retrieving ID..."
    NSW_M2M_ROLE_ID=$(get_role_id_by_name "NswM2M" "$GOVERNMENT_ORG_OU_ID")
else
    log_error "Failed to create NswM2M role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi
if [[ -z "$NSW_M2M_ROLE_ID" ]]; then
    log_error "Could not determine NswM2M role ID"
    exit 1
fi
log_info "NswM2M role ID: $NSW_M2M_ROLE_ID"

echo ""

# ============================================================================
# Create Users in OUs
# ============================================================================

log_info "Creating sample users..."

create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "suresh" "suresh@adam-pvt-ltd.private-sector.dev" "Suresh" "Fernando" "$SURESH_PASSWORD" "+94771234567"
USER_SURESH="$CREATED_USER_ID"

create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "ramesh" "ramesh@adam-pvt-ltd.private-sector.dev" "Ramesh" "Fernando" "$RAMESH_PASSWORD" "+94771234568"
USER_RAMESH="$CREATED_USER_ID"

create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "gomesh" "gomesh@adam-pvt-ltd.private-sector.dev" "Gomesh" "Fernando" "$GOMESH_PASSWORD" "+94771234569"
USER_GOMESH="$CREATED_USER_ID"

create_user_in_ou "Private_User" "$EDWARD_PVT_LTD_OU_ID" "naresh" "naresh@edward-pvt-ltd.private-sector.dev" "Naresh" "Fernando" "$NARESH_PASSWORD" "+94771234570"
USER_NARESH="$CREATED_USER_ID"

create_user_in_ou "Government_User" "$NPQS_OU_ID" "npqs_officer" "npqs_officer@government.dev" "NPQS" "Officer" "$NPQS_OFFICER_PASSWORD" "+94771234560"
USER_NPQS_ID="$CREATED_USER_ID"

create_user_in_ou "Government_User" "$FCAU_OU_ID" "fcau_officer" "fcau_officer@government.dev" "FCAU" "Officer" "$FCAU_OFFICER_PASSWORD" "+94771234561"
USER_FCAU_ID="$CREATED_USER_ID"

create_user_in_ou "Government_User" "$CDA_OU_ID" "cda_officer" "cda_officer@government.dev" "CDA" "Officer" "$CDA_OFFICER_PASSWORD" "+94771234563"
USER_CDA_ID="$CREATED_USER_ID"

create_user_in_ou "Government_User" "$SLPA_OU_ID" "slpa_officer" "slpa_officer@government.dev" "SLPA" "Officer" "$SLPA_OFFICER_PASSWORD" "+94771234564"
USER_SLPA_ID="$CREATED_USER_ID"

echo ""

# ============================================================================
# Assign Users to Groups (Role inheritance is group-based)
# ============================================================================

log_info "Assigning users to groups..."

ensure_user_in_group "$TRADERS_GROUP_ID" "$USER_SURESH" "Traders" "suresh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_SURESH" "CHA" "suresh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_RAMESH" "CHA" "ramesh"
ensure_user_in_group "$TRADERS_GROUP_ID" "$USER_GOMESH" "Traders" "gomesh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_NARESH" "CHA" "naresh"

# Government reviewers join the shared OGA Reviewers group (grants AGENCY_API).
ensure_user_in_group "$OGA_REVIEWERS_GROUP_ID" "$USER_NPQS_ID" "OGA Reviewers" "npqs_officer"
ensure_user_in_group "$OGA_REVIEWERS_GROUP_ID" "$USER_FCAU_ID" "OGA Reviewers" "fcau_officer"
ensure_user_in_group "$OGA_REVIEWERS_GROUP_ID" "$USER_CDA_ID" "OGA Reviewers" "cda_officer"
ensure_user_in_group "$OGA_REVIEWERS_GROUP_ID" "$USER_SLPA_ID" "OGA Reviewers" "slpa_officer"

echo ""

# ============================================================================
# Fetch Theme and Flow IDs (optional)
# ============================================================================

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

echo ""

# ============================================================================
# Create SPA Applications
# ============================================================================

# Retrieve OU IDs for SPA applications
echo "Fetching OU IDs for SPA applications..."
DEFAULT_OU_ID_FOR_TRADER=$(get_ou_id_by_handle "default")
NPQS_OU_ID_FOR_APP=$(get_ou_id_by_handle "government-organization/npqs")
FCAU_OU_ID_FOR_APP=$(get_ou_id_by_handle "government-organization/fcau")
CDA_OU_ID_FOR_APP=$(get_ou_id_by_handle "government-organization/cda")
SLPA_OU_ID_FOR_APP=$(get_ou_id_by_handle "government-organization/slpa")

create_spa_application "TraderApp" "Application for trader portal built with React" "TRADER_PORTAL_APP" "5173" "Private_User" "${DEFAULT_OU_ID_FOR_TRADER}" "${TRADER_NSW_SCOPES}"
create_spa_application "NPQSPortalApp" "Application for NPQS portal built with React" "OGA_PORTAL_APP_NPQS" "5174" "Government_User" "${NPQS_OU_ID_FOR_APP}" "${AGENCY_REVIEWER_SCOPES}"
create_spa_application "FCAUPortalApp" "Application for FCAU portal built with React" "OGA_PORTAL_APP_FCAU" "5175" "Government_User" "${FCAU_OU_ID_FOR_APP}" "${AGENCY_REVIEWER_SCOPES}"
create_spa_application "CDAPortalApp" "Application for CDA portal built with React" "OGA_PORTAL_APP_CDA" "5176" "Government_User" "${CDA_OU_ID_FOR_APP}" "${AGENCY_REVIEWER_SCOPES}"
create_spa_application "SLPAPortalApp" "Application for SLPA portal built with React" "OGA_PORTAL_APP_SLPA" "5177" "Government_User" "${SLPA_OU_ID_FOR_APP}" "${AGENCY_REVIEWER_SCOPES}"

echo ""

# ============================================================================
# Resolve Default Organization Unit for M2M Applications
# ============================================================================

DEFAULT_OU_HANDLE="default"
log_info "Resolving Default organization unit for M2M applications..."

DEFAULT_OU_ID_FOR_M2M=$(get_ou_id_by_handle "${DEFAULT_OU_HANDLE}")

if [[ -z "$DEFAULT_OU_ID_FOR_M2M" ]]; then
    log_error "Could not determine Default organization unit ID for M2M applications"
    exit 1
fi

log_info "Default organization unit ID for M2M: ${DEFAULT_OU_ID_FOR_M2M}"

echo ""

# ============================================================================
# Create M2M Applications for external services calling NSW APIs
# ============================================================================

create_m2m_application "NPQS_TO_NSW_M2M" "Machine-to-machine integration for NPQS to NSW" "NPQS_TO_NSW" "${NPQS_M2M_CLIENT_SECRET}" "${DEFAULT_OU_ID_FOR_M2M}" "${M2M_NSW_SCOPES}"
NPQS_TO_NSW_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$AGENCY_M2M_ROLE_ID" "$NPQS_TO_NSW_M2M_APP_ID" "AgencyM2M" "NPQS_TO_NSW_M2M"

create_m2m_application "FCAU_TO_NSW_M2M" "Machine-to-machine integration for FCAU to NSW" "FCAU_TO_NSW" "${FCAU_M2M_CLIENT_SECRET}" "${DEFAULT_OU_ID_FOR_M2M}" "${M2M_NSW_SCOPES}"
FCAU_TO_NSW_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$AGENCY_M2M_ROLE_ID" "$FCAU_TO_NSW_M2M_APP_ID" "AgencyM2M" "FCAU_TO_NSW_M2M"

create_m2m_application "CDA_TO_NSW_M2M" "Machine-to-machine integration for CDA to NSW" "CDA_TO_NSW" "${CDA_M2M_CLIENT_SECRET}" "${DEFAULT_OU_ID_FOR_M2M}" "${M2M_NSW_SCOPES}"
CDA_TO_NSW_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$AGENCY_M2M_ROLE_ID" "$CDA_TO_NSW_M2M_APP_ID" "AgencyM2M" "CDA_TO_NSW_M2M"

create_m2m_application "SLPA_TO_NSW_M2M" "Machine-to-machine integration for SLPA to NSW" "SLPA_TO_NSW" "${SLPA_M2M_CLIENT_SECRET}" "${DEFAULT_OU_ID_FOR_M2M}" "${M2M_NSW_SCOPES}"
SLPA_TO_NSW_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$AGENCY_M2M_ROLE_ID" "$SLPA_TO_NSW_M2M_APP_ID" "AgencyM2M" "SLPA_TO_NSW_M2M"

echo ""

# ============================================================================
# Create M2M Applications for NSW core calling Agency APIs (NSW -> OGA)
# ============================================================================
# Reverse direction of the *_TO_NSW clients above: NSW core injects data into
# each agency's portal via POST /api/v1/inject. Each app is assigned the NswM2M
# role so its token carries aud=AGENCY_API + agency:application:inject. Created
# in the government-sector OU to match the NswM2M role's OU.

create_m2m_application "NSW_TO_NPQS_M2M" "Machine-to-machine integration for NSW to NPQS" "NSW_TO_NPQS" "${NSW_TO_NPQS_M2M_CLIENT_SECRET}" "${GOVERNMENT_ORG_OU_ID}" "${M2M_AGENCY_SCOPES}"
NSW_TO_NPQS_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$NSW_M2M_ROLE_ID" "$NSW_TO_NPQS_M2M_APP_ID" "NswM2M" "NSW_TO_NPQS_M2M"

create_m2m_application "NSW_TO_FCAU_M2M" "Machine-to-machine integration for NSW to FCAU" "NSW_TO_FCAU" "${NSW_TO_FCAU_M2M_CLIENT_SECRET}" "${GOVERNMENT_ORG_OU_ID}" "${M2M_AGENCY_SCOPES}"
NSW_TO_FCAU_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$NSW_M2M_ROLE_ID" "$NSW_TO_FCAU_M2M_APP_ID" "NswM2M" "NSW_TO_FCAU_M2M"

create_m2m_application "NSW_TO_CDA_M2M" "Machine-to-machine integration for NSW to CDA" "NSW_TO_CDA" "${NSW_TO_CDA_M2M_CLIENT_SECRET}" "${GOVERNMENT_ORG_OU_ID}" "${M2M_AGENCY_SCOPES}"
NSW_TO_CDA_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$NSW_M2M_ROLE_ID" "$NSW_TO_CDA_M2M_APP_ID" "NswM2M" "NSW_TO_CDA_M2M"

create_m2m_application "NSW_TO_SLPA_M2M" "Machine-to-machine integration for NSW to SLPA" "NSW_TO_SLPA" "${NSW_TO_SLPA_M2M_CLIENT_SECRET}" "${GOVERNMENT_ORG_OU_ID}" "${M2M_AGENCY_SCOPES}"
NSW_TO_SLPA_M2M_APP_ID="$CREATED_M2M_APP_ID"
assign_role_to_app "$NSW_M2M_ROLE_ID" "$NSW_TO_SLPA_M2M_APP_ID" "NswM2M" "NSW_TO_SLPA_M2M"

echo ""

# ============================================================================
# Summary
# ============================================================================

log_success "Sample resources setup completed successfully!"
log_info "Private Sector OU path: ${PRIVATE_SECTOR_OU_HANDLE}"
log_info "ADAM PVT LTD OU path: ${ADAM_PVT_LTD_OU_PATH}"
log_info "EDWARD PVT LTD OU path: ${EDWARD_PVT_LTD_OU_PATH}"
log_info "Government Organization OU path: ${GOVERNMENT_ORG_OU_HANDLE}"
log_info "Government child OUs: ${NPQS_OU_HANDLE}, ${FCAU_OU_HANDLE}, ${CDA_OU_HANDLE}, ${SLPA_OU_HANDLE}"
log_info "Private user type: Private_User"
log_info "Government user type: Government_User"
log_info "Traders group -> Trader role (NSW_API scopes)"
log_info "CHA group -> CHA role (NSW_API scopes)"
log_info "OGA Reviewers group -> OGA Reviewer role (AGENCY_API scopes)"
log_info "suresh in groups: Traders, CHA"
log_info "ramesh in groups: CHA"
log_info "gomesh in groups: Traders"
log_info "naresh (EDWARD PVT LTD) in groups: CHA"
log_info "Government users: npqs_user, fcau_user, cda_user, slpa_user - all in OGA Reviewers group"
log_info "App client IDs: TRADER_PORTAL_APP, OGA_PORTAL_APP_NPQS, OGA_PORTAL_APP_FCAU, OGA_PORTAL_APP_CDA, OGA_PORTAL_APP_SLPA"
log_info "M2M client IDs (OGA -> NSW): NPQS_TO_NSW, FCAU_TO_NSW, CDA_TO_NSW, SLPA_TO_NSW"
log_info "M2M client IDs (NSW -> OGA): NSW_TO_NPQS, NSW_TO_FCAU, NSW_TO_CDA, NSW_TO_SLPA"
log_info "M2M roles: AgencyM2M (clients -> NSW_API), NswM2M (clients -> AGENCY_API)"
log_info "M2M auth method: client_secret_basic"
echo ""
log_info "Resource servers (token audiences):"
log_info "  NSW_API    -> TraderApp users (Trader/CHA roles) + *_TO_NSW M2M clients (AgencyM2M role on app)"
log_info "  AGENCY_API -> OGA portal users (OGA Reviewers group / OGA Reviewer role)"
log_info "NSW_API scopes: nsw:{consignment,task,storage}:{read,write,delete}, nsw:{hscode,company,cha}:read"
log_info "AGENCY_API scopes: agency:application:{read,review,feedback}, agency:consignment:read, agency:storage:{read,write}"
echo ""
