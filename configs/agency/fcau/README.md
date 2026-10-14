# FCAU agency

Configuration to run this backend as the Food Control Administration Unit (FCAU),
next to TNSW on the same machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                        |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8082, own database, Temporal namespace, artifacts, authn                                                                  |
| `config.docker.yaml`   | `config.yaml` for the `fcau-init` and `fcau-api` containers: Postgres, Temporal and the IdP by container hostname                                                              |
| `fcau.env`             | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` and `services.json` reference                                                             |
| `catalog.json`         | `officer` → the `FCAU Officer` role FCAU officers hold in the IdP                                                                                                              |
| `services.json`        | `tnsw`: where the workflows' `notify_decision` step posts the decision back to TNSW (`POST /api/v1/callbacks/{callbackToken}`), and where storage is proxied, as `FCAU_TO_NSW` |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                          |
| `services.docker.json` | `services.json` for the `fcau-api` container: TNSW at `api:8080`, the IdP at `thunderid:8090`                                                                                  |
| `portal-config.js`     | Runtime config of the `fcau-portal` container: the trader-app in agency mode                                                                                                   |
| `branding.json`        | Branding for the `fcau-portal` container, as the FCAU officer portal had it in nsw-agency                                                                                      |

FCAU keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
TNSW as `FCAU_TO_NSW`. TNSW applies its own upload limits.

One thing nsw-agency did for FCAU isn't here yet: a second round on the same
TNSW step. A review sent back to the trader (`needs_more_info` on the application
review or on either lab fee receipt review) reaches TNSW, but the trader's
resubmission doesn't reach FCAU yet, and neither does the trader's new proposal
after the officer reschedules the warehouse inspection. See #600.

The task configs nsw-agency had for `fcau_sample_assessment_v1`,
`fcau_sample_decision_v1` and `fcau_sample_wait_v1` aren't carried over: no TNSW
workflow sends those tasks.

## Run with Docker Compose

`make dev` and `make preview` start FCAU with the rest of the stack. It adds:

| Service       | What it does                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------- |
| `fcau-init`   | One-shot: creates `fcau_db` and the `fcau` Temporal namespace if missing, then migrates `fcau_db` |
| `fcau-api`    | The backend in agency mode, on <http://localhost:8082>                                            |
| `fcau-portal` | The officer portal, on <http://localhost:5175>                                                    |

`fcau-api` and `fcau-portal` run the built images even under `make dev` (no hot
reload); rebuild them after a change with
`docker compose up -d --build fcau-api fcau-portal`.

TNSW in compose reaches it at `host.docker.internal:8082`, through the `fcau` entry
its `services.docker.json` already has. To change a setting, edit `config.docker.yaml`
and restart `fcau-api`. `fcau-api` loads its artifacts from a `one-trade-artifacts`
clone next to this repo (`../one-trade-artifacts/fcau-v2`); set
`FCAU_ARTIFACT_LOCAL_ROOT` in `.env` to use another clone's `fcau-v2` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid thunderid-seed temporal`):

```sh
# Once: the FCAU database and Temporal namespace
docker compose run --rm fcau-init

set -a; . configs/agency/fcau/fcau.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/services.json` entry `fcau`
(`http://localhost:8082`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/fcau-v2`); change `artifactLoader.local.root` in
`config.yaml` to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/resources/government/fcau.json`](../../../idp/resources/government/fcau.json)
seeds FCAU for this deployment, on the `FCAU API` resource server
(`https://api.fcau.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                             | Client                | Grant                                                                                                                |
| ---------------------------------- | --------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Officer `fcau_officer`             | `OGA_PORTAL_APP_FCAU` | `FCAU Officer` role (via `FCAU Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*` |
| TNSW injecting                     | `NSW_TO_FCAU`         | `NswToFcauM2M` role: `nsw:workflow:inject`                                                                           |
| FCAU decision callback and storage | `FCAU_TO_NSW`         | `AgencyM2M` role: `nsw:task:write` and `nsw:storage:read`/`write` on TNSW                                            |

TNSW's `services.json` `fcau` entry requests `nsw:workflow:inject` with
`resource=https://api.fcau.nsw-agency.local` (see `configs/services*.example.json`).
An IdP seeded before this change still has the old FCAU entities, and a re-seed skips
anything that exists: `OGA_PORTAL_APP_FCAU` keeps its `agency:*` scopes,
`NSW_TO_FCAU` keeps `NswM2M`, and `fcau_officer` stays in `OGA Reviewers`. Reset it
once, by running `idp/sample-resources.down.sh` then the seed, or by recreating the IdP
volumes (`docker compose down` and removing the `thunderid-*` volumes).
