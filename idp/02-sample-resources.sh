#!/bin/bash

set -e

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]:-$0}")"
source "${SCRIPT_DIR}/common.sh"

ENV_FILE="${SCRIPT_DIR}/.env"
if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi
SAMPLE_USER_PASSWORD="${SAMPLE_USER_PASSWORD:-1234}"
M2M_CLIENT_SECRET="${M2M_CLIENT_SECRET:-1234}"
SURESH_PASSWORD="${SAMPLE_SURESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
RAMESH_PASSWORD="${SAMPLE_RAMESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
GOMESH_PASSWORD="${SAMPLE_GOMESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"
NARESH_PASSWORD="${SAMPLE_NARESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}"

# Agency Registry — FORMAT: handle|name|description|port
# To add a new agency, add ONE line here. Everything else is derived automatically.
AGENCIES=(
    "npqs|NPQS|National Plant Quarantine Service|5174"
    "fcau|FCAU|Food Control Administration Unit|5175"
    "cda|CDA|Coconut Development Authority|5176"
    "slpa|SLPA|Sri Lanka Ports Authority|5177"
    "customs|Customs|Sri Lanka Customs|5178"
)
get_agency_officer_password() {
    local upper="$1"
    local var="SAMPLE_${upper}_OFFICER_PASSWORD"
    local val="${!var}"
    echo "${val:-${SAMPLE_USER_PASSWORD}}"
}
get_agency_to_nsw_secret() {
    local upper="$1"
    local var="M2M_${upper}_TO_NSW_SECRET"
    local val="${!var}"
    echo "${val:-${M2M_CLIENT_SECRET}}"
}
get_nsw_to_agency_secret() {
    local upper="$1"
    local var="M2M_NSW_TO_${upper}_SECRET"
    local val="${!var}"
    echo "${val:-${M2M_CLIENT_SECRET}}"
}
NSW_API_IDENTIFIER="NSW_API"
AGENCY_API_IDENTIFIER="AGENCY_API"

TRADER_NSW_SCOPES='"nsw:consignment:read", "nsw:consignment:write", "nsw:task:read", "nsw:task:write", "nsw:hscode:read", "nsw:company:read", "nsw:cha:read", "nsw:storage:read", "nsw:storage:write"'
M2M_NSW_SCOPES='"nsw:task:write", "nsw:consignment:read", "nsw:storage:read", "nsw:storage:write"'
AGENCY_REVIEWER_SCOPES='"agency:application:read", "agency:application:review", "agency:application:feedback", "agency:consignment:read", "agency:storage:read", "agency:storage:write"'
M2M_AGENCY_SCOPES='"agency:application:inject"'

log_info "Creating sample Thunder resources..."
echo ""

# --- Helpers ---

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

# Idempotent OU creation. Sets CREATED_OU_ID.
create_ou() {
    local HANDLE="$1" NAME="$2" DESCRIPTION="$3" PARENT_ID="${4:-}" TREE_PATH="${5:-$1}"
    local RESPONSE HTTP_CODE BODY PARENT_FIELD=""

    [[ -n "$PARENT_ID" ]] && PARENT_FIELD=",
    \"parent\": \"${PARENT_ID}\""

    log_info "Creating ${NAME} organization unit..."

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
        CREATED_OU_ID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} organization unit already exists, retrieving ID..."
        RESPONSE=$(api_call GET "/organization-units/tree/${TREE_PATH}")
        HTTP_CODE="${RESPONSE: -3}"
        BODY="${RESPONSE%???}"

        if [[ "$HTTP_CODE" == "200" ]]; then
            CREATED_OU_ID=$(extract_first_id "$BODY")
        else
            log_error "Failed to fetch ${NAME} OU (HTTP $HTTP_CODE)"
            echo "Response: $BODY"
            exit 1
        fi
    else
        log_error "Failed to create ${NAME} organization unit (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi

    if [[ -z "$CREATED_OU_ID" ]]; then
        log_error "Could not determine ${NAME} organization unit ID"
        exit 1
    fi

    log_info "${NAME} OU ID: $CREATED_OU_ID"
    echo ""
}

# Idempotent group creation. Sets CREATED_GROUP_ID.
create_group() {
    local NAME="$1" DESCRIPTION="$2" OU_ID="$3"
    local RESPONSE HTTP_CODE BODY

    log_info "Creating ${NAME} group..."

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
        CREATED_GROUP_ID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} group already exists, retrieving ID..."
        CREATED_GROUP_ID=$(get_group_id_by_name "$NAME" "$OU_ID")
    else
        log_error "Failed to create ${NAME} group (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi

    if [[ -z "$CREATED_GROUP_ID" ]]; then
        log_error "Could not determine ${NAME} group ID"
        exit 1
    fi

    log_info "${NAME} group ID: $CREATED_GROUP_ID"
    echo ""
}

