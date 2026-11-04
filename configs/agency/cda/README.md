# CDA agency

Configuration to run this backend as the Coconut Development Authority, next to
TNSW on the same machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                 | Purpose                                                                                                                                                         |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`        | Server config for a native run: `mode: agency`, port 8083, own database, Temporal namespace, artifacts, authn, outbound services, reference ID formats, catalog |
| `config.docker.yaml` | `config.yaml` for the `cda-init` and `cda-api` containers: Postgres, Temporal and the IdP by container hostname                                                 |
| `cda.env`            | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` references                                                                 |
| `portal-config.js`   | Runtime config of the `cda-portal` container: the trader-app in agency mode                                                                                     |
| `branding.json`      | CDA branding for the `cda-portal` container                                                                                                                     |

CDA keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
TNSW as `CDA_TO_NSW`. Officers' uploads go to TNSW's upload route (`storage:write`). CDA generates no files itself, so its client doesn't hold `storage:internalwrite`, the scope of TNSW's internal upload route. `storage.proxy.allowedUploadTypes` is what its officers may upload.

The YAML's `services` list registers `tnsw` for storage and the workflows'
`notify_decision` callbacks (`POST /api/v1/callbacks/{callbackToken}`).

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
`config.docker.yaml` services list already has. To change a setting, edit `config.docker.yaml` and
restart `cda-api`. `cda-api` loads its artifacts from a `one-trade-artifacts` clone
next to this repo (`../one-trade-artifacts/cda`); set `CDA_ARTIFACT_LOCAL_ROOT` in
`.env` to use another clone's `cda` dir.

## Run natively

With the shared stack up (`docker compose up db thunderid temporal`):

```sh
# Once: CDA's own database and Temporal namespace
docker compose run --rm cda-init

set -a; . configs/agency/cda/cda.env; set +a
go run ./cmd/server
```

TNSW reaches it through its own `configs/config.yaml` services entry `cda`
(`http://localhost:8083`).

It loads its artifacts from a `one-trade-artifacts` clone next to this repo
(`../one-trade-artifacts/cda`); change `artifactLoader.local.root` in `config.yaml`
to use another clone.

For the officer UI, run the trader-app with the values in `portal-config.js`.

## IdP

[`idp/bootstrap/20-agency-cda.yaml`](../../../idp/bootstrap/20-agency-cda.yaml) creates
CDA for this deployment, on the `CDA API` resource server
(`https://api.cda.nsw-agency.local`, `authn.audience` in `config.yaml`):

| Caller                              | Client               | Grant                                                                                                                    |
| ----------------------------------- | -------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Officer `cda_officer`               | `OGA_PORTAL_APP_CDA` | `CDA Officer` role (via `CDA Officers`): `consignment:read`, `task:*`, `profile:read`, `storage:*`                       |
| Admin `cda_admin`                   | `OGA_PORTAL_APP_CDA` | `CDA Admin` role (via `CDA Admins`): `consignment:adminread`/`adminwrite`, `profile:read`                                |
| TNSW injecting and replying         | `NSW_TO_CDA`         | `NswToCdaM2M` role: `workflow:inject`, and `task:write` to reply on CDA's parked steps, which its catalog maps `tnsw` to |
| CDA's decision callback and storage | `CDA_TO_NSW`         | `AGENCY_TO_TNSW` role: `task:write` on TNSW, which maps `cda` to it in its catalog, and `storage:read`/`write`           |

`CDA_TO_NSW` and the roles that grant it the NSW API are in TNSW's OU, in
[`idp/bootstrap/15-tnsw-agency-clients.yaml`](../../../idp/bootstrap/15-tnsw-agency-clients.yaml).

TNSW's `config.yaml` services entry `cda` requests `workflow:inject` and `task:write` with
`resource=https://api.cda.nsw-agency.local` (see `configs/config*.example.yaml`).
An IdP seeded before the move to `idp/bootstrap/` holds the same entities under other
IDs, so `thunderid-setup` fails on it. Recreate the IdP volumes once (`docker compose
down` and remove the `thunderid-*` volumes; see
[idp/README.md](../../../idp/README.md#changing-resources)).
