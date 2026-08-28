# Trade National Single Window (TNSW) Helm Chart

Helm chart for Sri Lanka's Trade National Single Window platform: the
**Backend API**
(`cmd/server`) and the **Trader Portal** frontend
([`portals/apps/trader-app`](../../../portals/apps/trader-app)), deployed
together as one release with `backend`/`frontend` sections in values. Infra
this stack depends on (Postgres, Temporal, Thunder ID, Argus) is deployed
separately — component env just points at their in-cluster Service names or
external URLs.

**Start from [`../values-example.yaml`](../values-example.yaml)** — a
complete, ready-to-edit override file covering every env var each component
reads, which secrets to create, and route/ingress setup. Don't hand-assemble
your own values from `values.yaml` + the templates; the example file already
did that reverse-engineering for you.

## Files Included

Templates are grouped by component under `templates/backend/` and `templates/frontend/` (Helm renders `templates/` recursively, so subdirectories are purely organizational):

- **[backend/deployment.yaml](templates/backend/deployment.yaml)** / **[frontend/deployment.yaml](templates/frontend/deployment.yaml)**: Deployment, container, ports, environment variables, mounts, and probes for each component.
- **[backend/service.yaml](templates/backend/service.yaml)** / **[frontend/service.yaml](templates/frontend/service.yaml)**: Exposes each component's container port as a cluster-internal Service.
- **[backend/migration-job.yaml](templates/backend/migration-job.yaml)**: Runs schema migrations as a pre-install/pre-upgrade hook (off by default). No frontend equivalent — the portal has no database.
- **[backend/route.yaml](templates/backend/route.yaml)** / **[frontend/route.yaml](templates/frontend/route.yaml)**: Exposes each component externally via an OpenShift Route (when `<component>.route.enabled`).
- **[backend/ingress.yaml](templates/backend/ingress.yaml)** / **[frontend/ingress.yaml](templates/frontend/ingress.yaml)**: Exposes each component externally via a Kubernetes Ingress (when `<component>.ingress.enabled`).

## Layout

```text
deployments/helm/
├── values-example.yaml # complete example override; not bundled in chart packages
└── lk-tnsw/             # this chart (templates + neutral defaults)
```

The example override lives one level up, outside the chart directory, so
`helm package lk-tnsw` doesn't bundle it into the published chart artifact.

## Usage

```bash
helm install lk-tnsw ./lk-tnsw -f ../values-example.yaml
```

`values.yaml` holds only neutral defaults, split into `backend:` and
`frontend:` sections. Copy the example file and fill in your environment's
URLs and secrets. Note that **both `backend.image.tag` and
`frontend.image.tag` are required** (there is no default for either); the
example file sets both, or pass `--set backend.image.tag=1.4.0 --set
frontend.image.tag=1.4.0`.

### Three images, one chart

The root [`Dockerfile`](../../../Dockerfile) has two build targets and
[`portals/apps/trader-app/Dockerfile`](../../../portals/apps/trader-app/Dockerfile)
is a third — all three are published as separate GHCR images by the same
[`release.yml`](../../../.github/workflows/release.yml) run (same git tag →
same version for all three):

| Image                          | Built from                                    | Deployed by                  |
|--------------------------------|-----------------------------------------------|------------------------------|
| `ghcr.io/opennsw/tnsw-api`  | root `Dockerfile`, `runtime` (default) target | `backend/deployment.yaml`    |
| `ghcr.io/opennsw/tnsw-migrate`  | root `Dockerfile`, `migrate` target           | `backend/migration-job.yaml` |
| `ghcr.io/opennsw/tnsw-web` | `portals/apps/trader-app/Dockerfile`          | `frontend/deployment.yaml`   |

All three are published as multi-arch manifest lists covering `linux/amd64` and
`linux/arm64`, so one tag scheduled onto a mixed-arch cluster resolves to the
right image per node — no `nodeSelector` on `kubernetes.io/arch` is needed.

The migration Job (`backend.migration.enabled: true`) uses a **different
image** from the backend Deployment — see `backend.migration.image` in
`values.yaml`. It also uses **different DB env var names** than the backend
(`DB_USER`, not `DB_USERNAME`) because it runs the external nsw-agency
migrator's own binary, not this backend's code. `backend.migration.image.tag`
defaults to `backend.image.tag` when left unset.

### Prerequisite: secrets

Secrets are **not** stored in the values files — they are referenced from a
Kubernetes Secret that you create out-of-band before installing:

```bash
kubectl create secret generic nsw-secrets \
  --from-literal=db-password=... \
  --from-literal=m2m-npqs-secret=... \
  --from-literal=m2m-fcau-secret=... \
  --from-literal=m2m-cda-secret=... \
  --from-literal=m2m-slpa-secret=... \
  --from-literal=m2m-customs-secret=... \
  --from-literal=m2m-sltb-secret=... \
  --from-literal=m2m-asycuda-secret=... \
  --from-literal=argus-api-key=...
```

See [`.env.example`](../../../.env.example) for what each of these secrets
backs and the full set of non-secret config the backend reads. The frontend
needs no secrets — its `env` is all public SPA config (see below).

### Frontend runtime config, not secrets

`frontend.env` holds no secrets. The values are `VITE_*` config written into
`runtime-env.js` at container start (see
[`apps/trader-app/docker-entrypoint.sh`](../../../portals/apps/trader-app/docker-entrypoint.sh))
and read directly by the browser — so every URL must be the one the browser
will actually hit (e.g. the public backend host), not an in-cluster Service
name.

### Health checks

Both components default their `livenessProbe`/`readinessProbe` to `GET
/health`:
- Backend: see `internal/bootstrap/app.go` — returns 503 while the database
  or authn dependency is unreachable.
- Frontend: answered directly by
  [`nginx.conf`](../../../portals/apps/trader-app/nginx.conf) with `200 OK` —
  it does not depend on the backend being reachable.

## Configuration Reference

See [values.yaml](values.yaml) for the full list of configurable options, and
[../values-example.yaml](../values-example.yaml) for a complete,
ready-to-edit example.
