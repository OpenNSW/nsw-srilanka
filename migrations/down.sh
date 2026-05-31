#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ---------------------------------------------------------------------------
# Load environment variables (skip if already injected, e.g. via Docker)
# ---------------------------------------------------------------------------
ENV_FILE_PATH="${ENV_FILE:-../.env}"

_REQUIRED_VARS_SET=true
for VAR in DB_HOST DB_PORT DB_USERNAME DB_PASSWORD DB_NAME; do
  if [ -z "${!VAR:-}" ]; then
    _REQUIRED_VARS_SET=false
    break
  fi
done

if [[ "$_REQUIRED_VARS_SET" == "false" ]]; then
  if [ -f "$ENV_FILE_PATH" ]; then
    set -o allexport
    source "$ENV_FILE_PATH"
    set +o allexport
  else
    echo "Error: env file not found: $ENV_FILE_PATH"
    exit 1
  fi
fi

# ---------------------------------------------------------------------------
# Validate required variables
# ---------------------------------------------------------------------------
for VAR in DB_HOST DB_PORT DB_USERNAME DB_PASSWORD DB_NAME; do
  if [ -z "${!VAR:-}" ]; then
    echo "Error: Required environment variable $VAR is not set."
    exit 1
  fi
done

MIGRATION_DB_HOST="${MIGRATION_DB_HOST:-$DB_HOST}"
MIGRATION_DB_HOST="${MIGRATION_DB_HOST//host.docker.internal/localhost}"

# ---------------------------------------------------------------------------
# Discover and filter migrations in reverse order
# ---------------------------------------------------------------------------
mapfile -t ALL_DOWNS < <(find "$SCRIPT_DIR" -maxdepth 1 -name "*.down.sql" | sort -r)

if [[ ${#ALL_DOWNS[@]} -eq 0 ]]; then
  echo "No rollback files found in $SCRIPT_DIR"
  exit 0
fi

echo "Running ${#ALL_DOWNS[@]} rollback(s)..."

# Move to the script's directory so file references work
cd "$SCRIPT_DIR"

# ---------------------------------------------------------------------------
# Execute rollbacks
# ---------------------------------------------------------------------------
for FILE in "${ALL_DOWNS[@]}"; do
  echo "Executing: $(basename "$FILE")"
  PGPASSWORD=$DB_PASSWORD psql -v ON_ERROR_STOP=1 \
    -h "$MIGRATION_DB_HOST" -p "$DB_PORT" -U "$DB_USERNAME" -d "$DB_NAME" -f "$FILE"
done

echo "Rollbacks completed successfully."
