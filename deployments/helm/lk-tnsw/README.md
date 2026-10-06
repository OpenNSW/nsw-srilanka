# Trade National Single Window (TNSW) Helm Chart

Helm chart for Sri Lanka's Trade National Single Window platform: the
**Backend API**
(`cmd/server`) and the **Trader Portal** frontend
([`portals/apps/trader-app`](../../../portals/apps/trader-app)), deployed
together as one release with `backend`/`frontend` sections in values. Infra
this stack depends on (Postgres, Temporal, Thunder ID, Argus) is deployed
separately — the backend's config just points at their in-cluster Service
names or external URLs.

**Start from [`../values-example.yaml`](../values-example.yaml)** — a
complete, ready-to-edit override file covering every config value each
component reads, which secrets to create, and route/ingress setup. Don't
hand-assemble
your own values from `values.yaml` + the templates; the example file already
did that reverse-engineering for you.

## Files Included

Templates are grouped by component under `templates/backend/`, `templates/migration/` and `templates/frontend/` (Helm renders `templates/` recursively, so subdirectories are purely organizational):

- **[backend/deployment.yaml](templates/backend/deployment.yaml)**: Deployment, container, ports, environment variables, mounts, and probes for the backend.
- **[backend/configmap.yaml](templates/backend/configmap.yaml)**: Renders `backend.config` into the server's `config.yaml`, mounted into the backend container (see "Backend config file" below).
- **[frontend/deployment.yaml](templates/frontend/deployment.yaml)**: Deployment, container, ports, mounts, and probes for the frontend — its runtime config comes from the mounted ConfigMap below, not container env vars.
- **[frontend/configmap.yaml](templates/frontend/configmap.yaml)**: Renders `frontend.config` into `config.js`, mounted into the frontend container for the browser to read (see "Frontend runtime config, not secrets" below).
- **[backend/service.yaml](templates/backend/service.yaml)** / **[frontend/service.yaml](templates/frontend/service.yaml)**: Exposes each component's container port as a cluster-internal Service.
- **[migration/job.yaml](templates/migration/job.yaml)** / **[migration/configmap.yaml](templates/migration/configmap.yaml)**: Runs schema migrations as a pre-install/pre-upgrade hook (when `migration.enabled`), with the migrator's own `config.yaml` rendered from `migration.config` (see "Migration Job" below).
- **[backend/route.yaml](templates/backend/route.yaml)** / **[frontend/route.yaml](templates/frontend/route.yaml)**: Exposes each component externally via an OpenShift Route (when `<component>.route.enabled`).
- **[backend/ingress.yaml](templates/backend/ingress.yaml)** / **[frontend/ingress.yaml](templates/frontend/ingress.yaml)**: Exposes each component externally via a Kubernetes Ingress (when `<component>.ingress.enabled`).
- **[frontend/branding-configmap.yaml](templates/frontend/branding-configmap.yaml)**: Renders `frontend.branding` into a ConfigMap and mounts it over the image's baked-in `branding.json` (when `frontend.branding` is set).

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
helm install lk-tnsw oci://ghcr.io/opennsw/charts/lk-tnsw --version 0.1.0 -f values.yaml
```

The chart is released with the app, at the same version: chart `0.1.0` has
`appVersion: 0.1.0` and deploys the `0.1.0` images unless you set
`backend.image.tag` or `frontend.image.tag`. See the
[GitHub Releases](https://github.com/OpenNSW/nsw-srilanka/releases) for the
versions.

`values.yaml` holds only neutral defaults, split into `backend:`,
`migration:` and `frontend:` sections. Copy [`values-example.yaml`](../values-example.yaml)
and fill in your environment's URLs and secrets.

To install from this directory instead — to test chart changes — set both
image tags: the chart's `Chart.yaml` only holds `0.0.0` placeholders, which the
templates refuse.

```bash
helm install lk-tnsw ./lk-tnsw -f ../values-example.yaml \
  --set backend.image.tag=0.1.0 --set frontend.image.tag=0.1.0