# Idempotent role creation with resource-server permissions. Sets CREATED_ROLE_ID.
create_role() {
    local NAME="$1" DESCRIPTION="$2" OU_ID="$3" RS_ID="$4" SCOPES="$5"
    local RESPONSE HTTP_CODE BODY

    log_info "Creating ${NAME} role..."

    read -r -d '' ROLE_PAYLOAD <<JSON || true
{
    "name": "${NAME}",
    "description": "${DESCRIPTION}",
    "ouId": "${OU_ID}",
    "permissions": [
        {
            "resourceServerId": "${RS_ID}",
            "permissions": [ ${SCOPES} ]
        }
    ]
}
JSON

    RESPONSE=$(api_call POST "/roles" "${ROLE_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "${NAME} role created successfully"
        CREATED_ROLE_ID=$(extract_first_id "$BODY")
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${NAME} role already exists, retrieving ID..."
        CREATED_ROLE_ID=$(get_role_id_by_name "$NAME" "$OU_ID")
    else
        log_error "Failed to create ${NAME} role (HTTP $HTTP_CODE)"
        echo "Response: $BODY"
        exit 1
    fi

    if [[ -z "$CREATED_ROLE_ID" ]]; then
        log_error "Could not determine ${NAME} role ID"
        exit 1
    fi

    log_info "${NAME} role ID: $CREATED_ROLE_ID"
    echo ""
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
    local REDIRECT_DOMAIN="${8:-}"
    local RESPONSE HTTP_CODE BODY
    local APP_ID APP_CLIENT_ID
    local REDIRECT_URI=""
    if [[ -n "$REDIRECT_DOMAIN" ]]; then
        if [[ "$REDIRECT_DOMAIN" =~ ^https?:// ]]; then
            REDIRECT_URI="$REDIRECT_DOMAIN"
        else
            REDIRECT_URI="https://${REDIRECT_DOMAIN}"
        fi
    fi
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
                    ${REDIRECT_URIS}
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

# --- OAuth2 Resource Servers (NSW_API, AGENCY_API) ---

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

create_role "AgencyM2M" "Role for agency machine-to-machine clients calling the NSW API" \
    "${DEFAULT_OU_ID}" "${NSW_RS_ID}" "${M2M_NSW_SCOPES}"
AGENCY_M2M_ROLE_ID="$CREATED_ROLE_ID"

# --- Organization Units ---

GOVERNMENT_ORG_OU_HANDLE="government-organization"
PRIVATE_SECTOR_OU_HANDLE="private-sector"
create_ou "${PRIVATE_SECTOR_OU_HANDLE}" "Private Sector" "Organization unit for private sector entities"
PRIVATE_SECTOR_OU_ID="$CREATED_OU_ID"

create_ou "adam-pvt-ltd" "ADAM PVT LTD" "Child organization unit for ADAM PVT LTD" \
    "$PRIVATE_SECTOR_OU_ID" "${PRIVATE_SECTOR_OU_HANDLE}/adam-pvt-ltd"
ADAM_PVT_LTD_OU_ID="$CREATED_OU_ID"

create_ou "edward-pvt-ltd" "EDWARD PVT LTD" "Child organization unit for EDWARD PVT LTD" \
    "$PRIVATE_SECTOR_OU_ID" "${PRIVATE_SECTOR_OU_HANDLE}/edward-pvt-ltd"
EDWARD_PVT_LTD_OU_ID="$CREATED_OU_ID"
create_ou "${GOVERNMENT_ORG_OU_HANDLE}" "Government Organization" "Root organization unit for government entities"
GOVERNMENT_ORG_OU_ID="$CREATED_OU_ID"
declare -A AGENCY_OU_IDS
for entry in "${AGENCIES[@]}"; do
    IFS='|' read -r handle name description port <<< "$entry"
    create_ou "$handle" "$name" "$description" \
        "$GOVERNMENT_ORG_OU_ID" "${GOVERNMENT_ORG_OU_HANDLE}/${handle}"
    AGENCY_OU_IDS["$handle"]="$CREATED_OU_ID"
done

# --- User Types ---

create_user_type() {
    local TYPE_NAME="$1" OU_ID="$2"
    local RESPONSE HTTP_CODE

    log_info "Creating ${TYPE_NAME} user type..."

    read -r -d '' USER_TYPE_PAYLOAD <<JSON || true
{
    "name": "${TYPE_NAME}",
    "ouId": "${OU_ID}",
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

    RESPONSE=$(api_call POST "/user-types" "${USER_TYPE_PAYLOAD}")
    HTTP_CODE="${RESPONSE: -3}"

    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "${TYPE_NAME} user type created successfully"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "${TYPE_NAME} user type already exists, skipping"
    else
        log_error "Failed to create ${TYPE_NAME} user type (HTTP $HTTP_CODE)"
        exit 1
    fi
    echo ""
}

create_user_type "Private_User" "${PRIVATE_SECTOR_OU_ID}"
create_user_type "Government_User" "${GOVERNMENT_ORG_OU_ID}"

# --- Groups and Roles ---

create_group "Traders" "Trader members group" "${PRIVATE_SECTOR_OU_ID}"
TRADERS_GROUP_ID="$CREATED_GROUP_ID"

create_group "CHA" "CHA members group" "${PRIVATE_SECTOR_OU_ID}"
CHA_GROUP_ID="$CREATED_GROUP_ID"

create_role "Trader" "Role for trader operations" \
    "${PRIVATE_SECTOR_OU_ID}" "${NSW_RS_ID}" "${TRADER_NSW_SCOPES}"
TRADER_ROLE_ID="$CREATED_ROLE_ID"

create_role "CHA" "Role for CHA operations" \
    "${PRIVATE_SECTOR_OU_ID}" "${NSW_RS_ID}" "${TRADER_NSW_SCOPES}"
CHA_ROLE_ID="$CREATED_ROLE_ID"

assign_role_to_group "$TRADER_ROLE_ID" "$TRADERS_GROUP_ID" "Trader" "Traders"
assign_role_to_group "$CHA_ROLE_ID" "$CHA_GROUP_ID" "CHA" "CHA"
echo ""

create_group "OGA Reviewers" "Government agency reviewers group" "${GOVERNMENT_ORG_OU_ID}"
OGA_REVIEWERS_GROUP_ID="$CREATED_GROUP_ID"

create_role "OGA Reviewer" "Role for government agency reviewers (AGENCY_API)" \
    "${GOVERNMENT_ORG_OU_ID}" "${AGENCY_RS_ID}" "${AGENCY_REVIEWER_SCOPES}"
OGA_REVIEWER_ROLE_ID="$CREATED_ROLE_ID"

assign_role_to_group "$OGA_REVIEWER_ROLE_ID" "$OGA_REVIEWERS_GROUP_ID" "OGA Reviewer" "OGA Reviewers"
echo ""
create_role "NswM2M" "Role for NSW machine-to-machine clients calling the Agency API" \
    "${GOVERNMENT_ORG_OU_ID}" "${AGENCY_RS_ID}" "${M2M_AGENCY_SCOPES}"
NSW_M2M_ROLE_ID="$CREATED_ROLE_ID"

# --- Users ---

log_info "Creating sample users..."
create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "suresh" "suresh@adam-pvt-ltd.private-sector.dev" "Suresh" "Fernando" "$SURESH_PASSWORD" "+94771234567"
USER_SURESH="$CREATED_USER_ID"

create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "ramesh" "ramesh@adam-pvt-ltd.private-sector.dev" "Ramesh" "Fernando" "$RAMESH_PASSWORD" "+94771234568"
USER_RAMESH="$CREATED_USER_ID"

create_user_in_ou "Private_User" "$ADAM_PVT_LTD_OU_ID" "gomesh" "gomesh@adam-pvt-ltd.private-sector.dev" "Gomesh" "Fernando" "$GOMESH_PASSWORD" "+94771234569"
USER_GOMESH="$CREATED_USER_ID"

create_user_in_ou "Private_User" "$EDWARD_PVT_LTD_OU_ID" "naresh" "naresh@edward-pvt-ltd.private-sector.dev" "Naresh" "Fernando" "$NARESH_PASSWORD" "+94771234570"
USER_NARESH="$CREATED_USER_ID"
declare -A AGENCY_USER_IDS
PHONE_COUNTER=60
for entry in "${AGENCIES[@]}"; do
    IFS='|' read -r handle name description port <<< "$entry"
    local_upper=$(echo "$handle" | tr '[:lower:]' '[:upper:]')
    officer_pw=$(get_agency_officer_password "$local_upper")

    create_user_in_ou "Government_User" "${AGENCY_OU_IDS[$handle]}" \
        "${handle}_officer" "${handle}_officer@government.dev" \
        "$name" "Officer" "$officer_pw" "+947712345${PHONE_COUNTER}"
    AGENCY_USER_IDS["$handle"]="$CREATED_USER_ID"
    PHONE_COUNTER=$((PHONE_COUNTER + 1))
done

echo ""

# --- Group Membership ---

log_info "Assigning users to groups..."

ensure_user_in_group "$TRADERS_GROUP_ID" "$USER_SURESH" "Traders" "suresh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_SURESH" "CHA" "suresh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_RAMESH" "CHA" "ramesh"
ensure_user_in_group "$TRADERS_GROUP_ID" "$USER_GOMESH" "Traders" "gomesh"
ensure_user_in_group "$CHA_GROUP_ID" "$USER_NARESH" "CHA" "naresh"
for entry in "${AGENCIES[@]}"; do
    IFS='|' read -r handle name description port <<< "$entry"
    ensure_user_in_group "$OGA_REVIEWERS_GROUP_ID" "${AGENCY_USER_IDS[$handle]}" "OGA Reviewers" "${handle}_officer"
done

echo ""

# --- Theme and Flow IDs ---

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

# --- SPA Applications ---
DEFAULT_OU_ID_FOR_TRADER="$DEFAULT_OU_ID"
create_spa_application "TraderApp" "Application for trader portal built with React" \
    "TRADER_PORTAL_APP" "5173" "Private_User" "${DEFAULT_OU_ID_FOR_TRADER}" \
    "${TRADER_NSW_SCOPES}" "${TRADER_APP_REDIRECT_DOMAIN:-}"
for entry in "${AGENCIES[@]}"; do
    IFS='|' read -r handle name description port <<< "$entry"
    local_upper=$(echo "$handle" | tr '[:lower:]' '[:upper:]')
    ou_id="${AGENCY_OU_IDS[$handle]}"
    redirect_var="${local_upper}_APP_REDIRECT_DOMAIN"

    create_spa_application "${name}PortalApp" "Application for ${name} portal built with React" \
        "OGA_PORTAL_APP_${local_upper}" "$port" "Government_User" "${ou_id}" \
        "${AGENCY_REVIEWER_SCOPES}" "${!redirect_var:-}"
done

echo ""

# --- M2M Applications ---

DEFAULT_OU_ID_FOR_M2M="$DEFAULT_OU_ID"
for entry in "${AGENCIES[@]}"; do
    IFS='|' read -r handle name description port <<< "$entry"
    local_upper=$(echo "$handle" | tr '[:lower:]' '[:upper:]')
    secret=$(get_agency_to_nsw_secret "$local_upper")

    create_m2m_application "${local_upper}_TO_NSW_M2M" \
        "Machine-to-machine integration for ${name} to NSW" \
        "${local_upper}_TO_NSW" "$secret" "${DEFAULT_OU_ID_FOR_M2M}" "${M2M_NSW_SCOPES}"
    assign_role_to_app "$AGENCY_M2M_ROLE_ID" "$CREATED_M2M_APP_ID" "AgencyM2M" "${local_upper}_TO_NSW_M2M"
done

echo ""
for entry in "${AGENCIES[@]}"; do
    IFS='|' read -r handle name description port <<< "$entry"
    local_upper=$(echo "$handle" | tr '[:lower:]' '[:upper:]')
    secret=$(get_nsw_to_agency_secret "$local_upper")

    create_m2m_application "NSW_TO_${local_upper}_M2M" \
        "Machine-to-machine integration for NSW to ${name}" \
        "NSW_TO_${local_upper}" "$secret" "${GOVERNMENT_ORG_OU_ID}" "${M2M_AGENCY_SCOPES}"
    assign_role_to_app "$NSW_M2M_ROLE_ID" "$CREATED_M2M_APP_ID" "NswM2M" "NSW_TO_${local_upper}_M2M"
done

echo ""

# --- Summary ---
log_success "Sample resources setup completed successfully!"
log_info "Agencies: $(printf '%s\n' "${AGENCIES[@]}" | cut -d'|' -f1 | tr '\n' ',' | sed 's/,$//')"
log_info "SPA clients: TRADER_PORTAL_APP, $(for e in "${AGENCIES[@]}"; do IFS='|' read -r h _ _ _ <<< "$e"; printf "OGA_PORTAL_APP_%s, " "$(echo "$h" | tr '[:lower:]' '[:upper:]')"; done | sed 's/, $//')"
log_info "M2M clients: $(for e in "${AGENCIES[@]}"; do IFS='|' read -r h _ _ _ <<< "$e"; u=$(echo "$h" | tr '[:lower:]' '[:upper:]'); printf "%s_TO_NSW, NSW_TO_%s, " "$u" "$u"; done | sed 's/, $//')"
echo ""
