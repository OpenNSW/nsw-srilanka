---
name: create-pull-request
description: Create a GitHub pull request for this repository using its standard template (.github/pull_request_template.md). Use when the user asks to create/open a PR, submit or publish committed changes to GitHub, or when changes are committed and ready to be pushed/submitted for review.
---

# Create Pull Request Skill

Creates a GitHub PR using this repo's official template, never a freehand summary.

## Hard rules

- **Reproduce every checkbox from the template, verbatim, unchecked by default.** Mark `[x]` only for items concretely true. Never drop, collapse, or hide an unchecked box to make the PR look tidier — visibility of what's *not* done is the point.
- **Be concise.** Each section is a few words to a couple of sentences. This is a filled-in template, not a report.
- **Describe only the end state of the change** — what it does and why. Never narrate how the session got there: no mention of earlier drafts, corrections, back-and-forth, or things that were tried and reverted. The reader wants the diff's rationale, not its history.

## Resolve the target repo (fork-safe)

The PR must always land on `OpenNSW/nsw-srilanka`, never on a contributor's fork, and this must be figured out explicitly — not left to `gh`'s ambient remote-priority guessing, which is easy to get wrong silently.

1. `origin_url=$(git remote get-url origin)` and parse `OWNER/REPO` out of it (handles both `git@github.com:OWNER/REPO.git` and `https://github.com/OWNER/REPO` forms).
2. Compare against the canonical `OpenNSW/nsw-srilanka`:
   - **Same repo** (the common case: you cloned it directly) → `origin` is both the push target and the PR's base repo. No owner-qualification needed on `--head`.
   - **Different repo** (fork workflow: `origin` is `<your-username>/nsw-srilanka`) → you still push to `origin` (you don't have write access to upstream), but the PR's base repo is always `OpenNSW/nsw-srilanka`, and `--head` must be qualified as `<origin-owner>:<branch-name>` or `gh` will look for that branch in the base repo and fail (or worse, silently target the wrong repo).
3. Always pass `--repo OpenNSW/nsw-srilanka` explicitly on every `gh` call in this skill (`gh repo view`, `gh pr view`, `gh pr create`/`edit`) — don't rely on the current-directory default, which is exactly what breaks for forks.

## Preflight

1. `gh --version` and `gh auth status` — if either fails, tell the user and stop; don't try to work around missing auth.
2. `git status` — everything that should be in the PR must be committed. Uncommitted changes are not silently included.
3. `git branch --show-current` for the head branch.
4. Base branch: `gh repo view OpenNSW/nsw-srilanka --json defaultBranchRef --template '{{.defaultBranchRef.name}}'` — don't hardcode `main`, but always resolve it against the explicit canonical repo, not the ambient one.
5. `git fetch origin` then diff against the base. If `origin` is a fork, `origin/<base>` may be a stale mirror of upstream's base branch — if an `upstream` remote exists, fetch and diff against that instead (`git fetch upstream && git diff upstream/<base>...HEAD`); otherwise diff against `origin/<base>` and note to the user that the local diff may not reflect the very latest upstream base (GitHub computes the real diff server-side regardless).

## Get the template

Read `.github/pull_request_template.md` from repo root. If it doesn't exist, ask the user whether to proceed without one rather than inventing a format.

## Fill it out

- **Title**: concise, follows this repo's commit convention (`feat(scope): ...`, `fix(scope): ...`, etc. — check `git log` for the actual prevailing style).
- **Summary**: what the PR does and why, in a sentence or two — not a changelog.
- **Type of Change / Testing / Checklist**: per the hard rules above — full checkbox list, only concretely-true items checked.
- **Changes Made**: bullet the actual diff (files/areas + what changed), not the commit messages restated.
- **Related Issues**: check the branch name for an issue number (`fix/123-...`, `123-...`, etc.) and use `Closes #<n>` if found; otherwise ask the user once, or write `N/A` if they say there isn't one.
- **Screenshots/Deployment/Additional Notes**: fill in if relevant, `N/A` if not — don't delete the sections.

## Confirm before acting

- Show the user the draft title + body before doing anything remote.
- **Always ask before `git push`**, even if this branch already has an open PR and you're just adding a follow-up commit — a prior push approval does not carry forward.
- Ask whether the PR should be opened as a draft or ready for review. Default to `--draft` if the user doesn't say and there's no clear signal otherwise (e.g. they haven't asked for review yet).

## Execute

```bash
git push -u origin <branch-name>   # always to origin, even in a fork — only after confirmation

# same-repo clone:
gh pr create [--draft] --repo OpenNSW/nsw-srilanka \
  --title "<title>" --body-file <scratchpad>/pr-body.md \
  --base "<base-branch>" --head "<branch-name>"

# fork clone (origin != OpenNSW/nsw-srilanka):
gh pr create [--draft] --repo OpenNSW/nsw-srilanka \
  --title "<title>" --body-file <scratchpad>/pr-body.md \
  --base "<base-branch>" --head "<origin-owner>:<branch-name>"
```

If a PR already exists for this branch (`gh pr view --repo OpenNSW/nsw-srilanka <origin-owner>:<branch-name>` — or just `<branch-name>` in the same-repo case), use `gh pr edit --repo OpenNSW/nsw-srilanka <number> --body-file ...` instead of creating a second one.

Write the body file to the scratchpad directory, not the repo root — it's not project content and shouldn't risk being committed.

## Wrap up

Give the user the PR URL. Don't leave the scratchpad body file referenced as if it were still needed.
