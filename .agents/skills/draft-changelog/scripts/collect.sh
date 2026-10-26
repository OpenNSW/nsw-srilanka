#!/usr/bin/env bash
# Collects what the draft-changelog skill needs to write a CHANGELOG section:
# every PR merged between two refs, grouped the way the GitHub Release groups
# them, each with its "Deployment Notes"; then the changes to the files that
# decide how TNSW is deployed. Read-only: it fetches nothing and edits nothing.
#
# Usage: collect.sh --from <ref> [--to <ref>]     --to defaults to origin/main
set -euo pipefail

REPO=OpenNSW/nsw-srilanka
usage() { echo "usage: collect.sh --from <ref> [--to <ref>]" >&2; exit 2; }

from="" to="origin/main"
while [ $# -gt 0 ]; do
  case "$1" in
    --from) from="${2:-}"; shift 2 ;;
    --to) to="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$from" ] && [ -n "$to" ] || usage
for ref in "$from" "$to"; do
  git rev-parse --verify --quiet "${ref}^{commit}" >/dev/null || { echo "collect.sh: unknown ref '$ref'" >&2; exit 1; }
done
for tool in gh jq; do
  command -v "$tool" >/dev/null || { echo "collect.sh: needs $tool" >&2; exit 1; }
done

range="${from}..${to}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Squash merges end their subject with "(#NNN)"; the last one is the PR.
# Anything else reached main without a PR and is listed on its own.
git log --first-parent --reverse --format='%h%x09%s' "$range" > "$tmp/commits"
: > "$tmp/prs"
: > "$tmp/direct"
while IFS=$'\t' read -r sha subject; do
  n=$(printf '%s\n' "$subject" | sed -nE 's/.*\(#([0-9]+)\)[[:space:]]*$/\1/p')
  if [ -n "$n" ]; then echo "$n" >> "$tmp/prs"; else printf '%s %s\n' "$sha" "$subject" >> "$tmp/direct"; fi
done < "$tmp/commits"

# One line of JSON per PR: where it lands in the release notes, and its
# Deployment Notes with the template's placeholder text removed.
while read -r n; do
  gh pr view "$n" --repo "$REPO" --json number,title,url,labels,author,body > "$tmp/pr.json"
  notes=$(jq -r '.body // ""' "$tmp/pr.json" | tr -d '\r' | awk '
    /^##[[:space:]]*Deployment Notes/ { f = 1; next }
    f && /^##[[:space:]]/ { exit }
    f && !/^\(If applicable/ { print }
  ' | sed -e '/./,$!d' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}')
  case "$(printf '%s' "$notes" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:].')" in
    ""|n/a|na|none|-) notes="" ;;
  esac
  jq -c --arg notes "$notes" '
    ([.labels[].name]) as $labels
    | (.title | capture("^(?<type>[a-z]+)(\\([^)]*\\))?(?<bang>!)?:")? // {type: "", bang: null}) as $t
    | {number, title, url, author: .author.login, bot: .author.is_bot, labels: $labels, notes: $notes,
       section: (
         if   ($labels | index("skip-changelog"))   then "Left out (skip-changelog)"
         elif ($labels | index("breaking change"))  then "Breaking changes"
         elif ($labels | index("Type/New Feature")) then "New features"
         elif ($labels | index("Type/Improvement")) then "Improvements"
         elif ($labels | index("Type/Bug"))         then "Bug fixes"
         elif ($labels | index("Type/Task"))        then "Other changes"
         elif .author.is_bot                        then "Left out (bot)"
         elif $t.bang                               then "Breaking changes (by title, unlabelled)"
         elif $t.type == "feat"                     then "New features (by title, unlabelled)"
         elif $t.type == "fix"                      then "Bug fixes (by title, unlabelled)"
         else "Other changes (unlabelled)" end)}
  ' "$tmp/pr.json" >> "$tmp/prs.jsonl"
done < "$tmp/prs"
touch "$tmp/prs.jsonl"

echo "# Release input: ${from} → ${to}"
echo
echo "$(wc -l < "$tmp/prs" | tr -d ' ') pull requests, $(wc -l < "$tmp/direct" | tr -d ' ') commits without one."
for section in "Breaking changes" "New features" "Improvements" "Bug fixes" "Other changes" "Left out"; do
  jq -r --arg s "$section" '
    select(.section | startswith($s))
    | "- [#\(.number)](\(.url)) \(.title) — @\(.author)\(if .bot then " (bot)" else "" end)"
      + (if (.section | test("unlabelled")) then " — _\(.section)_" else "" end)
      + (if .notes != "" then "\n  - Deployment Notes: " + (.notes | gsub("\n"; "\n    ")) else "" end)
  ' "$tmp/prs.jsonl" > "$tmp/section"
  if [ -s "$tmp/section" ]; then
    echo; echo "## ${section}"; echo; cat "$tmp/section"
  fi
done
if [ -s "$tmp/direct" ]; then
  echo; echo "## Commits without a PR"; echo; sed 's/^/- /' "$tmp/direct"
fi

# Deployer-facing files. Full diffs for the small, high-signal ones; a summary
# for the rest. An upgrade note needs one of these (or a Deployment Note) behind it.
echo; echo "# Deployer-facing changes"
echo; echo "## Files changed"; echo; echo '```'
git diff --stat "$range" -- migrations .env.example 'configs/*.example.json' compose.yml deployments/helm \
  Dockerfile portals/apps/trader-app/Dockerfile portals/apps/trader-app/docker-entrypoint.sh \
  idp internal/scopes cmd/server/config go.mod
echo '```'

echo; echo "## Migrations added or removed"; echo; echo '```'
git diff --name-status --diff-filter=ADR "$range" -- migrations
echo '```'

echo; echo "## Environment variables read by the server (added and removed lines)"; echo; echo '```'
git diff "$range" -- cmd/server/config | grep -E '^[+-][^+-].*(getEnv|Getenv|LookupEnv|getIntEnv|getBool)' || true
echo '```'

echo; echo "## Diffs"
for path in .env.example 'configs/*.example.json' deployments/helm/values-example.yaml \
  deployments/helm/lk-tnsw/values.yaml deployments/helm/lk-tnsw/Chart.yaml \
  portals/apps/trader-app/docker-entrypoint.sh internal/scopes go.mod; do
  if ! git diff --quiet "$range" -- "$path"; then
    echo; echo "### ${path}"; echo; echo '```diff'
    git diff "$range" -- "$path"
    echo '```'
  fi
done
