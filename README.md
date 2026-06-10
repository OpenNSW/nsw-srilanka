# NSW Sri Lanka Platform

[![Go Version](https://img.shields.io/badge/Go-1.25.7-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/NSW-Platform-green.svg)](#)

`nsw-srilanka` is the deployer-specific application repository for the **Sri Lanka instance** of the National Single Window (NSW) Platform.

It depends on the core engine published as `github.com/OpenNSW/nsw/backend` (taskv2 branch) and wires Sri Lanka–specific service endpoints, payment gateways, and FCAU workflow configurations on top of it.

---

## Repository Layout

```
nsw-srilanka/
├── cmd/
│   └── server/
│       └── main.go                       # Entry point: loads config, builds the app, runs the HTTP server
├── internal/
│   └── bootstrap/
│       └── app.go                        # Wires DB, Temporal, taskv2, auth, storage, notifications, routes
├── configs/
│   ├── services.json                     # Remote service endpoints (gitignored)
│   ├── services.example.json             # Template for services.json
│   ├── payment_methods.json              # Payment gateway catalogue (gitignored)
│   ├── payment_methods.example.json      # Template for payment_methods.json
│   └── fcau/                             # FCAU health-certificate workflow + JSONForms (gitignored)
├── .env.example                          # Template for environment variables
├── .gitignore
├── Dockerfile
├── go.mod
└── go.sum
```

The Sri Lanka–specific FCAU workflow lives under `configs/fcau/` as a set of JSON files (workflow graph, JSONForms, render configs). The Go server itself is intentionally thin — most behaviour is configured via these JSON files and the `github.com/OpenNSW/nsw/backend` module.

---

## How to Run Locally

### 1. Prepare local config files
Copy the templates and edit each one for your environment:
```bash
cp .env.example .env
cp configs/services.example.json configs/services.json
cp configs/payment_methods.example.json configs/payment_methods.json
```
The FCAU workflow JSON tree under `configs/fcau/` is gitignored — make sure you have pulled it from your team's workflow template store before running the application.

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

#### Working against the core engine (`OpenNSW/nsw/backend`)

This repo depends on the core engine as a normal, version-pinned Go module — there is **no** sibling clone, `replace` directive, or `GOWORK` setting involved (see [Upstream Dependency](#upstream-dependency)). Two common workflows:

* **Bump to a newer engine build** — point `go.mod` at a new commit and let the dev container pick it up on its next rebuild:
  ```bash
  go get github.com/OpenNSW/nsw/backend@taskv2
  go mod tidy
  ```
* **Develop the engine and this repo together** — push your engine change to a branch and `go get` that ref, or use the [native cross-repo workflow](#native-cross-repo-development) below for a live edit loop across repositories.

#### Native cross-repo development

The dev container is hermetic: it builds from the pinned `go.mod` version, ignores any `go.work` (`GOWORK=off`), and does **not** mount sibling repos. That's intentional — it keeps every container build reproducible. When you need to edit `OpenNSW/nsw` (or another sibling) and see the change live, run the **Go API natively on your host** and use Docker only for the backing services:

1. **Clone the siblings** next to `nsw-srilanka` and create a workspace (`go.work` is gitignored, so this stays personal):
   ```bash
   go work init . ../nsw/backend ../nsw-task-flow ../go-temporal-workflow
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

Edits in the sibling repos are now picked up by the host compiler, and you get a native debugger. Because `docker compose` reads the same `.env`, the published service ports and the ports your host binary connects to stay in sync automatically (e.g. `DB_PORT`).

> Don't mix the two: if `make dev` is already running, its `api` container holds port `8080` — run `make down` (or just `docker compose stop api`) before starting the native server.

---

### 4. Verify

- Health check: `curl http://localhost:8080/health` should return `{"status":"ok","service":"nsw-backend"}`.
- Logs will report DB connection, Temporal worker startup, and the FCAU workflow registrations from `configs/fcau/`.

### 5. Simulating a payment webhook (dev only)

INFO-type gateways (e.g. `govpay`) don't fire a real callback. To advance a `PENDING_PAYMENT` task manually:

```bash
curl -X POST http://localhost:8080/api/v1/payments/webhook \
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

REDIRECT-type gateways (e.g. `lankapay`) fire this webhook on their own.

---

## Database migrations

Schema migrations live in `migrations/` as `NNN_name.sql` files, each holding a
`-- @UP` and a `-- @DOWN` block. They are applied by the standalone migrator from
[`nsw-agency`](https://github.com/OpenNSW/nsw-agency) (`backend/cmd/migrate`),
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
# Pin the same version the Docker image uses (see MIGRATE_VERSION in the Dockerfile / Makefile).
go install github.com/OpenNSW/nsw-agency/backend/cmd/migrate@v0.0.0-20260610120959-d981e67a7a47

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
github.com/OpenNSW/nsw/backend v0.0.0-…  // pinned to a taskv2 commit
```

To pull the latest taskv2:

```bash
go get github.com/OpenNSW/nsw/backend@taskv2
go mod tidy
```

There is **no** `replace` directive and **no** sibling clone of `OpenNSW/nsw` required to build.

---

## Configuration Reference

| File                           | Purpose                                                                  | Source of truth                        |
|--------------------------------|--------------------------------------------------------------------------|----------------------------------------|
| `.env`                         | Runtime environment (DB, Temporal, CORS, auth, storage, config paths)    | `.env.example`                         |
| `configs/services.json`        | Outbound service endpoint registry (FCAU, NPQS, IRD, customs, …)         | `configs/services.example.json`        |
| `configs/payment_methods.json` | Payment gateway catalogue (id, type, gateway URL, instruction template)  | `configs/payment_methods.example.json` |
| `configs/fcau/`                | FCAU health-certificate workflow definition + JSONForms + render configs | Per-team workflow store                |

Workflow execution mechanics (input/output mappings, task plugins, render projections) are documented in the upstream `github.com/OpenNSW/nsw/backend` README and the FCAU configs themselves.
