#!/usr/bin/env bash
# Checks a version from version.txt: a "v" followed by SemVer (the tag name,
# e.g. v1.2.3), and, when the previous version is given, higher than it.
#
# Usage: check-version.sh vX.Y.Z [vPREVIOUS]
set -euo pipefail

tag="${1:?usage: check-version.sh vX.Y.Z [vPREVIOUS]}"
previous="${2:-}"

# "v" plus the full SemVer 2 grammar, as in chart-release.yml.
SEMVER_RE='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*)?(\+[0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*)?$'
if ! [[ "$tag" =~ $SEMVER_RE ]]; then
  echo "version.txt holds '${tag}', which is not a version like v1.2.3." >&2
  exit 1
fi
[ -n "$previous" ] || exit 0

version="${tag#v}"
previous="${previous#v}"

# Compare X.Y.Z first; on a tie, a release outranks its pre-releases, and two
# pre-releases compare by their suffix. Build metadata (+...) never counts.
core() { local v="${1%%+*}"; printf '%s' "${v%%-*}"; }
pre() { local v="${1%%+*}"; [[ "$v" == *-* ]] && printf '%s' "${v#*-}" || true; }
newer() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" ]; }

if [ "$(core "$version")" = "$(core "$previous")" ]; then
  new_pre=$(pre "$version") old_pre=$(pre "$previous")
  if { [ -z "$new_pre" ] && [ -n "$old_pre" ]; } || { [ -n "$new_pre" ] && [ -n "$old_pre" ] && newer "$new_pre" "$old_pre"; }; then
    exit 0
  fi
elif newer "$(core "$version")" "$(core "$previous")"; then
  exit 0
fi
echo "version.txt goes from v${previous} to v${version}; a release must raise it." >&2
exit 1
