# Export Facilitation Center (EFC) agency

Configuration to run this backend as the Export Facilitation Center (EFC), next to TNSW on the same
machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                       |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8085, own database, Temporal namespace, artifacts, authn                                                                 |
| `config.docker.yaml`   | `config.yaml` for the `efc-init` and `efc-api` containers: Postgres, Temporal and the IdP by container hostname                                                               |
| `efc.env`              | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` and `services.json` reference                                                            |
| `catalog.json`         | `officer` → the `EFC Officer` role EFC officers hold in the IdP                                                                                                               |
| `services.json`        | `tnsw`: where the workflows' `notify_decision` step posts the decision back to TNSW (`POST /api/v1/callbacks/{callbackToken}`), and where storage is proxied, as `EFC_TO_NSW` |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                         |
| `services.docker.json` | `services.json` for the `efc-api` container: TNSW at `api:8080`, the IdP at `thunderid:8090`                                                                                  |
| `portal-config.js`     | Runtime config of the `efc-portal` container: the trader-app in agency mode                                                                                                   |
| `branding.json`        | Branding for the `efc-portal` container, as the EFC officer portal had it in nsw-agency                                                                                       |

EFC keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
TNSW as `EFC_TO_NSW`. TNSW applies its own upload limits.

## Run with Docker Compose

`make dev` and `make preview` start EFC with the rest of the stack. It adds:

| Service      | What it does                                                                                   |
| ------------ | ---------------------------------------------------------------------------------------------- |
| `efc-init`   | One-shot: creates `efc_db` and the `efc` Temporal namespace if missing, then migrates `efc_db` |
| `efc-api`    | The backend in agency mode, on <http://localhost:8085>                                         |
| `efc-portal` | The officer portal, on <http://localhost:5178>                                                 |

`efc-api` and `efc-portal` run the built images even under `make dev` (no hot
reload); rebuild them after a change with
`docker compose up -d --build efc-api efc-portal`.

TNSW in compose reaches it at `host.docker.internal:8085`, through the `efc` entry
its `services.docker.json` already has. To change a setting, edit `config.docker.yaml`
and restart `efc-api`. `efc-api` loads its artifacts from a `one-trade-artifacts`
clone next to this repo (`../one-trade-artifacts/efc-v2`); set
`EFC_ARTIFACT_LOCAL_ROOT` in `.env` to use another clone's `efc-v2` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid temporal`):

```sh
# Once: EFC's own database and Temporal namespace
docker compose run --rm efc-init

set -a; . configs/agency/efc/efc.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/services.json` entry `efc`
(`http://localhost:8085`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/efc-v2`); change `artifactLoader.local.root` in
`config.yaml` to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/resources/government/efc.json`](../../../idp/resources/government/efc.json)
seeds EFC for this deployment, on the `EFC API` resource server
(`https://api.efc.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                              | Client               | Grant                                                                                                              |
| ----------------------------------- | -------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Officer `efc_officer`               | `OGA_PORTAL_APP_EFC` | `EFC Officer` role (via `EFC Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*` |
| TNSW injecting                      | `NSW_TO_EFC`         | `NswToEfcM2M` role: `nsw:workflow:inject`                                                                          |
| EFC's decision callback and storage | `EFC_TO_NSW`         | `AgencyM2M` role: `nsw:task:write` and `nsw:storage:read`/`write` on TNSW                                          |

TNSW's `services.json` `efc` entry requests `nsw:workflow:inject` with
`resource=https://api.efc.nsw-agency.local` (see `configs/services*.example.json`).
An IdP seeded before this agency was renamed from Customs still holds the Customs entities
(the `customs` OU, `customs_officer`, `OGA_PORTAL_APP_CUSTOMS`, `CUSTOMS_TO_NSW` and
`NSW_TO_CUSTOMS`), and a re-seed adds the EFC ones beside them. `idp/sample-resources.down.sh`
deletes only what the resource files declare, so remove the Customs entities with the
script and resources from before the rename, then seed again. Or recreate the IdP volumes
(`docker compose down` and removing the `thunderid-*` volumes).
