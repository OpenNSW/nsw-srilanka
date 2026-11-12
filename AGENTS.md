# Repository Guidelines

## Project Structure & Module Organization

Sri Lanka’s National Single Window uses `OpenNSW/core`.

- `cmd/server/`: server entry point; `cmd/otc/`: artifact configuration CLI.
- `internal/`: application wiring, domains, authentication, tasks, and storage.
- `external-integration/`: GovPay, ASYCUDA, ePhyto, and SLPA integrations.
- `portals/apps/trader-app/`: React/TypeScript frontend; source in `src/`, static assets in `public/`.
- `portals/packages/ui/`: `@portals/ui`, the generic components and design tokens (`theme.css`) the portals share; domain components stay in the app.
- `portals/packages/theme/`: `@portals/theme`, which paints a deployment's `branding.theme` into Radix's CSS variables; the token names stay in `@portals/ui`.
- `migrations/`: PostgreSQL migrations; `test/e2e/replay/`: replay tests and JSON fixtures.
- `configs/`, `idp/`, `deployments/helm/`, and `docs/`: runtime configuration, identity setup, deployment charts, and documentation.

Workflow and form artifacts live separately in `OpenNSW/one-trade-artifacts` under `tnsw/` and load at startup.

## Build, Test, and Development Commands

Use Go 1.27+, Docker with BuildKit and Compose v2, Node.js 24.15+, and pnpm 12.16+.

- `make setup`: install tools, configure hooks, and seed local configuration; install golangci-lint v2 first.
- `make dev`: start the full stack with hot reload.
- `make build`: build container images; `make preview`: build and run them.
- `make deps`: start backing services for native development with `go run ./cmd/server`.
- `make check`: tidy modules, format, lint, and run race-enabled Go tests.
- `make mdlint`: lint Markdown.
- From `portals/`, run `pnpm install`, `pnpm dev-trader`, and `pnpm build`; validate with `pnpm lint`, `pnpm type-check`, `pnpm test`, and `pnpm format:check`.

## Coding Style & Naming Conventions

Follow `.editorconfig`: tabs for Go and Makefiles; two spaces for TypeScript, JSON, and YAML. Use `gofmt`, goimports, and `.golangci.yml`. Frontend Prettier uses single quotes, no semicolons, and trailing commas. Use PascalCase React components and `useX` hook names. Follow existing package boundaries and ESLint import rules. Do not add license headers.

Write comments about the code as it is now, not about how it got there. Don't mention earlier behavior, where code used to live, what was renamed, or the issues and PRs that changed it, and don't use words like "now", "no longer" or "used to". That history belongs in the commit message and the PR. A reference to an open issue for planned work belongs in a `TODO(#NNN)` comment.

## API Contracts

The OpenAPI specs in `api/*.openapi.yaml` are the HTTP API's contract; nothing generates them, so they drift unless edited by hand. Change the spec in the same PR as any change to a path, parameter, request or response field, or status code. A field can appear in several schemas and specs (e.g. a summary and a detail DTO, or the consignment and admin APIs), so grep `api/` for its name and update every one. Note a removed or renamed field in the PR's Deployment Notes and mark it breaking with `!`.

OAuth scopes are part of the contract too. `internal/scopes/scopes.go` names the scope each route requires, and the trader portal shows each screen and button by the same names in `portals/apps/trader-app/src/constants/scopes.ts`. Nothing checks that the two match, and a mismatch hides the portal's screens without an error, so add, rename or remove a scope in both in the same PR. Grep for the scope and update every other place that names it: the specs' "Requires scope" text, the IdP bootstrap and `idp/README.md`, each `IDP_SCOPES` (`config.example.js`, `configs/agency/*/portal-config.js`, the Helm values), and the replay fixtures. Note a removed or renamed scope in the PR's Deployment Notes and mark it breaking with `!`.

The workflow and form artifacts in `OpenNSW/one-trade-artifacts` are released separately, so keep TNSW backward compatible with the artifacts already released: a deployment upgrades TNSW without changing its artifacts. Add what artifacts need as something they opt in to, and never change how TNSW reads what released artifacts already use. Each artifacts release names the oldest TNSW release it works with, so a PR's Deployment Notes never ask for an artifacts release or pin.

## Testing Guidelines

Place Go tests beside implementation as `*_test.go`, using `TestXxx`, standard `testing`, Testify, and SQL mocks where appropriate. Run `make test`; no numeric coverage threshold is configured. Run replay tests serially with `make deps` followed by `make test-e2e`; this stops the API container. Place frontend tests beside the code as `*.test.ts(x)`, using Vitest (importing from `vitest`, no globals) and Testing Library; run them with `pnpm test` from `portals/`.

Every change that adds or changes behaviour comes with tests in the same PR: each new or changed function, handler or component gets a test beside it covering its main path, its error paths and its edge cases, not only through the screen or service that uses it. A change that can't be tested this way (styling only, wiring) says why in the PR's Testing section.

## Commit & Pull Request Guidelines

Name branches `<type>/<issue>-<short-slug>`, e.g. `chore/952-order-config-sections`, using the issue the work closes; create the issue first if there is none. Settle the name before pushing: renaming a fork branch closes its open PR. Use Conventional Commits, e.g. `fix(storage): describe refused file type`; mark breaking changes with `!`. PR titles follow the same format. Complete `.github/pull_request_template.md` with rationale, validation, related issues, screenshots for UI changes, and deployment notes. Include a release-notes label and pass applicable CI checks. Never edit `CHANGELOG.md` by hand in a feature or fix PR: only a release PR writes it, drafted from the merged PRs' Deployment Notes (the `draft-changelog` skill).

## Configuration & Migrations

Keep credentials out of Git; document placeholders in example files. Keep the server config's top-level sections in this order, most important first: `mode`, `server`, `db`, `temporal`, `storage`, `authn`, `catalog`, `artifactLoader`, `artifacts`, `cors`, `services`, `payment`, `refid`, `notification`, `integrations`, `consignment`, `otel`. It applies to `configs/config.example.yaml`, `configs/config.docker.example.yaml`, `configs/agency/*/config*.yaml` (which leave out the sections they don't use), `backend.config` in `deployments/helm/values-example.yaml`, and the fields of `Config` in `cmd/server/config/config.go`; put a new section in its place in every one. Generate migrations with `make migration name=add_example_table`; use sequential `NNNNNN_name.sql` files with reversible `@UP`/`@DOWN` blocks. Never edit applied migrations. See `CONTRIBUTING.md` for details.
