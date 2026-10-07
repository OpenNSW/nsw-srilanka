# Customs agency

Configuration to run this backend as Sri Lanka Customs, next to TNSW on the same
machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                           |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8085, own database, Temporal namespace, artifacts, authn                                                                     |
| `config.docker.yaml`   | `config.yaml` for the `customs-init` and `customs-api` containers: Postgres, Temporal and the IdP by container hostname                                                           |
| `customs.env`          | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` and `services.json` reference                                                                |
| `catalog.json`         | `officer` → the `Customs Officer` role Customs officers hold in the IdP                                                                                                           |
| `services.json`        | `tnsw`: where the workflows' `notify_decision` step posts the decision back to TNSW (`POST /api/v1/callbacks/{callbackToken}`), and where storage is proxied, as `CUSTOMS_TO_NSW` |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                             |
| `services.docker.json` | `services.json` for the `customs-api` container: TNSW at `api:8080`, the IdP at `thunderid:8090`                                                                                  |
| `portal-config.js`     | Runtime config of the `customs-portal` container: the trader-app in agency mode                                                                                                   |
| `branding.json`        | Branding for the `customs-portal` container, as the Customs officer portal had it in nsw-agency                                                                                   |

Customs keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open, upload and delete the documents in TNSW's storage, calling
TNSW as `CUSTOMS_TO_NSW`. TNSW applies its own upload limits.

## Run with Docker Compose

`make dev` and `make preview` start Customs with the rest of the stack. It adds:

| Service          | What it does                                                                                               |
| ---------------- | ---------------------------------------------------------------------------------------------------------- |
| `customs-init`   | One-shot: creates `customs_db` and the `customs` Temporal namespace if missing, then migrates `customs_db` |
| `customs-api`    | The backend in agency mode, on <http://localhost:8085>                                                     |
| `customs-portal` | The officer portal, on <http://localhost:5178>                                                             |

`customs-api` and `customs-portal` run the built images even under `make dev` (no hot
reload); rebuild them after a change with
`docker compose up -d --build customs-api customs-portal`.

TNSW in compose reaches it at `host.docker.internal:8085`, through the `customs` entry
its `services.docker.json` already has. To change a setting, edit `config.docker.yaml`
and restart `customs-api`. `customs-api` loads its artifacts from a `one-trade-artifacts`
clone next to this repo (`../one-trade-artifacts/customs-v2`); set
`CUSTOMS_ARTIFACT_LOCAL_ROOT` in `.env` to use another clone's `customs-v2` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid temporal`):

```sh
# Once: Customs' own database and Temporal namespace
docker compose run --rm customs-init

set -a; . configs/agency/customs/customs.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/services.json` entry `customs`
(`http://localhost:8085`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/customs-v2`); change `artifactLoader.local.root` in
`config.yaml` to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/resources/government/customs.json`](../../../idp/resources/government/customs.json)
seeds Customs for this deployment, on the `Customs API` resource server
(`https://api.customs.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                                 | Client                   | Grant                                                                                                                      |
| -------------------------------------- | ------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| Officer `customs_officer`              | `OGA_PORTAL_APP_CUSTOMS` | `Customs Officer` role (via `Customs Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*` |
| TNSW injecting                         | `NSW_TO_CUSTOMS`         | `NswToCustomsM2M` role: `nsw:workflow:inject`                                                                              |
| Customs' decision callback and storage | `CUSTOMS_TO_NSW`         | `AgencyM2M` role: `nsw:task:write` and `nsw:storage:read`/`write`/`delete` on TNSW                                         |

TNSW's `services.json` `customs` entry requests `nsw:workflow:inject` with
`resource=https://api.customs.nsw-agency.local` (see `configs/services*.example.json`).
An IdP seeded before this change still has the old Customs entities, and a re-seed skips
anything that exists: `OGA_PORTAL_APP_CUSTOMS` keeps its `agency:*` scopes,
`NSW_TO_CUSTOMS` keeps `NswM2M`, and `customs_officer` stays in `OGA Reviewers`. Reset it
once, by running `idp/sample-resources.down.sh` then the seed, or by recreating the IdP
volumes (`docker compose down` and removing the `thunderid-*` volumes).
