#!/bin/sh
set -eu

escape_js() {
  printf '%s' "$1" | awk '
    BEGIN { ORS=""; first=1 }
    {
      if (!first) {
        printf "\\n"
      }
      first=0
      gsub(/\\/,"\\\\")
      gsub(/"/,"\\\"")
      gsub(/\t/,"\\t")
      gsub(sprintf("%c",13),"\\r")
      gsub(sprintf("%c",12),"\\f")
      gsub(sprintf("%c",8),"\\b")
      printf "%s", $0
    }
  '
}

RUNTIME_FILE="/usr/share/nginx/html/runtime-env.js"

cat <<EOF > "$RUNTIME_FILE"
window.__APP_CONFIG__ = {
  "VITE_API_BASE_URL": "$(escape_js "${VITE_API_BASE_URL:-http://localhost:8080}")",
  "VITE_IDP_BASE_URL": "$(escape_js "${VITE_IDP_BASE_URL:-https://localhost:8090}")",
  "VITE_IDP_CLIENT_ID": "$(escape_js "${VITE_IDP_CLIENT_ID:-TRADER_PORTAL_APP}")",
  "VITE_APP_URL": "$(escape_js "${VITE_APP_URL:-http://localhost:5173}")",
  "VITE_IDP_SCOPES": "$(escape_js "${VITE_IDP_SCOPES:-openid,profile,email,group,role}")",
  "VITE_IDP_PLATFORM": "$(escape_js "${VITE_IDP_PLATFORM:-AsgardeoV2}")",
  "VITE_IDP_TRADER_GROUP_NAME": "$(escape_js "${VITE_IDP_TRADER_GROUP_NAME:-Traders}")",
  "VITE_IDP_CHA_GROUP_NAME": "$(escape_js "${VITE_IDP_CHA_GROUP_NAME:-CHA}")",
  "VITE_SHOW_AUTOFILL_BUTTON": "$(escape_js "${VITE_SHOW_AUTOFILL_BUTTON:-true}")"
};
EOF
