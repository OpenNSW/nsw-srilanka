# NSW Sri Lanka Platform

[![Go Version](https://img.shields.io/badge/Go-1.26.3-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/NSW-Platform-green.svg)](#)

`nsw-srilanka` is the deployer-specific application repository for the **Sri Lanka instance** of the National Single Window (NSW) Platform.

It depends on the open-source core engine published at `github.com/OpenNSW/core` and wires Sri Lanka–specific service endpoints, payment gateways, and agency workflow configurations on top of it.

---

## Repository Layout

```
nsw-srilanka/
├── cmd/
│   └── server/
│       └── main.go                       # Entry point: loads config, builds the app, runs the HTTP server
├── internal/
│   └── bootstrap/
│       └── app.go                        # Wires DB, Temporal, taskflow, auth, storage, notifications, routes
├── integration/
│   └── payment/                          # Sri Lanka–specific payment gateway implementations (GovPay+)
├── migrations/                           # PostgreSQL migration files (up/down SQL)
├── portals/                              # Trader Portal frontend (React/Vite monorepo)
├── idp/                                  # Identity Provider configuration and seed resources
├── configs/
│   ├── manifest.json                     # Artifact registry manifest — lists all workflow/form configs
│   ├── services.json                     # Remote service endpoints (gitignored)
│   ├── services.example.json             # Template for services.json
│   ├── payment_methods.json              # Payment gateway catalogue (gitignored)
│   ├── payment_methods.example.json      # Template for payment_methods.json
│   ├── notification.json                 # Notification provider configuration
│   ├── fcau/                             # FCAU health-certificate workflow, JSONForms, render configs
│   ├── trade/                            # Trade (consignment) workflow and form configs
│   └── npqs/                             # NPQS workflow and form configs
├── .env.example                          # Template for environment variables
├── .gitignore
├── Dockerfile
├── go.mod
└── go.sum
```

The agency-specific workflow definitions live under `configs/<agency_code>/` as JSON files (workflow graphs, JSONForms schemas, render configs). All behaviour is configured through these JSON files — the Go server itself is intentionally thin. The `configs/manifest.json` file is the index that tells the artifact registry which files to load at startup.

For a comprehensive guide to authoring and modifying workflow and form configuration files, see [WORKFLOW_GUIDE.md](WORKFLOW_GUIDE.md).

---

## How to Run Locally

### 1. Prepare local config files
Copy the templates and edit each one for your environment:
```bash
cp .env.example .env
cp configs/services.example.json configs/services.json
cp configs/payment_methods.example.json configs/payment_methods.json
```

### 2. Start the Docker Stack
The repository provides a `compose.yml` stack that brings up all backing services (PostgreSQL, IDP, Temporal), the Go backend API, and the Trader Portal frontend. Use the `Makefile` targets:

```bash
make dev      # development: hot reload (air for Go, Vite HMR for the portal)
make preview  # build and run the real images from the Dockerfiles, locally
make help     # list all targets
```

This spins up:
* **`nsw-postgres`** (Port `5432`): Database populated with base tables/schemas.
* **`nsw-idp`** (Port `8090`): Thunder Identity Provider.
* **`temporal`** (Port `7233`) & **`temporal-ui`** (Port `8233`): Temporal workflow orchestration engine.
* **`nsw-backend-api`** (Port `8080`): The Go backend server.
* **`nsw-trader-portal`** (Port `5173`): The React Trader Portal frontend.

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
2. **Prepare env** — the template is already tuned for native runs (`DB_HOST=localhost`, `TEMPORAL_HOST=localhost`, `AUTH_JWKS_URL=https://localhost:8090`, `SERVICES_CONFIG_PATH=./configs/services.json`):
   ```bash
   cp .env.example .env
   ```
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

- Health check: `curl http://localhost:8080/health` should return `{"status":"ok","service":"nsw-backend"}`.
- Logs will report DB connection, Temporal worker startup, and the workflow artifact registrations from `configs/manifest.json`.

### 5. Simulating a payment webhook (dev only)

INFO-type gateways (e.g. `govpay`) don't fire a real callback. To advance a `PENDING_PAYMENT` task manually:

```bash
curl -X POST http://localhost:8080/api/v1/payments/govpay/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "reference_number": "TNSW-XXXXXXXX",
    "session_id": "manual-test-1",
    "gateway_transaction_id": "MOCK-001",
    "status": "SUCCESS",
    "amount": "1500",
    "currency": "LKR",
    "payment_method": "govpay",
    "timestamp": "2026-01-01T00:00:00Z",
    "metadata": {}
  }'
```

REDIRECT-type gateways (e.g. `lankapay`) fire this webhook on their own after a successful redirect.

---

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

| File                           | Purpose                                                                  | Source of truth                        |
|--------------------------------|--------------------------------------------------------------------------|----------------------------------------|
| `.env`                         | Runtime environment (DB, Temporal, CORS, auth, storage, config paths)    | `.env.example`                         |
| `configs/manifest.json`        | Artifact registry index — lists every workflow/form/render config file   | Committed to the repository            |
| `configs/services.json`        | Outbound service endpoint registry (FCAU, NPQS, IRD, customs, …)         | `configs/services.example.json`        |
| `configs/payment_methods.json` | Payment gateway catalogue (id, type, gateway URL, instruction template)  | `configs/payment_methods.example.json` |
| `configs/notification.json`    | Notification provider settings (SMS, email channels)                     | Committed to the repository            |
| `configs/<agency_code>/`       | Agency workflow definitions, JSONForms schemas, and render configs       | Committed to the repository            |

Workflow execution mechanics (input/output mappings, task plugins, render projections) are documented in [WORKFLOW_GUIDE.md](WORKFLOW_GUIDE.md) and the `github.com/OpenNSW/core` README.
