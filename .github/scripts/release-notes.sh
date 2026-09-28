#!/usr/bin/env bash
# Prints the release notes for VERSION — its CHANGELOG.md section at HEAD — and
# fails when they can't be trusted to describe what HEAD releases:
#   - CHANGELOG.md has no section for VERSION, or it is empty;
#   - HEAD is not the commit that last wrote that section, so anything merged
#     after it would ship without being in the notes.
# Errors go to stderr as one line. Needs the full history (actions/checkout
# with fetch-depth: 0).
#
# Usage: release-notes.sh VERSION
set -euo pipefail

version="${1:?usage: release-notes.sh VERSION}"
here=$(dirname "$0")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ "$(git rev-parse --is-shallow-repository)" != false ]; then
  echo "release-notes.sh needs the full git history (actions/checkout with fetch-depth: 0)." >&2
  exit 1
fi

# The section as it stands at a commit, heading included so that fixing the
# date counts as writing it. Empty when the commit has no such section.
section_at() {
  git show "$1:CHANGELOG.md" > "$tmp/CHANGELOG.md" 2>/dev/null || : > "$tmp/CHANGELOG.md"
  grep -m1 -F -- "## [${version}]" "$tmp/CHANGELOG.md" || true
  bash "$here/changelog-section.sh" "$version" "$tmp/CHANGELOG.md"
}

head=$(git rev-parse --short HEAD)
notes=$(bash "$here/changelog-section.sh" "$version" CHANGELOG.md)
if [ -z "$notes" ]; then
  echo "CHANGELOG.md at ${head} has no '## [${version}]' section, or it is empty. Add it, merge it, and tag the merge commit." >&2
  exit 1
fi

if [ "$(section_at HEAD)" = "$(section_at HEAD^)" ]; then
  # Name the commit that did write it, newest first.
  wrote=""
  for commit in $(git log --format=%H -- CHANGELOG.md); do
    if [ "$(section_at "$commit")" != "$(section_at "${commit}^")" ]; then
      wrote=$commit
      break
    fi
  done
  since=$(git rev-list --count "${wrote}..HEAD")
  echo "${head} is not the commit that wrote the ${version} section of CHANGELOG.md; $(git log -1 --format='%h ("%s")' "$wrote") is, and the ${since} commit(s) after it would ship without being in the notes. Tag $(git rev-parse --short "$wrote") instead, or bring the section up to date and tag that merge commit." >&2
  exit 1
fi

printf '%s\n' "$notes"
