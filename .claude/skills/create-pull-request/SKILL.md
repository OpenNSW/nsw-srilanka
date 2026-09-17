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

## Preflight

1. `gh --version` and `gh auth status` — if either fails, tell the user and stop; don't try to work around missing auth.
2. `git status` — everything that should be in the PR must be committed. Uncommitted changes are not silently included.
3. `git branch --show-current` for the head branch.
4. Base branch: `gh repo view --json defaultBranchRef --template '{{.defaultBranchRef.name}}'` — don't hardcode `main`.
5. `git fetch origin` then diff against the base (`git diff origin/<base>...HEAD` and `git log origin/<base>..HEAD --oneline`) to see what's actually going in.

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
git push -u origin <branch-name>   # only after confirmation
gh pr create [--draft] --title "<title>" --body-file <scratchpad>/pr-body.md --base "<base-branch>" --head "<branch-name>"
```

If a PR already exists for this branch (`gh pr view <branch>`), use `gh pr edit <number> --body-file ...` instead of creating a second one.

Write the body file to the scratchpad directory, not the repo root — it's not project content and shouldn't risk being committed.

## Wrap up

Give the user the PR URL. Don't leave the scratchpad body file referenced as if it were still needed.
