#!/usr/bin/env bash
# Applies migrations/ to PostgreSQL with the migrator version the tnsw-migrate
# image pins (`migrate` on PATH), the way deployments run it:
#   1. a fresh install: every migration into an empty database;
#   2. an upgrade: BASE's migrations applied by BASE's migrator, then each
#      migration this change adds, one at a time: applied, rolled back and
#      compared with the schema from before it, then applied again. So every
#      new migration needs a @DOWN that undoes its schema changes. BASE_MIGRATE
#      names BASE's migrator when the Dockerfile pin differs; it defaults to
#      `migrate`. Older migrations are not rolled back: several have no @DOWN
#      on purpose.
# Connection settings come from DB_HOST, DB_PORT, DB_USER, DB_PASSWORD and
# DB_SSLMODE (default require, as the migrator's). The migrator reads them, and
# the migrations directory, from a config.yaml the script writes: the db section
# and migrationDir the server's config file uses. Schemas are compared with
# PG_DUMP (default `pg_dump`), which may be a wrapper such as a `docker exec`
# into the server's container, so the client matches the server version. The
# script creates its own databases and drops them when it ends.
#
# Usage: test-migrations.sh BASE
set -euo pipefail

base="${1:?usage: test-migrations.sh BASE}"
: "${DB_HOST:?}" "${DB_PORT:?}" "${DB_USER:?}" "${DB_PASSWORD:?}"
export DB_PASSWORD DB_SSLMODE="${DB_SSLMODE:-require}"
base_migrate="${BASE_MIGRATE:-migrate}"
read -r -a pg_dump_cmd <<< "${PG_DUMP:-pg_dump}"

cd "$(git rev-parse --show-toplevel)"
if ! git rev-parse --verify --quiet "${base}^{commit}" >/dev/null; then
  echo "test-migrations.sh: '${base}' is not a commit in this repository; fetch it first." >&2
  exit 1
fi

# File names in a directory, in sorted order.
names() { local f; for f in "$1"/*; do [ -e "$f" ] && printf '%s\n' "${f##*/}"; done; }
admin() {
  PGPASSWORD="$DB_PASSWORD" PGSSLMODE="$DB_SSLMODE" \
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 -qc "$1"
}
new_database() { admin "DROP DATABASE IF EXISTS $1"; admin "CREATE DATABASE $1"; }
# Runs a migrator command ($3, by migrator $4, default `migrate`) on database $1
# with the migrations in directory $2. The password stays in the environment,
# named by a placeholder, so it is never written to the file.
migrate_in() {
  local dir
  dir=$(cd "$2" && pwd)
  cat > "$tmp/config.yaml" <<EOF
db:
  driver: postgres
  postgres:
    host: ${DB_HOST}
    port: ${DB_PORT}
    user: ${DB_USER}
    password: "{{env:DB_PASSWORD}}"
    name: $1
    sslMode: ${DB_SSLMODE}
migrationDir: ${dir}
EOF
  CONFIG_PATH="$tmp/config.yaml" "${4:-migrate}" "$3"
}
pending() { migrate_in "$1" "$2" status | awk '$3 == "pending"' | wc -l | tr -d ' '; }
expect_pending() {
  local count
  count=$(pending "$1" migrations)
  if [ "$count" -ne "$2" ]; then
    echo "Expected $2 pending migration(s) in $1, found ${count}." >&2
    migrate_in "$1" migrations status >&2
    exit 1
  fi
}
# The database's schema, ownership and privileges included, without the
# migrator's own table. pg_dump's \restrict lines carry a random key per dump,
# so they are dropped.
schema() {
  PGPASSWORD="$DB_PASSWORD" PGSSLMODE="$DB_SSLMODE" \
    "${pg_dump_cmd[@]}" -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$1" \
      --schema-only --exclude-table=__migrations \
    | grep -vE '^\\(un)?restrict '
}

tmp=$(mktemp -d)
cleanup() {
  rm -rf "$tmp"
  admin "DROP DATABASE IF EXISTS migrations_fresh" 2>/dev/null || true
  admin "DROP DATABASE IF EXISTS migrations_upgrade" 2>/dev/null || true
}
trap cleanup EXIT
git archive "$base" migrations | tar -x -C "$tmp"
steps="$tmp/migrations"

echo "== Fresh install"
new_database migrations_fresh
migrate_in migrations_fresh migrations up
expect_pending migrations_fresh 0

echo "== Upgrade from ${base}"
new_database migrations_upgrade
migrate_in migrations_upgrade "$steps" up "$base_migrate"

added=$(comm -13 <(names "$steps") <(names migrations))
if [ -z "$added" ]; then
  migrate_in migrations_upgrade migrations up
  expect_pending migrations_upgrade 0
  echo "== No new migrations to roll back"
  exit 0
fi

# $steps holds BASE's migrations plus the new ones up to the current one, so
# `up` applies just that one and `down` rolls just it back.
while IFS= read -r file; do
  echo "== ${file}: apply, roll back, compare, apply again"
  cp "migrations/${file}" "$steps/"
  schema migrations_upgrade > "$tmp/before.sql"
  migrate_in migrations_upgrade "$steps" up
  migrate_in migrations_upgrade "$steps" down
  schema migrations_upgrade > "$tmp/after.sql"
  if ! diff -u --label "schema before ${file}" --label "schema after rolling it back" \
         "$tmp/before.sql" "$tmp/after.sql" >&2; then
    echo "Rolling back ${file} does not return the schema to its state before it; its @DOWN must undo every change its @UP makes." >&2
    exit 1
  fi
  migrate_in migrations_upgrade "$steps" up
done <<< "$added"
expect_pending migrations_upgrade 0
