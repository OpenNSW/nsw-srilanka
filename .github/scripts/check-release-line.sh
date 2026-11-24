#!/usr/bin/env bash
# Checks that a release stays in its branch's line. A maintenance branch
# release/X.Y.x releases only the patches vX.Y.Z, so a patch can't carry a
# minor or major release, or a pre-release. A release from any other branch
# passes.
#
# Usage: check-release-line.sh BRANCH vX.Y.Z
set -euo pipefail

branch="${1:?usage: check-release-line.sh BRANCH vX.Y.Z}"
tag="${2:?usage: check-release-line.sh BRANCH vX.Y.Z}"
branch="${branch#refs/heads/}"

case "$branch" in
  release/*) ;;
  *) exit 0 ;;
esac

if ! [[ "$branch" =~ ^release/(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.x$ ]]; then
  echo "${branch} isn't named like a maintenance branch: name it release/X.Y.x, after the line it patches." >&2
  exit 1
fi
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
if ! [[ "$tag" =~ ^v${major}\.${minor}\.(0|[1-9][0-9]*)$ ]]; then
  echo "${branch} releases only ${major}.${minor} patches (v${major}.${minor}.Z), but version.txt says ${tag}." >&2
  exit 1
fi
