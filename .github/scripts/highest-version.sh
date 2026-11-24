#!/usr/bin/env bash
# Prints the highest of the versions read from stdin, one tag name per line
# (v1.2.3), by SemVer precedence: the comparison check-version.sh makes. Lines
# that aren't versions are skipped. Prints nothing when there are none.
#
# Usage: git tag --list 'v*' | highest-version.sh
set -euo pipefail

here=$(dirname "$0")
highest=""
while IFS= read -r tag || [ -n "$tag" ]; do
  tag=$(printf '%s' "$tag" | tr -d '[:space:]')
  [ -n "$tag" ] || continue
  bash "$here/check-version.sh" "$tag" 2>/dev/null || continue
  # check-version.sh succeeds when its first version is higher than its second.
  if [ -z "$highest" ] || bash "$here/check-version.sh" "$tag" "$highest" 2>/dev/null; then
    highest=$tag
  fi
done
[ -z "$highest" ] || printf '%s\n' "$highest"
