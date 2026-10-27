# SLTB agency

Configuration to run this backend as the Sri Lanka Tea Board (SLTB),
next to TNSW on the same machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8086, own database, Temporal namespace, artifacts, authn, outbound services, reference ID formats, catalog        |
| `config.docker.yaml`   | `config.yaml` for the `sltb-init` and `sltb-api` containers: Postgres, Temporal and the IdP by container hostname                                                      |
| `sltb.env`             | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` references                                                                        |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                  |
| `portal-config.js`     | Runtime config of the `sltb-portal` container: the trader-app in agency mode                                                                                           |
| `branding.json`        | Branding for the `sltb-portal` container, as the SLTB officer portal had it in nsw-agency                                                                              |

SLTB keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
TNSW as `SLTB_TO_NSW`. Officers' uploads go to TNSW's upload route (`nsw:storage:write`), and the files this agency generates to its internal upload route (`nsw:storage:internalwrite`). `storage.proxy.allowedUploadTypes` is what its officers may upload.

One thing nsw-agency did for SLTB isn't here yet: a second round on the same
TNSW step. A review sent back to the trader (`needs_more_info` on the application
review or the levy receipt review) reaches TNSW, but the trader's resubmission
doesn't reach SLTB yet, and neither does a trader's counter-proposal to the pickup
date the officer proposed. See #600.

The YAML's `services` list registers `tnsw` for storage and the workflows'
`notify_decision` callbacks (`POST /api/v1/callbacks/{callbackToken}`).

## Run with Docker Compose

`make dev` and `make preview` start SLTB with the rest of the stack. It adds:

| Service       | What it does                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------- |
| `sltb-init`   | One-shot: creates `sltb_db` and the `sltb` Temporal namespace if missing, then migrates `sltb_db` |
| `sltb-api`    | The backend in agency mode, on <http://localhost:8086>                                            |
| `sltb-portal` | The officer portal, on <http://localhost:5179>                                                    |

`sltb-api` and `sltb-portal` run the built images even under `make dev` (no hot
reload); rebuild them after a change with
`docker compose up -d --build sltb-api sltb-portal`.

TNSW in compose reaches it at `host.docker.internal:8086`, through the `sltb` entry
its `config.docker.yaml` services list already has. To change a setting, edit `config.docker.yaml`
and restart `sltb-api`. `sltb-api` loads its artifacts from a `one-trade-artifacts`
clone next to this repo (`../one-trade-artifacts/sltb`); set
`SLTB_ARTIFACT_LOCAL_ROOT` in `.env` to use another clone's `sltb` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid temporal`):

```sh
# Once: the SLTB database and Temporal namespace
docker compose run --rm sltb-init

set -a; . configs/agency/sltb/sltb.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/config.yaml` services entry `sltb`
(`http://localhost:8086`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/sltb`); change `artifactLoader.local.root` in
`config.yaml` to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/bootstrap/20-agency-sltb.yaml`](../../../idp/bootstrap/20-agency-sltb.yaml)
creates SLTB for this deployment, on the `SLTB API` resource server
(`https://api.sltb.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                             | Client                | Grant                                                                                                                                 |
| ---------------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Officer `sltb_officer`             | `OGA_PORTAL_APP_SLTB` | `SLTB Officer` role (via `SLTB Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*`                  |
| TNSW injecting and replying        | `NSW_TO_SLTB`         | `NswToSltbM2M` role: `nsw:workflow:inject`, and `nsw:task:write` to reply on SLTB's parked steps, which its catalog maps `tnsw` to    |
| SLTB decision callback and storage | `SLTB_TO_NSW`         | `SltbToNswM2M` role: `nsw:task:write` and `nsw:storage:read`/`write`/`internalwrite` on TNSW                                          |

TNSW's `config.yaml` services entry `sltb` requests `nsw:workflow:inject` and `nsw:task:write` with
`resource=https://api.sltb.nsw-agency.local` (see `configs/config*.example.yaml`).
An IdP seeded before the move to `idp/bootstrap/` holds the same entities under other
IDs, so `thunderid-setup` fails on it. Recreate the IdP volumes once (`docker compose
down` and remove the `thunderid-*` volumes; see
[idp/README.md](../../../idp/README.md#changing-resources)).
