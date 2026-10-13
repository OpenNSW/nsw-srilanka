# CDA agency

Configuration to run this backend as the Coconut Development Authority, next to
TNSW on the same machine. How agency mode works is in [docs/agency.md](../../../docs/agency.md).
Values marked ASSUMED in `config.yaml` are placeholders to replace.

| File                   | Purpose                                                                                                                                                                       |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.yaml`          | Server config for a native run: `mode: agency`, port 8083, own database, Temporal namespace, artifacts, authn, reference ID formats                                           |
| `config.docker.yaml`   | `config.yaml` for the `cda-init` and `cda-api` containers: Postgres, Temporal and the IdP by container hostname                                                               |
| `cda.env`              | Environment for a native run: `CONFIG_PATH`, `APP_ENV` and the secrets `config.yaml` and `services.json` reference                                                            |
| `catalog.json`         | `officer` → `CDA Officer`, plus a temporary `reviewer` → `CDA Reviewer` (see Per-task roles test)                                                                             |
| `services.json`        | `tnsw`: where the workflows' `notify_decision` step posts the decision back to TNSW (`POST /api/v1/callbacks/{callbackToken}`), and where storage is proxied, as `CDA_TO_NSW` |
| `payment_methods.json` | Empty: an agency takes no payments, but the task stack loads the file                                                                                                         |
| `services.docker.json` | `services.json` for the `cda-api` container: TNSW at `api:8080`, the IdP at `thunderid:8090`                                                                                  |
| `portal-config.js`     | Runtime config of the `cda-portal` container: the trader-app in agency mode                                                                                                   |
| `branding.json`        | CDA branding for the `cda-portal` container                                                                                                                                   |

CDA keeps no files of its own. Its `storage` section is `type: proxy` to the `tnsw`
service, so officers open and upload the documents in TNSW's storage, calling
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

| Caller                              | Client               | Grant                                                                                                              |
| ----------------------------------- | -------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Officer `cda_officer`               | `OGA_PORTAL_APP_CDA` | `CDA Officer` role (via `CDA Officers`): `nsw:consignment:read`, `nsw:task:*`, `nsw:profile:read`, `nsw:storage:*` |
| Temporary reviewer `cda_reviewer`   | `OGA_PORTAL_APP_CDA` | `CDA Officer` and `CDA Reviewer` roles (via `CDA Officers` and `CDA Reviewers`)                                    |
| TNSW injecting                      | `NSW_TO_CDA`         | `NswToCdaM2M` role: `nsw:workflow:inject`                                                                          |
| CDA's decision callback and storage | `CDA_TO_NSW`         | `AgencyM2M` role: `nsw:task:write` on TNSW, which maps `cda` to it in its catalog, and `nsw:storage:read`/`write`  |

TNSW's `services.json` `cda` entry requests `nsw:workflow:inject` with
`resource=https://api.cda.nsw-agency.local` (see `configs/services*.example.json`).
An IdP seeded before this change still has the old CDA entities, and a re-seed skips
anything that exists: `OGA_PORTAL_APP_CDA` keeps its `agency:*` scopes, `NSW_TO_CDA`
keeps `NswM2M`, and `cda_officer` stays in `OGA Reviewers`. Reset it once, by running
`idp/sample-resources.down.sh` then the seed, or by recreating the IdP volumes
(`docker compose down` and removing the `thunderid-*` volumes).

## Per-task roles test (temporary)

Test setup for per-task roles (#602), removed when RBAC lands. It restricts every task of
CDA flow 1 (application verification, ASYCUDA green check, review result, lot adjustment
review) to a `reviewer` role, for reading and writing, so an officer is refused anywhere
in a case and a reviewer is admitted. It lives on two branches, neither meant to be merged:

| Piece                                                                                    | Where                                                                       |
| ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| This repo: everything below except the artifacts                                         | branch `test/602-cda-reviewer-fixtures` (the feature plus these fixtures)   |
| `reviewer` → `CDA Reviewer`                                                              | `catalog.json`                                                              |
| `CDA Reviewer` role, `CDA Reviewers` group, user `cda_reviewer` (also in `CDA Officers`) | `idp/resources/government/cda.json`                                         |
| `IDP_CHA_ROLE_NAME: 'CDA Reviewer'`                                                      | `portal-config.js`: the portal admits a user holding only the reviewer role |
| `authz` and `read.roles` on the four flow-1 tasks                                        | `one-trade-artifacts`, branch `test/cda-reviewer-authz` (`cda-v2`)          |

1. Check out `test/602-cda-reviewer-fixtures` here and `test/cda-reviewer-authz` in the
   `one-trade-artifacts` clone next to this repo (or point `CDA_ARTIFACT_LOCAL_ROOT` at
   a clone that has it).
2. Seed the new IdP entities: `docker compose run --rm thunderid-seed`. A re-seed
   creates what is missing and skips the rest, so the existing users are unchanged. Set
   `SAMPLE_CDA_REVIEWER_PASSWORD` in `idp/.env` to give `cda_reviewer` its own password;
   otherwise it uses `SAMPLE_USER_PASSWORD`.
3. Rebuild and restart CDA so it picks up the catalog, the portal config and the backend
   change: `docker compose up -d --build cda-api cda-portal`.
4. Start a **new** CDA consignment from TNSW. The first CDA task is the application
   verification. A task's `read` rule is copied into it when it starts, so only new
   cases get it; the write rule applies to every case at once.
5. In the CDA portal (<http://localhost:5176>):
   - `cda_officer`: the case list and detail load, but opening a task is refused (404)
     and submitting it is refused (403).
   - `cda_reviewer`: opens and completes each task; the case moves on.

If a submit still goes through, the task it hit has no rule. `docker logs cda-api`
names the template of each step written.
