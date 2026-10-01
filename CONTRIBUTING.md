# Contributing to nsw-srilanka

## Prerequisites

- Go 1.27+ (see `go.mod` for the exact version)
- Docker with BuildKit — required, not just recommended: the Dockerfiles use
  `FROM --platform=$BUILDPLATFORM`, which the legacy builder cannot parse. Docker
  >= 23 and Compose v2 default to BuildKit, so this only bites if you have
  exported `DOCKER_BUILDKIT=0`.
- pnpm 11.1.2+ (for frontend work only)
- Node.js 22.18.0+ (see `portals/.nvmrc`)

## First-time setup

Run `make setup` from the repo root. This installs the Go quality tools and configures git hooks so quality checks run automatically before every commit and push.

```bash
make setup
```

## Available make targets

| Target         | Description                                                  |
|----------------|--------------------------------------------------------------|
| `make setup`   | Install Go tools and configure git hooks                     |
| `make tools`   | Install Go quality tools only (golangci-lint, gosec, etc.)  |
| `make fmt`     | Format all Go source files with gofmt                        |
| `make lint`    | Run golangci-lint                                            |
| `make tidy`    | Run go mod tidy                                              |
| `make test`    | Run all tests with the race detector                         |
| `make vuln`    | Run govulncheck against the Go vulnerability database        |
| `make secrets` | Run gitleaks secret scan on the repository                   |
| `make check`   | Run all quality checks in sequence (tidy → fmt → lint → test)|
| `make dev`     | Start the full docker-compose stack with hot reload          |
| `make deps`    | Start dependencies only (run Go API natively via go.work)    |
| `make preview` | Build and run the real images locally                        |
| `make migration name=<desc>` | Scaffold a new SQL migration file                |

For frontend-specific targets, see `portals/Makefile`.

## Local quality workflow

The pre-commit hook runs automatically on `git commit`. It checks:
1. **gitleaks** — scans staged files for accidentally committed secrets
2. **gofmt** — verifies staged Go files are formatted
3. **golangci-lint** — lints the packages containing staged files
4. **go build** — verifies the module compiles
5. **go mod tidy** — fails if go.mod/go.sum would change

The pre-push hook runs `go test -race ./...` before every push.

To skip an individual check in an emergency:
```bash
SKIP_LINT=1 git commit -m "..."   # skip lint only
SKIP_TESTS=1 git push              # skip tests only
```

## CI pipeline

Pull requests to `main` run these pipelines. The code pipelines run only when
their files change, and otherwise skip their jobs; a skipped check counts as
passed.

### Backend CI (`backend-ci.yml`)
Runs when `.go` files, `go.mod`, `go.sum`, `Dockerfile`, `migrations/**` or `Makefile` change.

1. **Quality Gate** — go mod tidy check + golangci-lint
2. **Test & Security** — go test -race + gosec (findings uploaded to GitHub Security)
3. **Go Vulnerability Check** — govulncheck (advisory, PRs only)

### Portals CI (`portals-ci.yml`)
Runs when `portals/**` changes.

1. **Quality Check & Build** — TypeScript type-check + ESLint + Prettier + build
2. **Security Scan** — dependency review + pnpm audit (advisory, PRs only)

### Docker Validation (`docker-validation.yml`)
Runs when `portals/**`, `Dockerfile`, Go source, `go.mod`/`go.sum`, `migrations/**`,
`configs/**` or `.dockerignore` change. Builds all three images (tnsw-web,
tnsw-api, tnsw-migrate) without pushing; **Docker images** reports the result.

### Chart Validation (`chart-validation.yml`)
Runs when `deployments/helm/**` changes. **Lint, Render & Package Helm Chart**
packages the chart the way a release does, then lints and renders it.

### Secret Scan (`secret-scan.yml`)
Runs on every PR. **Secret Scan** checks the PR's commits for credentials with
gitleaks.

### PR Title (`pr-title.yml`)
Runs on every PR. **Conventional Commit Title** checks the title follows
Conventional Commits.

