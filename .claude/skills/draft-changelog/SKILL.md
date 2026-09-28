---
name: draft-changelog
description: Draft the CHANGELOG.md section for the next TNSW release (vX.Y.Z), written for the people who deploy it — upgrade steps first, then what was added, changed and fixed — from the pull requests merged since the last release, their "Deployment Notes", and the changes to migrations, env vars, config, IdP scopes and the Helm chart. Use it whenever someone is preparing a release or asks for release notes, a changelog entry, what changed since the last release, or what deployers need to know for the next version, even if they don't say "changelog". It edits CHANGELOG.md and version.txt only; it never tags, pushes, merges or publishes.
---

# Draft Changelog Skill

Drafts the next release's section of `CHANGELOG.md`. Deployers read that section first: the release workflow copies it to the top of the GitHub Release, above the image digests and GitHub's generated list of every merged PR. So the section answers one question — *what do I need to know, or do, to run this version?* — and leaves the complete PR list to the release page.

## Hard rules

- **Edit `CHANGELOG.md` and `version.txt` only, and stop at a draft.** Never create a tag, push, merge, or publish a release. Merging the release PR tags and publishes images for every deployer; that is a person's call, made after review.
- **Back every statement with evidence** — a PR, its Deployment Notes, or a diff from the collector. When a PR looks like it needs an upgrade step but nothing says what, read its diff; if it is still unclear, list it for the user as an open question instead of guessing. A wrong upgrade note does more harm than a missing one.
- **Use absolute links** (`https://github.com/OpenNSW/nsw-srilanka/...`). The section is copied into the GitHub Release, where relative links break.
- **Write for deployers, not developers.** Say what changes for a running deployment and what to do about it. Leave out refactors, tests, CI and internal renames unless a deployer would notice them.

## Preflight

1. `gh auth status` — if it fails, tell the user and stop.
2. `git fetch origin`, and work from `origin/main`: releases are cut from it, and drafting on a feature branch describes the wrong code. Unless the user already has a release branch, create one: `git switch -c chore/release-vX.Y.Z origin/main`.
3. Find the previous release on GitHub, and resolve its commit on `origin` — local clones can carry stale test tags with the same names:
   ```bash
   prev=$(gh release list --repo OpenNSW/nsw-srilanka --exclude-drafts --exclude-pre-releases --limit 1 --json tagName --jq '.[0].tagName')
   from=$(git ls-remote origin "refs/tags/${prev}^{}" | cut -f1)       # annotated tag
   [ -n "$from" ] || from=$(git ls-remote origin "refs/tags/${prev}" | cut -f1)
   ```
   No release yet? Ask the user which commit to start from.
4. Agree the version with the user. Suggest one from what you collect: while on 0.x, a breaking change or a new feature means the next minor (0.2.0 → 0.3.0), fixes alone the next patch (0.2.0 → 0.2.1).

## Collect

Run the bundled collector and read all of its output:

```bash
.claude/skills/draft-changelog/scripts/collect.sh --from "$from" --to origin/main > <scratch dir>/release-input.md
```

Write it outside the repo (your scratchpad, or `/tmp`) so it is never committed. It is read-only and takes about a second per PR. It prints:
- every PR in the range, grouped the way the GitHub Release will group them (by label, or by title type where a PR was merged without one), each with its Deployment Notes;
- the deployer-facing changes: files changed, migrations added or removed, env vars the server started or stopped reading, and diffs of `.env.example`, the Helm values, the portal's runtime settings, the API scopes and `go.mod`.

Then read CONTRIBUTING.md's "Releasing" section for the conventions, and the newest section of `CHANGELOG.md` to match its voice.

## Write the section

Put it directly under `## [Unreleased]` — moving in anything already there — headed `## [X.Y.Z] - YYYY-MM-DD` with the planned tag date (ask if you don't know it). Use these subsections in this order, and leave out any that would be empty:

- `### Upgrade notes` — what a deployer must do or know when moving from the previous version: new migrations (say if one is slow or can't be rolled back), env vars and config keys that are new, renamed or removed (with defaults), config file format changes, IdP changes (scopes, clients, roles, groups), Helm values changes, and everything labelled `breaking change`. Say what to change, not only that something changed.
- `### Added`, `### Changed`, `### Fixed` — the changes a deployer or an agency would notice, one line each with the PR linked. Fold related PRs (a backend and a frontend part, say) into one line. Don't list every PR; the release page already does.
- `### Security` — fixes and hardening worth knowing about.
- `### Known issues` — the previous section's known issues that still hold, plus new ones.

The difference between a deployer's line and a restated PR title:

- Deployer's line: "Resolving a parked workflow step needs the new `nsw:consignment:adminwrite` scope: add it to the `NSW Admin` role in ThunderID, and have admins sign in again. ([#489](https://github.com/OpenNSW/nsw-srilanka/pull/489))"
- Restated title (avoid): "feat(admin): resolve nodes parked for admin intervention (backend) [Part 1/2] (#489)"

Set `version.txt` to `vX.Y.Z`, the tag name; the CHANGELOG heading stays `X.Y.Z`. Merging the release PR is what releases that version.

Finally, update the link references at the foot of the file:

```
[Unreleased]: https://github.com/OpenNSW/nsw-srilanka/compare/vX.Y.Z...HEAD
[X.Y.Z]: https://github.com/OpenNSW/nsw-srilanka/compare/vPREV...vX.Y.Z
```

## Check

1. `bash .github/scripts/changelog-section.sh X.Y.Z` must print exactly your section. It is the extraction the release workflow runs, and the release fails before building anything if it prints nothing.
2. `bash .github/scripts/changelog-section.sh X.Y.Z | grep -oE '\]\([^)]*\)' | grep -v '^](https://'` must print nothing — any output is a relative link.
3. `bash .github/scripts/check-version.sh vX.Y.Z <previous vX.Y.Z>` passes, and `git status --short` shows only `CHANGELOG.md` and `version.txt` modified.
4. Re-read the Upgrade notes against the collector output: each one has a source.

## Hand off

Show the user the drafted section, then the open questions: PRs that look like they need an upgrade step but say nothing, PRs merged without a release-notes label, and anything you could not confirm. Don't commit unless asked. The release PR goes through the `create-pull-request` skill. Tell the user plainly that merging it releases `vX.Y.Z` — the merge commit is tagged and published automatically — so they merge only when ready to ship.
