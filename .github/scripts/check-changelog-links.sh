#!/usr/bin/env bash
# Checks the GitHub pull request, issue and commit links in one version's
# CHANGELOG.md section through the GitHub API: each pull request must be
# merged, and each issue and commit must exist. Link Check (lychee.toml) leaves
# these links out, so a release PR checks the ones its section adds here.
# Errors go to stderr, one line per bad link. Needs gh signed in; in Actions,
# GH_TOKEN set to the job's token.
#
# Usage: check-changelog-links.sh VERSION [FILE]    FILE defaults to CHANGELOG.md
set -euo pipefail

version="${1:?usage: check-changelog-links.sh VERSION [FILE]}"
file="${2:-CHANGELOG.md}"
here=$(dirname "$0")

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

# Prints FIELD of the API resource at PATH, or nothing when FIELD is null. A
# 5xx or network error is tried again, up to three times, five seconds apart.
# When the resource is missing, fails and prints gh's error.
lookup() {
  local out attempt
  for attempt in 1 2 3; do
    if out=$(gh api "$1" --jq "$2 // empty" 2> "$tmp"); then
      printf '%s' "$out"
      return 0
    fi
    grep -q '(HTTP 4' "$tmp" && break
    [ "$attempt" -lt 3 ] && sleep 5
  done
  sed 's/^gh: //' "$tmp"
  return 1
}

links=$(bash "$here/changelog-section.sh" "$version" "$file" \
  | grep -oE 'https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/((pull|issues)/[0-9]+|commit/[0-9a-fA-F]+)' \
  | sort -u || true)

failed=0
for url in $links; do
  IFS=/ read -r owner repo kind id <<< "${url#https://github.com/}"
  case "$kind" in
    pull) path="repos/${owner}/${repo}/pulls/${id}" field=.merged_at ;;
    issues) path="repos/${owner}/${repo}/issues/${id}" field=.number ;;
    commit) path="repos/${owner}/${repo}/commits/${id}" field=.sha ;;
  esac
  if ! found=$(lookup "$path" "$field"); then
    echo "CHANGELOG.md's ${version} section links to ${url}, which the GitHub API can't find: ${found}" >&2
    failed=1
  elif [ -z "$found" ]; then
    echo "CHANGELOG.md's ${version} section links to ${url}, which isn't merged." >&2
    failed=1
  fi
done
exit "$failed"