### PR Labels (`pr-labels.yml`)
Runs on every PR. Labels it from its title and author, then fails the
**Release notes label** check until it carries a release-notes label (see
[Releasing](#releasing)).

### Version Check (`version-check.yml`)
Runs on every PR. When the PR changes `version.txt`, it runs the checks the
release will run, so a bad release PR fails in review; otherwise it passes at
once.

### Required checks and the merge queue
`main` merges through a merge queue, and these checks must pass:
**Conventional Commit Title**, **Release notes label**, **Release version**,
**Secret Scan**, **Quality Gate**, **Test & Security**, **Quality Check & Build**,
**Docker images** and **Lint, Render & Package Helm Chart**.

The queue tests each PR on top of `main` and the PRs queued ahead of it. The
code checks and **Release version** run again there, when the combined change
touches their files, so PRs that pass on their own but break together don't
reach `main`. **Conventional Commit Title**, **Release notes label** and
**Secret Scan** check the PR itself and skip in the queue.

## Releasing

[`version.txt`](version.txt) holds the released version as its tag name (`v0.2.0`); CHANGELOG headings use the number alone (`## [0.2.0]`). Merging a PR that changes it releases that version: [`auto-tag.yml`](.github/workflows/auto-tag.yml) tags the merge commit and starts [`release.yml`](.github/workflows/release.yml), which builds and publishes the tnsw-api, tnsw-web and tnsw-migrate images, then creates the GitHub Release. The release opens with the version's section of [CHANGELOG.md](CHANGELOG.md), then the image digests, then GitHub's list of the PRs merged since the previous release. Nobody tags by hand.

### Release-notes labels

Every PR needs a label that places it in that list. PRs titled `feat` or `fix`, or marked breaking with `!`, are labelled for you; PRs opened by bots and agents get `skip-changelog`, which a maintainer can remove to list one.

| Label | Section of the release notes |
|--------------------|-------------------------------------------------------------------------|
| `breaking change`  | Breaking changes. Add it next to another label whenever deployers must act. |
| `Type/New Feature` | New features |
| `Type/Improvement` | Improvements |
| `Type/Bug`         | Bug fixes |
| `Type/Task`        | Other changes |
| `skip-changelog`   | Left out: CI, tests, refactors and anything else deployers won't notice. Not allowed with `breaking change`. |

Fill in the PR template's **Deployment Notes** whenever a deployer has to do something — run a migration, set a new or changed env var or config file, change the IdP. The CHANGELOG is written from them.

### Cutting a release

1. **Open a release PR** from `main` that does two things:
   - sets `version.txt` to the new version, e.g. `v0.2.0`;
   - moves `## [Unreleased]` under `## [X.Y.Z] - YYYY-MM-DD` in CHANGELOG.md, written for the people who deploy TNSW: upgrade notes first, then what was added, changed and fixed. In Claude Code, the `draft-changelog` skill drafts it from the merged PRs and their Deployment Notes. Use absolute links; the section is copied into the GitHub Release.
   - dates that heading **the day the PR will merge, in Sri Lanka time** (UTC+05:30): `TZ=Asia/Colombo date +%F`.

   The **Release version** check confirms the version is valid, higher than the last one and not yet released, and that the PR writes its CHANGELOG section.
2. **Merge it.** That is the release: the merge commit is tagged and released. Merge it only when you mean to ship, and if the PR has waited, update the heading's date to today first. The release run warns when the date and the merge day differ.
3. **If the release fails at "Release Notes"**, nothing was built. Fix the cause in a new release PR; the tag is only created once the checks pass. To retry a release that failed later on, re-run the failed run of `release.yml`.

A tag pushed by hand still starts `release.yml`, but only releases if `version.txt` and CHANGELOG.md at that commit agree with it. Never use `git push --tags`.

While on 0.x, a breaking change or a new feature bumps the minor version (0.2.0 → 0.3.0), and fixes alone bump the patch (0.2.0 → 0.2.1).

The Helm chart is released with the app, in lockstep: `release.yml` publishes `lk-tnsw` at the same version, with `appVersion` set to it, so chart X.Y.Z deploys the X.Y.Z images by default. [`Chart.yaml`](deployments/helm/lk-tnsw/Chart.yaml) holds only placeholders — don't bump it. A chart-only change ships in the next release.

## Code style

**Go:** Standard `gofmt` formatting. Linter rules are in [`.golangci.yml`](.golangci.yml). Run `make lint` to check locally.

**TypeScript/React:** ESLint (`tseslint.configs.recommendedTypeChecked`) + Prettier. Run `pnpm run format` from `portals/` to auto-fix formatting.

## Security

The pre-commit hook and CI both run gitleaks. Do not commit real credentials — use the `.env.example` pattern for documenting required environment variables.

Report vulnerabilities privately via GitHub's Security tab (see [SECURITY.md](SECURITY.md)).

## License headers

License policy is pending team discussion. Do not add license headers to source files.
