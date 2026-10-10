#!/usr/bin/env bash
# Checks migrations/ against BASE, the commit the change is applied on:
#   - every file is named NNNNNN_name.sql, and the versions run from 000001
#     with no gaps or duplicates;
#   - every migration that exists at BASE is unchanged. The migrator never
#     re-runs an applied migration, so an edit would never reach a database
#     that already has it.
# Together these put new migrations after BASE's newest, so they apply in the
# same order on new and existing databases. The SQL itself is checked by
# applying it (test-migrations.sh).
#
# Usage: check-migrations.sh BASE
set -euo pipefail

base="${1:?usage: check-migrations.sh BASE}"
cd "$(git rev-parse --show-toplevel)"
if ! git rev-parse --verify --quiet "${base}^{commit}" >/dev/null; then
  echo "check-migrations.sh: '${base}' is not a commit in this repository; fetch it first." >&2
  exit 1
fi

errors=0
fail() { echo "$1" >&2; errors=$((errors + 1)); }
# File names in a directory, in sorted order.
names() { local f; for f in "$1"/*; do [ -e "$f" ] && printf '%s\n' "${f##*/}"; done; }

expected=1
previous=""
while IFS= read -r file; do
  if ! [[ "$file" =~ ^[0-9]{6}_[a-z0-9_]+\.sql$ ]]; then
    fail "migrations/${file}: name it NNNNNN_name.sql, in lower case (make migration name=<description> creates it)."
    continue
  fi
  version=$((10#${file%%_*}))
  if [ "$version" -eq $((expected - 1)) ] && [ -n "$previous" ]; then
    fail "migrations/${file} and migrations/${previous} share version $(printf '%06d' "$version"); give the new one the next free number."
  elif [ "$version" -ne "$expected" ]; then
    fail "migrations/${file}: expected version $(printf '%06d' "$expected") here; versions must run 000001, 000002, … with no gaps."
  fi
  expected=$((version + 1))
  previous=$file
done < <(names migrations)

while IFS= read -r file; do
  if [ ! -f "migrations/${file}" ]; then
    fail "migrations/${file} is deleted or renamed. A migration already on the base branch must stay as it is; add a new migration instead."
  elif [ "$(git rev-parse "${base}:migrations/${file}")" != "$(git hash-object "migrations/${file}")" ]; then
    fail "migrations/${file} is edited. The migrator never re-runs an applied migration, so the edit would never reach existing databases; add a new migration instead."
  fi
done < <(git ls-tree --name-only "$base" migrations/ | sed 's|^migrations/||')

if [ "$errors" -gt 0 ]; then
  echo "${errors} problem(s) in migrations/." >&2
  exit 1
fi
echo "migrations/: $((expected - 1)) migration(s), named and numbered in order; none on ${base} changed."
