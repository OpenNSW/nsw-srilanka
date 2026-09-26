# NSW Sri Lanka Platform

[![Go Version](https://img.shields.io/badge/Go-1.27-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/NSW-Platform-green.svg)](#)

`nsw-srilanka` is the deployer-specific application repository for the **Sri Lanka instance** of the National Single Window (NSW) Platform.

It depends on the open-source core engine published at [github.com/OpenNSW/core](https://github.com/OpenNSW/core) and wires Sri Lanka–specific service endpoints, payment gateways, and agency workflow configurations on top of it.

---

## Repository Layout

```
nsw-srilanka/
├── cmd/
│   ├── server/                            # Main API entry point: loads config, builds the app, runs the HTTP server
│   └── otc/                               # CLI for applying one-trade-artifacts config against a running instance
├── internal/                              # Application code — bootstrap wiring, consignment/profile domains,
│                                           # task plugins & authz, replay engine, audit, catalog, static data
├── external-integration/                  # Sri Lanka–specific integrations
│   ├── payment/                           #   Payment gateways (GovPay+)
│   ├── customs/                          #   Customs declaration integration (ASYCUDA)
│   ├── ephyto/                           #   IPPC ePhyto Hub client
│   └── slpa/                             #   Sri Lanka Ports Authority (CMS, gate pass, consolidation, invoicing…)
├── test/e2e/replay/                       # End-to-end replay tests (config-driven harness, mock agency/gateway)
├── migrations/                            # PostgreSQL migration files (up/down SQL)
├── portals/                                # Trader Portal frontend (React/Vite monorepo)
├── idp/                                    # Identity Provider (ThunderID) configuration and seed resources
├── deployments/helm/                       # Helm chart for deploying the stack
├── docs/                                   # WORKFLOW_GUIDE.md and other developer docs
├── configs/                                 # Runtime configs only (no workflow/form artifacts)
│   ├── services.docker.example.json         # Template for services.docker.json (Docker Compose — container hostnames)
│   ├── services.example.json                # Template for services.json (local/native dev — localhost)
│   ├── payment_methods.example.json         # Template for payment_methods.json
│   ├── notification.example.json            # Template for notification.json
│   ├── catalog.example.json                 # Template for catalog.json
│   └── companies.example.json               # Template for companies.json
├── compose.yml / compose.override.yml     # Docker Compose stack (base + dev hot-reload override)
├── .env.example                            # Template for environment variables
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

The agency-specific workflow definitions (workflow graphs, JSONForms schemas, render configs) are **not** committed to this repo — keeping the application deployment-free. They live in the public repo [OpenNSW/one-trade-artifacts](https://github.com/OpenNSW/one-trade-artifacts) under the `tnsw/` base path, and are fetched at startup by the pluggable artifact loader (configured via `ARTIFACT_*` env — see [`.env.example`](.env.example)). All behaviour is configured through those JSON files — the Go server itself is intentionally thin. The `tnsw/manifest.json` file is the index that tells the artifact registry which files to load at startup.

For a comprehensive guide to authoring and modifying workflow and form configuration files, see [WORKFLOW_GUIDE.md](docs/WORKFLOW_GUIDE.md).

---

## How to Run Locally
>[!NOTE]
> ⚠️ **This quickstart is for local development only.** The example configs enable
> insecure TLS (`AUTH_JWKS_INSECURE_SKIP_VERIFY=true`, `insecure_skip_tls_verify`
> in `services.json`) for the self-signed local IdP. The backend only honors these
> when `APP_ENV=development` — which `make dev`/`make preview` inject and nothing
> else does — so a raw `docker compose up` or any real deployment **refuses to
> start** on them. For non-local environments, trust the IdP/agency certificate
> chain, keep those flags off, and never set `APP_ENV=development`.

### 1. First-time setup

```bash
make setup
```

This installs the Go quality tools, configures the git hooks (see [CONTRIBUTING.md](CONTRIBUTING.md)), and seeds every local config file from its `*.example` template — `.env`, `idp/.env`, and `configs/{services.docker,payment_methods,notification,catalog,companies}.json`. Existing files are never overwritten, so it is safe to re-run.

> [!NOTE]
> `make setup` requires **golangci-lint v2** to already be installed (`brew install golangci-lint`); it stops with an error otherwise.

Edit the seeded files for your environment before starting the stack. See the [Configuration Reference](#configuration-reference) for what each one holds.

### 2. Start the Docker Stack
The repository provides a `compose.yml` stack that brings up all backing services (PostgreSQL, IDP, Temporal), the Go backend API, and the Trader Portal frontend. Use the `Makefile` targets:

```bash
make dev      # development: hot reload (air for Go, Vite HMR for the portal)
make preview  # build and run the real images from the Dockerfiles, locally
make help     # list all targets
```

This spins up:
* **`nsw-postgres`** (Port `5432`): Database populated with base tables/schemas.
* **`thunderid`** (Port `8090`): ThunderID Identity Provider.
* **`temporal`** (Port `7233`) & **`temporal-ui`** (Port `8233`): Temporal workflow orchestration engine.
* **`tnsw-api`** (Port `8080`): The Go backend server.
* **`tnsw-web`** (Port `5173`): The React Trader Portal frontend.

> [!IMPORTANT]
> **`docker compose up` gives you the *development* stack.**
> A `compose.override.yml` sits next to `compose.yml` and Docker Compose
> **auto-merges it** on any bare `docker compose` command — so a plain
> `docker compose up` runs the hot-reload dev stack (stock language images,
> source bind-mounted, `air`/Vite recompiling in place). The real built images
> from the Dockerfiles are **only** used when the override is excluded with
> `-f compose.yml`.
>
> | Goal                  | Command                                                                 |
> |-----------------------|-------------------------------------------------------------------------|
> | Dev (hot reload)      | `make dev` &nbsp;·&nbsp; `docker compose up`                            |
> | Preview (real images) | `make preview` &nbsp;·&nbsp; `docker compose -f compose.yml up --build` |
>
> **CI/deploy scripts that shell out to `docker compose` directly must pass
> `-f compose.yml`** (or call `make preview`), otherwise they will silently build
> and run the dev stack.

In development, edits to Go files trigger an `air` rebuild and frontend edits hot-reload via Vite — no image rebuild and no container restart needed.

The Trader Portal frontend runs as part of the stack — `make dev` serves it via the Vite dev server at `http://localhost:5173`, with backend requests going to `localhost:8080` and auth to the Thunder IDP at `localhost:8090`. No separate repository or process is required.

### 3. Iterating on Go code

In `make dev`, the API container runs [`air`](https://github.com/air-verse/air) against your bind-mounted source. Saving a `.go` file triggers an automatic rebuild and restart of the server inside the container — usually a second or two — while PostgreSQL, Temporal, and the IDP keep running undisturbed. There is nothing to restart manually.

To watch the rebuild output:
```bash
make logs
```

#### Working against the core engine (`OpenNSW/core`)

This repo depends on the core engine as a normal, version-pinned Go module — there is **no** sibling clone, `replace` directive, or `GOWORK` setting involved by default (see [Upstream Dependency](#upstream-dependency)). Two common workflows:

* **Bump to a newer engine release** — update `go.mod` to a new version and let the dev container pick it up on its next rebuild:
  ```bash
  go get github.com/OpenNSW/core@latest
  go mod tidy
  ```
* **Develop the engine and this repo together** — use the [native cross-repo workflow](#native-cross-repo-development) below for a live edit loop across both repositories.

#### Native cross-repo development

The dev container is hermetic: it builds from the pinned `go.mod` version, ignores any `go.work` (`GOWORK=off`), and does **not** mount sibling repos. That's intentional — it keeps every container build reproducible. When you need to edit `OpenNSW/core` and see the change live, run the **Go API natively on your host** and use Docker only for the backing services:

1. **Clone `OpenNSW/core`** next to `nsw-srilanka` and create a workspace (`go.work` is gitignored, so this stays personal):
   ```bash
   go work init . ../core
   ```
2. **Prepare config** — the `.env` seeded by `make setup` already points the DB, Temporal and IdP at `localhost`. The host binary also needs the `localhost` variant of the service endpoints, which `make setup` does not seed:
   ```bash
   cp configs/services.example.json configs/services.json
   ```
   then in `.env`, switch `SERVICES_CONFIG_PATH` to the commented-out `./configs/services.json` line.
3. **Start everything except the API and portal** (db, temporal, idp, migrations, …) so you run those two natively:
   ```bash
   make deps
   ```
4. **Run the API on the host**, where your `go.work` is fully honored:
   ```bash
   go run ./cmd/server
   ```

Edits in `OpenNSW/core` are now picked up by the host compiler, and you get a native debugger. Because `docker compose` reads the same `.env`, the published service ports and the ports your host binary connects to stay in sync automatically (e.g. `DB_PORT`).

> Don't mix the two: if `make dev` is already running, its `api` container holds port `8080` — run `make down` (or just `docker compose stop api`) before starting the native server.

---

### 4. Verify

- Health check: `curl http://localhost:8080/health` should return `{"status":"ok","service":"tnsw-api"}`.
- Logs will report DB connection, Temporal worker startup, and the workflow artifact registrations loaded via the artifact loader from `tnsw/manifest.json` in [OpenNSW/one-trade-artifacts](https://github.com/OpenNSW/one-trade-artifacts) (the default; configurable via `ARTIFACT_*` env).

### 5. Simulating a payment webhook (dev only)

INFO-type gateways (e.g. `govpay`) don't fire a real callback. To advance a `PENDING_PAYMENT` task manually:

```bash
curl -X POST "http://localhost:8080/api/v1/payments/govpay/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "transactionID": "TNSWPYRMTWMY",
    "subinstId": "sub-001",
    "serviceid": "FCAU",
    "serviceName": "FCAU Application Fee",
    "data": [
      {
        "seq": "1",
        "paramName": "refNo",
        "value": "TNSWPYRMTWMY"
      },
      {
        "seq": "2",
        "paramName": "amount",
        "value": "1500"
      },
      {
        "seq": "3",
        "paramName": "currency",
        "value": "LKR"
      },
      {
        "seq": "4",
        "paramName": "status",
        "value": "SUCCESS"
      },
      {
        "seq": "5",
        "paramName": "paymentMethod",
        "value": "ONLINE_BANKING"
      }
    ]
  }'
```

REDIRECT-type gateways (e.g. `lankapay`) fire this webhook on their own after a successful redirect.

---

## Database migrations

Schema migrations live in `migrations/` as `NNN_name.sql` files, each holding a
`-- @UP` and a `-- @DOWN` block. They are applied by the standalone migrator from
[`agency`](https://github.com/OpenNSW/agency) (`backend/cmd/migrate`),
which tracks applied versions in a `__migrations` table and runs each migration
in its own transaction.

**In Docker (default):** the `migrate` service builds the dedicated `migrate`
image target (SQL files baked in), runs `migrate up` to completion, and the `api`
service waits on it via `depends_on: service_completed_successfully`. A fresh
`db` volume already creates the database, so there is no separate "create DB"
step. To reset everything, wipe the volume:

```bash
make clean        # docker compose down -v — drops the db volume
make deps         # brings the stack back up; migrate re-applies from scratch
```

Run ad-hoc commands against the running stack:

```bash
docker compose run --rm migrate status   # show applied / pending
docker compose run --rm migrate down     # roll back the latest migration
```

**Locally (native, without Docker):** install the tool once, then point it at
your database. Note the env var names differ slightly from the app's
(`DB_USER`, not `DB_USERNAME`):

```bash
# Read the pin out of the Dockerfile rather than repeating it — MIGRATE_VERSION
# there is the single source of truth, and compose and the Makefile both defer to it.
go install github.com/OpenNSW/agency/backend/cmd/migrate@"$(sed -n 's/^ARG MIGRATE_VERSION=//p' Dockerfile)"

DB_DRIVER=postgres MIGRATION_DIR=./migrations \
  DB_HOST=localhost DB_PORT="$DB_PORT" DB_NAME="$DB_NAME" \
  DB_USER="$DB_USERNAME" DB_PASSWORD="$DB_PASSWORD" \
  migrate up        # or: status | down | generate <name>
```

`migrate generate <name>` scaffolds the next `NNN_<name>.sql` with empty
`@UP`/`@DOWN` stubs.

## Upstream Dependency

The core engine is pulled directly from GitHub via Go modules:

```
github.com/OpenNSW/core v0.0.0-…  // pinned to a specific commit
```

To pull the latest release:

```bash
go get github.com/OpenNSW/core@latest
go mod tidy
```

There is **no** `replace` directive and **no** sibling clone of `OpenNSW/core` required to build.

The `OpenNSW/core` SDK provides all the infrastructure building blocks used by this application — workflow orchestration, task management, payment gateways, authentication, storage, notifications, and more. See the [core README](https://github.com/OpenNSW/core) for the full package reference and architecture overview.

---

## Configuration Reference

| File                           | Purpose                                                                          | Source of truth                        |
|--------------------------------|----------------------------------------------------------------------------------|----------------------------------------|
| `.env`                         | Runtime environment (DB, Temporal, CORS, auth, storage, config paths)            | `.env.example`                         |
| `idp/.env`                     | Identity Provider environment (client IDs, secrets, JWKS config)                 | `idp/.env.example`                     |
| `configs/services.docker.json` | Outbound service endpoints — uses Docker container hostnames (for `compose.yml`) | `configs/services.docker.example.json` |
| `configs/services.json`        | Outbound service endpoints — uses `localhost` (for native/host dev runs)         | `configs/services.example.json`        |
| `configs/payment_methods.json` | Payment gateway catalogue (id, type, gateway URL, instruction template)          | `configs/payment_methods.example.json` |
| `configs/notification.json`    | Notification provider settings (SMS, email channels)                             | `configs/notification.example.json`    |
| `configs/catalog.json`         | Global catalog — logical names → IdP token roles and OAuth2 client ids           | `configs/catalog.example.json`         |
| `configs/companies.json`       | Seed company/trader records (registration, VAT/TIN, per-agency IDs)              | `configs/companies.example.json`       |

Workflow execution mechanics (input/output mappings, task plugins, render projections) are documented in [WORKFLOW_GUIDE.md](docs/WORKFLOW_GUIDE.md) and the `github.com/OpenNSW/core` README.
