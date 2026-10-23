---
name: create-pull-request
description: Create or update a GitHub pull request for this repository using its standard template and release-notes labels. Use when the user asks to open a PR, submit changes for review, or update an existing PR.
---

# Create Pull Request Skill

Create a GitHub PR using this repo's official template. Read `AGENTS.md` and the relevant sections of `CONTRIBUTING.md` for current repository requirements.

## Hard rules

- **Reproduce every checkbox from the template, verbatim, unchecked by default.** Mark `[x]` only for items concretely true. Never drop, collapse, or hide an unchecked box to make the PR look tidier — visibility of what's _not_ done is the point.
- **Be concise.** Each section is a few words to a couple of sentences. This is a filled-in template, not a report.
- **Describe only the end state of the change** — what it does and why. Never narrate how the session got there: no mention of earlier drafts, corrections, back-and-forth, or things that were tried and reverted. The reader wants the diff's rationale, not its history.

## Resolve the target repo (fork-safe)

The PR must always land on `OpenNSW/nsw-srilanka`, never on a contributor's fork, and this must be figured out explicitly — not left to `gh`'s ambient remote-priority guessing, which is easy to get wrong silently.

1. `origin_url=$(git remote get-url origin)` and parse `OWNER/REPO` out of it (handles both `git@github.com:OWNER/REPO.git` and `https://github.com/OWNER/REPO` forms).
2. Compare against the canonical `OpenNSW/nsw-srilanka`:
   - **Same repo** (the common case: you cloned it directly) → `origin` is both the push target and the PR's base repo. No owner-qualification needed on `--head`.
   - **Different repo** (fork workflow: `origin` is `<your-username>/nsw-srilanka`) → you still push to `origin` (you don't have write access to upstream), but the PR's base repo is always `OpenNSW/nsw-srilanka`, and `--head` must be qualified as `<origin-owner>:<branch-name>` or `gh` will look for that branch in the base repo and fail (or worse, silently target the wrong repo).
3. Always pass `--repo OpenNSW/nsw-srilanka` explicitly on repository-scoped `gh pr` calls. For `gh repo view`, pass the canonical repository as its positional argument. Authentication and version commands need no repository flag.

## Preflight

1. `gh --version` and `gh auth status` — if either fails, prepare the local title and body and report the publishing blocker; don't try to work around missing auth.
2. `git status` — everything that should be in the PR must be committed. Uncommitted changes are not silently included.
3. `git branch --show-current` for the head branch.
4. Base branch: honor a user-specified base; otherwise resolve it with `gh repo view OpenNSW/nsw-srilanka --json defaultBranchRef --template '{{.defaultBranchRef.name}}'`. Don't hardcode `main` or resolve the base against a fork.
5. Fetch `origin`. For a direct clone of the canonical repository, inspect `git diff origin/<base>...HEAD` and `git log origin/<base>..HEAD`. For a fork, verify that `upstream` points to the canonical repository and refresh the base with `git fetch upstream <base>`; if there is no verified upstream remote, use `git fetch https://github.com/OpenNSW/nsw-srilanka.git <base>` instead. After that fetch succeeds, inspect `git diff FETCH_HEAD...HEAD` and `git log FETCH_HEAD..HEAD`. Review the full PR diff and commits against the freshly fetched canonical base, never a potentially stale fork or upstream ref.

## Get the template

Read `.github/pull_request_template.md` from repo root. If it doesn't exist, ask the user whether to proceed without one rather than inventing a format.

## Fill it out

- **Title**: concise, follows this repo's commit convention (`feat(scope): ...`, `fix(scope): ...`, etc. — check `git log` for the actual prevailing style). Mark a breaking change with `!` (`feat(authz)!: ...`).
- **Release-notes label**: the required **Release notes label** check fails until the PR has one (the table is in CONTRIBUTING.md, "Releasing"). The PR Labels workflow adds it by itself for `feat` → `Type/New Feature`, `fix` → `Type/Bug` and `!` → `breaking change`. For any other type, pick one: `Type/Improvement` or `Type/Task` if a deployer would notice the change, `skip-changelog` if not (CI, tests, internal refactors). Add `breaking change` whenever deployers must act, even without `!` in the title, and never together with `skip-changelog`.
- **Summary**: what the PR does and why, in a sentence or two — not a changelog.
- **Type of Change / Testing / Checklist**: per the hard rules above — full checkbox list, only concretely-true items checked.
- **Changes Made**: bullet the actual diff (files/areas + what changed), not the commit messages restated.
- **Related Issues**: use issues established by the user or repository context. Treat a branch issue number as a lead to verify. Use `Closes #<n>` only when the change resolves it, otherwise `Related to #<n>`. Write `N/A` when no issue is identified.
- **Deployment Notes**: what someone running TNSW must do for this change — a migration, a new or changed env var or config file, an IdP change (scopes, clients, roles), Helm values — and what to set. The next release's CHANGELOG is written from this section, so check the diff (`migrations/`, `.env.example`, `configs/`, `deployments/helm/`, `idp/`) rather than guessing. `N/A` only when nothing changes for a running deployment.
- **Screenshots/Additional Notes**: fill in if relevant, `N/A` if not — don't delete the sections.

## Authorization and review state

- Follow the user's requested scope and authorization already given in the session. A request to create or update a PR authorizes the necessary push and PR operations; a request only to prepare a title or body authorizes local preparation. Loading this skill or finding committed changes does not authorize publication.
- Prepare the title, body, release-notes labels, and relevant validation before requesting any missing authorization. Show that concrete result and identify the action requiring approval. Do not ask again when existing session authorization covers it. Tool-enforced approvals still apply.
- Honor an explicit draft or ready-for-review preference. Default new PRs to `--draft` when unspecified, and preserve an existing PR's review state unless the user asks to change it.

## Execute

```bash
git push -u origin <branch-name>   # to origin, within the user's authorized scope; do not force-push

# same-repo clone:
gh pr create [--draft] --repo OpenNSW/nsw-srilanka \
  --title "<title>" --body-file <scratchpad>/pr-body.md \
  --base "<base-branch>" --head "<branch-name>" [--label "<label>"]

# fork clone (origin != OpenNSW/nsw-srilanka):
gh pr create [--draft] --repo OpenNSW/nsw-srilanka \
  --title "<title>" --body-file <scratchpad>/pr-body.md \
  --base "<base-branch>" --head "<origin-owner>:<branch-name>" [--label "<label>"]
```

Pass `--label` for the labels the workflow won't add (everything except the `feat`/`fix`/`!` ones), once per label. Labelling needs triage access to `OpenNSW/nsw-srilanka`: if `gh` reports it couldn't add a label, the PR may still have been created — check with `gh pr view`, and tell the user a maintainer must add the label before the check can pass.

Before creating a PR, check for an existing open PR using `gh pr list --repo OpenNSW/nsw-srilanka --head <branch-name> --base <base> --state open --json number,headRefName,headRepositoryOwner`. For this lookup, `--head` takes the bare branch name, without an owner prefix. Select the result whose `headRepositoryOwner.login` matches `<origin-owner>` and whose `headRefName` matches `<branch-name>`. Use the owner-qualified head only for `gh pr create` in a fork workflow. If a matching PR exists, use `gh pr edit --repo OpenNSW/nsw-srilanka <number> --title ... --body-file ...` instead of creating a second one, with `--add-label` / `--remove-label` if its label should change. Preserve unrelated labels. After a publishing error, check whether the operation succeeded before retrying.

Write the body file with actual newlines to a temporary directory outside the repository, such as `$TMPDIR`, and pass it through `--body-file`. Avoid shell interpolation of the body text.

## Wrap up

Verify the resulting URL, base, head, body, labels, and draft state with `gh pr view --repo OpenNSW/nsw-srilanka <number>`. Give the user the PR URL and any unresolved validation or maintainer action. Don't leave the temporary body file referenced as if it were still needed.
