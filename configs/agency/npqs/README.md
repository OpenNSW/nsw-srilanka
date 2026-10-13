# NPQS agency

Configuration to run this backend as the National Plant Quarantine Service (NPQS),
next to TNSW on the same machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                        |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8081, own database, Temporal namespace, artifacts, authn                                                                  |
| `config.docker.yaml`   | `config.yaml` for the `npqs-init` and `npqs-api` containers: Postgres, Temporal and the IdP by container hostname                                                              |
| `npqs.env`             | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` and `services.json` reference                                                             |
| `catalog.json`         | `officer` → the `NPQS Officer` role NPQS officers hold in the IdP                                                                                                              |
| `services.json`        | `tnsw`: where the workflows' `notify_decision` step posts the decision back to TNSW (`POST /api/v1/callbacks/{callbackToken}`), and where storage is proxied, as `NPQS_TO_NSW` |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                          |
| `services.docker.json` | `services.json` for the `npqs-api` container: TNSW at `api:8080`, the IdP at `thunderid:8090`                                                                                  |
| `portal-config.js`     | Runtime config of the `npqs-portal` container: the trader-app in agency mode                                                                                                   |
| `branding.json`        | Branding for the `npqs-portal` container, as the NPQS officer portal had it in nsw-agency                                                                                      |

NPQS keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
TNSW as `NPQS_TO_NSW`. TNSW applies its own upload limits.

The officer forms' searchable lists (importing countries, commodity names) are
served from the `static_data` in `npqs-v2`, the same lists TNSW serves the trader.

Two things nsw-agency did for NPQS aren't here yet:

- Every NPQS officer sees every office's cases. nsw-agency limited officers to their
  own office (`nppo_office_location`); see #601.
- A review sent back to the trader (`needs_more_info` on the application or
  treatment request review) reaches TNSW, but the trader's resubmission doesn't
  reach NPQS yet; see #600.

## Run with Docker Compose

`make dev` and `make preview` start NPQS with the rest of the stack. It adds:

| Service       | What it does                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------- |
| `npqs-init`   | One-shot: creates `npqs_db` and the `npqs` Temporal namespace if missing, then migrates `npqs_db` |
| `npqs-api`    | The backend in agency mode, on <http://localhost:8081>                                            |
| `npqs-portal` | The officer portal, on <http://localhost:5174>                                                    |

`npqs-api` and `npqs-portal` run the built images even under `make dev` (no hot
reload); rebuild them after a change with
`docker compose up -d --build npqs-api npqs-portal`.

TNSW in compose reaches it at `host.docker.internal:8081`, through the `npqs` entry
its `services.docker.json` already has. To change a setting, edit `config.docker.yaml`
and restart `npqs-api`. `npqs-api` loads its artifacts from a `one-trade-artifacts`
clone next to this repo (`../one-trade-artifacts/npqs-v2`); set
`NPQS_ARTIFACT_LOCAL_ROOT` in `.env` to use another clone's `npqs-v2` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid thunderid-seed temporal`):

```sh
# Once: the NPQS database and Temporal namespace
docker compose run --rm npqs-init

set -a; . configs/agency/npqs/npqs.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/services.json` entry `npqs`
(`http://localhost:8081`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/npqs-v2`); change `artifactLoader.local.root` in
`config.yaml` to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/resources/government/npqs.json`](../../../idp/resources/government/npqs.json)
seeds NPQS for this deployment, on the `NPQS API` resource server
(`https://api.npqs.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                             | Client                | Grant                                                                                                                |
| ---------------------------------- | --------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Officer `npqs_officer`             | `OGA_PORTAL_APP_NPQS` | `NPQS Officer` role (via `NPQS Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*` |
| TNSW injecting                     | `NSW_TO_NPQS`         | `NswToNpqsM2M` role: `nsw:workflow:inject`                                                                           |
| NPQS decision callback and storage | `NPQS_TO_NSW`         | `AgencyM2M` role: `nsw:task:write` and `nsw:storage:read`/`write` on TNSW                                            |

TNSW's `services.json` `npqs` entry requests `nsw:workflow:inject` with
`resource=https://api.npqs.nsw-agency.local` (see `configs/services*.example.json`).
An IdP seeded before this change still has the old NPQS entities, and a re-seed skips
anything that exists: `OGA_PORTAL_APP_NPQS` keeps its `agency:*` scopes,
`NSW_TO_NPQS` keeps `NswM2M`, and `npqs_officer` stays in `OGA Reviewers`. Reset it
once, by running `idp/sample-resources.down.sh` then the seed, or by recreating the IdP
volumes (`docker compose down` and removing the `thunderid-*` volumes).
