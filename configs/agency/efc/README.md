# Export Facilitation Center (EFC) agency

Configuration to run this backend as the Export Facilitation Center (EFC), next to TNSW on the same
machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                 | Purpose                                                                                                                                   |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`        | Server config for a native run: `mode: agency`, port 8085, own database, Temporal namespace, artifacts, authn, outbound services, catalog |
| `config.docker.yaml` | `config.yaml` for the `efc-init` and `efc-api` containers: Postgres, Temporal and the IdP by container hostname                           |
| `efc.env`            | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` references                                           |
| `portal-config.js`   | Runtime config of the `efc-portal` container: the trader-app in agency mode                                                               |
| `branding.json`      | Branding for the `efc-portal` container, as the EFC officer portal had it in nsw-agency                                                   |

EFC keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
TNSW as `EFC_TO_NSW`. Officers' uploads go to TNSW's upload route (`nsw:storage:write`), and the files this agency generates to its internal upload route (`nsw:storage:internalwrite`). `storage.proxy.allowedUploadTypes` is what its officers may upload.

The YAML's `services` list registers `tnsw` for storage and the workflows'
`notify_decision` callbacks (`POST /api/v1/callbacks/{callbackToken}`).

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
its `config.docker.yaml` services list already has. To change a setting, edit `config.docker.yaml`
and restart `efc-api`. `efc-api` loads its artifacts from a `one-trade-artifacts`
clone next to this repo (`../one-trade-artifacts/efc`); set
`EFC_ARTIFACT_LOCAL_ROOT` in `.env` to use another clone's `efc` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid temporal`):

```sh
# Once: EFC's own database and Temporal namespace
docker compose run --rm efc-init

set -a; . configs/agency/efc/efc.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/config.yaml` services entry `efc`
(`http://localhost:8085`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/efc`); change `artifactLoader.local.root` in
`config.yaml` to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/bootstrap/20-agency-efc.yaml`](../../../idp/bootstrap/20-agency-efc.yaml)
creates EFC for this deployment, on the `EFC API` resource server
(`https://api.efc.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                            | Client               | Grant                                                                                                                            |
| --------------------------------- | -------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Officer `efc_officer`             | `OGA_PORTAL_APP_EFC` | `EFC Officer` role (via `EFC Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*`               |
| TNSW injecting and replying       | `NSW_TO_EFC`         | `NswToEfcM2M` role: `nsw:workflow:inject`, and `nsw:task:write` to reply on EFC's parked steps, which its catalog maps `tnsw` to |
| EFC decision callback and storage | `EFC_TO_NSW`         | `EfcToNswM2M` role: `nsw:task:write` and `nsw:storage:read`/`write`/`internalwrite` on TNSW                                      |

TNSW's `config.yaml` services entry `efc` requests `nsw:workflow:inject` and `nsw:task:write` with
`resource=https://api.efc.nsw-agency.local` (see `configs/config*.example.yaml`).
An IdP seeded before the move to `idp/bootstrap/` holds the same entities under other
IDs, so `thunderid-setup` fails on it. Recreate the IdP volumes once (`docker compose
down` and remove the `thunderid-*` volumes; see
[idp/README.md](../../../idp/README.md#changing-resources)).
