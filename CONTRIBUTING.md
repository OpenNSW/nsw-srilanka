# Contributing to nsw-srilanka

## Prerequisites

- Go 1.26+ (see `go.mod` for the exact version)
- Docker with BuildKit enabled
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

Pull requests to `main` run three independent pipelines:

### Backend CI (`backend-ci.yml`)
Triggers on `.go`, `go.mod`, `go.sum`, `Dockerfile`, `migrations/**` changes.

1. **quality-gate** — go mod tidy check + golangci-lint
2. **test-and-security** — go test -race + gosec (findings uploaded to GitHub Security)
3. **govulncheck** + **secret-scan** (parallel)

### Portals CI (`portals-ci.yml`)
Triggers on `portals/**` changes.

1. **qa-and-build** — TypeScript type-check + ESLint + Prettier + build
2. **security-scan** — dependency review + pnpm audit

### Docker Validation (`docker-validation.yml`)
Triggers on `portals/**`, `Dockerfile`, Go source, or migration changes.
Builds all three images (trader-portal, nsw-backend, nsw-migrate) without pushing.

All stages must pass before a PR can merge.

## Code style

**Go:** Standard `gofmt` formatting. Linter rules are in [`.golangci.yml`](.golangci.yml). Run `make lint` to check locally.

**TypeScript/React:** ESLint (`tseslint.configs.recommendedTypeChecked`) + Prettier. Run `pnpm run format` from `portals/` to auto-fix formatting.

## Security

The pre-commit hook and CI both run gitleaks. Do not commit real credentials — use the `.env.example` pattern for documenting required environment variables.

Report vulnerabilities privately via GitHub's Security tab (see [SECURITY.md](SECURITY.md)).

## License headers

License policy is pending team discussion. Do not add license headers to source files.