```

### Three images, one chart

The root [`Dockerfile`](../../../Dockerfile) has two build targets and
[`portals/apps/trader-app/Dockerfile`](../../../portals/apps/trader-app/Dockerfile)
is a third — all three are published as separate GHCR images by the same
[`release.yml`](../../../.github/workflows/release.yml) run (same git tag →
same version for all three):

| Image                          | Built from                                    | Deployed by                |
| ------------------------------ | --------------------------------------------- | -------------------------- |
| `ghcr.io/opennsw/tnsw-api`     | root `Dockerfile`, `runtime` (default) target | `backend/deployment.yaml`  |
| `ghcr.io/opennsw/tnsw-migrate` | root `Dockerfile`, `migrate` target           | `migration/job.yaml`       |
| `ghcr.io/opennsw/tnsw-web`     | `portals/apps/trader-app/Dockerfile`          | `frontend/deployment.yaml` |

All three are published as multi-arch manifest lists covering `linux/amd64` and
`linux/arm64`, so one tag scheduled onto a mixed-arch cluster resolves to the
right image per node — no `nodeSelector` on `kubernetes.io/arch` is needed.

`migration.image.tag` defaults to `backend.image.tag`, then to the chart's
`appVersion`.

### Migration Job

The migration Job (`migration.enabled: true`) is a component of its own. It
runs the external OpenNSW/agency migrator's binary, not this backend's code,
and shares no configuration with the backend:

- `migration.config` is the migrator's `config.yaml`. It reads only `db` (the
  same schema as `backend.config.db`) and, optionally, `migrationDir`, which
  defaults to the SQL baked into the image. `db.driver` must be `postgres` —
  the migrator defaults it to SQLite, so the chart refuses to render without
  it.
- `migration.env` / `migration.envFrom` carry only the secrets that file
  references, so the Job is never handed the backend's other secrets. Nothing
  is inherited from `backend.env` / `backend.envFrom`.
- Its image, pull secrets, security contexts and resources are set under
  `migration` too.

The file is rendered into a hook ConfigMap of its own, since on a first install
the Job runs before the release's regular resources exist.

#### Database accounts

[`values-example.yaml`](../values-example.yaml) uses one account for both: the
same user in `migration.config` as in `backend.config`, and `migration.env`
pointing at the same `db-password` Secret key as `backend.env`. That needs no
extra database setup.

For production, prefer a separate account. Because the configs are separate,
the migrator can connect as its own account — one that owns the schema — while
the server's account only reads and writes rows, so a compromised server
cannot alter or drop the schema. Give the migrator its own Secret key and point
`migration.env` at it. Tables the
migrator creates are owned by it, so grant the server's account access to them
once, as the migrator's account (shown here as `nsw_migrator` and `nsw_app`):

```sql
GRANT USAGE ON SCHEMA public TO nsw_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nsw_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO nsw_app;
```

Run the default-privilege grants before the first migration; for a database
that already has tables, also grant on `ALL TABLES` / `ALL SEQUENCES IN SCHEMA
public`.

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
  --from-literal=argus-api-key=... \
  --from-literal=notification-email-token=... \
  --from-literal=notification-sms-password=...
```

The GovPay+ gateway also needs this GO's RSA private key. GovPay+ encrypts
every call to the matching public key, so it must be given that public key.
The private key is a file rather than an env var, so it goes in a Secret of
its own:

```bash
kubectl create secret generic govpay-go-private-key \
  --from-file=go_private.pem=./go_private.pem
```

[`values-example.yaml`](../values-example.yaml) mounts it at `/certs/govpay`
through `backend.volumes` / `backend.volumeMounts`, which is where the govpay
entry in `payment_methods.json` points (`"private_key":
"file:/certs/govpay/go_private.pem"`). The key must be a `file:` (or `env:`)
reference; an inline PEM is refused. Without it the backend starts but answers
every GovPay+ call with 500; a key that is set but unreadable stops the backend
at startup.

`backend.env` exposes each `nsw-secrets` key to the server as an env var, and
`backend.config` references it by placeholder (see below). See
[`configs/config.example.yaml`](../../../configs/config.example.yaml) for
every setting the backend reads. The frontend
needs no secrets — its `config` is all public SPA config (see below).

### Backend config file

The server refuses to start without its `config.yaml`, so the chart always
provides one. `backend.config` holds the file's content as values (the schema
is [`configs/config.example.yaml`](../../../configs/config.example.yaml)). The
chart renders it into a ConfigMap and mounts it read-only at
`backend.configMountPath` (`/app/config`), then points `CONFIG_PATH` there.
Every server setting lives here — the backend reads no other env vars besides
`APP_ENV` and the secrets the file references. There are no built-in
defaults: every setting the server needs must be set, or it refuses to start
(see [`values-example.yaml`](../values-example.yaml) for a complete one).
`backend.config` is empty by default, so an override file always sets it.

- To use a file of your own instead, set `backend.env.CONFIG_PATH`. The chart
  then leaves `CONFIG_PATH` alone.
- No secrets go in `backend.config`. Write a placeholder instead, such as
  `"{{env:NAME}}"` (with `NAME` set through `backend.env`) or
  `"{{file:/path}}"`. Helm passes it through, and the server resolves it at
  startup.
- Changing `backend.config` rolls the pods.

### Frontend runtime config, not secrets

`frontend.config` holds no secrets. Unlike `backend.env`, it never becomes a
container environment variable — the values are public SPA config rendered
into a ConfigMap and mounted at `/usr/share/nginx/html/config.js` (see
[`templates/frontend/configmap.yaml`](templates/frontend/configmap.yaml)),
read directly by the browser — so every URL must be the one the browser will
actually hit (e.g. the public backend host), not an in-cluster Service name.

### Frontend branding

`frontend.branding` is empty by default, in which case the portal serves the
neutral placeholder `branding.json` baked into the `tnsw-web` image at build
time — fine to run with, but not meant to reach real users (placeholder
footer links, generic copy). Set `frontend.branding` per deployment (see
`../values-example.yaml`) to override it: the chart renders it into a
ConfigMap and mounts it over
`/usr/share/nginx/html/configs/branding.json`, which the SPA fetches at
startup (`initAppConfig()` in
[`src/config.ts`](../../../portals/apps/trader-app/src/config.ts)). See
[`src/configs/types.ts`](../../../portals/apps/trader-app/src/configs/types.ts)
for the full schema.

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
