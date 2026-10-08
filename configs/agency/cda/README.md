# CDA agency

Configuration to run this backend as the Coconut Development Authority, next to
TNSW on the same machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                       |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8083, own database, Temporal namespace, artifacts, authn                                                                 |
| `config.docker.yaml`   | `config.yaml` for the `cda-init` and `cda-api` containers: Postgres, Temporal and the IdP by container hostname                                                               |
| `cda.env`              | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` and `services.json` reference                                                            |
| `catalog.json`         | `officer` → the `CDA Officer` role CDA officers hold in the IdP                                                                                                               |
| `services.json`        | `tnsw`: where the workflows' `notify_decision` step posts the decision back to TNSW (`POST /api/v1/callbacks/{callbackToken}`), and where storage is proxied, as `CDA_TO_NSW` |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                         |
| `services.docker.json` | `services.json` for the `cda-api` container: TNSW at `api:8080`, the IdP at `thunderid:8090`                                                                                  |
| `portal-config.js`     | Runtime config of the `cda-portal` container: the trader-app in agency mode                                                                                                   |
| `branding.json`        | CDA branding for the `cda-portal` container                                                                                                                                   |

CDA keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open, upload and delete the documents in TNSW's storage, calling
TNSW as `CDA_TO_NSW`. TNSW applies its own upload limits.

## Run with Docker Compose

`make dev` and `make preview` start CDA with the rest of the stack. It adds:

| Service      | What it does                                                                                   |
| ------------ | ---------------------------------------------------------------------------------------------- |
| `cda-init`   | One-shot: creates `cda_db` and the `cda` Temporal namespace if missing, then migrates `cda_db` |
| `cda-api`    | The backend in agency mode, on <http://localhost:8083>                                         |
| `cda-portal` | The officer portal, on <http://localhost:5176>                                                 |

`cda-api` and `cda-portal` run the built images even under `make dev` (no hot reload);
rebuild them after a change with `docker compose up -d --build cda-api cda-portal`.

TNSW in compose reaches it at `host.docker.internal:8083`, through the `cda` entry its
`services.docker.json` already has. To change a setting, edit `config.docker.yaml` and
restart `cda-api`. `cda-api` loads its artifacts from a `one-trade-artifacts` clone
next to this repo (`../one-trade-artifacts/cda-v2`); set `CDA_ARTIFACT_LOCAL_ROOT` in
`.env` to use another clone's `cda-v2` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid temporal`):

```sh
# Once: CDA's own database and Temporal namespace
docker compose run --rm cda-init

set -a; . configs/agency/cda/cda.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/services.json` entry `cda`
(`http://localhost:8083`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/cda-v2`); change `artifactLoader.local.root` in `config.yaml`
to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/resources/government/cda.json`](../../../idp/resources/government/cda.json) seeds
CDA for this deployment, on the `CDA API` resource server
(`https://api.cda.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                              | Client               | Grant                                                                                                                      |
| ----------------------------------- | -------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Officer `cda_officer`               | `OGA_PORTAL_APP_CDA` | `CDA Officer` role (via `CDA Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*`         |
| TNSW injecting                      | `NSW_TO_CDA`         | `NswToCdaM2M` role: `nsw:workflow:inject`                                                                                  |
| CDA's decision callback and storage | `CDA_TO_NSW`         | `AgencyM2M` role: `nsw:task:write` on TNSW, which maps `cda` to it in its catalog, and `nsw:storage:read`/`write`/`delete` |

TNSW's `services.json` `cda` entry requests `nsw:workflow:inject` with
`resource=https://api.cda.nsw-agency.local` (see `configs/services*.example.json`).
An IdP seeded before this change still has the old CDA entities, and a re-seed skips
anything that exists: `OGA_PORTAL_APP_CDA` keeps its `agency:*` scopes, `NSW_TO_CDA`
keeps `NswM2M`, and `cda_officer` stays in `OGA Reviewers`. Reset it once, by running
`idp/sample-resources.down.sh` then the seed, or by recreating the IdP volumes
(`docker compose down` and removing the `thunderid-*` volumes).
