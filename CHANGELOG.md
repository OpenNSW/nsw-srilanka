# Changelog

All notable changes to TNSW, the Sri Lanka instance of the National Single Window platform, are recorded here for the people who deploy and run it. Each GitHub Release repeats its section from this file, then adds the image digests and the list of pull requests merged since the previous release.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Until 1.0.0, a minor release (0.x.0) may contain breaking changes; they are always listed under **Upgrade notes**. Release dates are the day of release in Sri Lanka time (UTC+05:30).

## [Unreleased]

## [0.2.0] - 2026-10-07

This release moves the settings of the API from environment variables into a `config.yaml` file, and the Trader Portal's settings into a mounted `config.js`. Neither image starts correctly with 0.1.0's settings, so change your deployment values, chart version and images in the same upgrade. It also adds agency mode, a reference ID generator for workflows, and end-to-end encryption for GovPay+ calls.

### Upgrade notes

#### API settings move to `config.yaml` ([#571](https://github.com/OpenNSW/nsw-srilanka/pull/571), [#549](https://github.com/OpenNSW/nsw-srilanka/pull/549))

- The API reads `config.yaml` from `CONFIG_PATH` (default `configs/config.yaml`) and does not start without it. Start from a complete example: [`configs/config.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.2.0/configs/config.example.yaml) for a native run, [`configs/config.docker.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.2.0/configs/config.docker.example.yaml) for Docker Compose, or `backend.config` in [`deployments/helm/values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.2.0/deployments/helm/values-example.yaml) for Helm.
- There are no built-in defaults. Every setting must be in the file, including the new `mode: tnsw`. Only `audit`, `refid`, and the settings of storage and artifact-loader backends you don't use may be left out.
- The API now reads only `CONFIG_PATH` and `APP_ENV` from the environment. Move every other 0.1.0 setting to its key:

  | 0.1.0 environment variable                                                   | `config.yaml` key                                                                                                               |
  | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
  | `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_NAME`, `DB_SSLMODE`                 | `db.postgres.host`, `port`, `user`, `name`, `sslMode`; also set `db.driver: postgres`                                           |
  | `DB_MAX_IDLE_CONNS`, `DB_MAX_OPEN_CONNS`, `DB_MAX_CONN_LIFETIME_SECONDS`     | `db.postgres.pool.maxIdleConns`, `maxOpenConns`, `maxConnLifetimeSeconds`                                                       |
  | `SERVER_*`, `SERVICE_URL`                                                    | `server.*`, `server.serviceURL`                                                                                                 |
  | `SERVICES_CONFIG_PATH`, `PAYMENT_METHODS_CONFIG_PATH`, `CATALOG_CONFIG_PATH` | `server.servicesConfigPath`, `paymentMethodsConfigPath`, `catalogConfigPath`                                                    |
  | `CORS_*`                                                                     | `cors.*`, with origins, methods and headers as YAML lists                                                                       |
  | `STORAGE_*`                                                                  | `storage.*`; `STORAGE_PRESIGN_TTL` (a duration, `15m`) becomes `storage.presignTTLSeconds` (`900`)                              |
  | `AUTH_*`                                                                     | `authn.*`; `AUTH_CLIENT_IDS` becomes the list `authn.clientIDs`, `AUTH_JWKS_INSECURE_SKIP_VERIFY` `authn.insecureSkipTLSVerify` |
  | `TEMPORAL_*`                                                                 | `temporal.*`                                                                                                                    |
  | `ARGUS_SERVICE_URL`                                                          | `audit.baseURL`                                                                                                                 |
  | `ARTIFACT_*`                                                                 | `artifactLoader.*`                                                                                                              |
  | `NOTIFICATIONS_CONFIG_PATH` and `notification.json`                          | `notification.providers`                                                                                                        |

- Keep secrets out of the file. Any value can be a placeholder resolved at startup, `"{{env:NAME}}"` or `"{{file:/path}}"`, and an unset one stops the API. The examples reference `DB_PASSWORD`, `STORAGE_LOCAL_PUT_SECRET`, `NOTIFICATION_EMAIL_TOKEN`, `NOTIFICATION_SMS_PASSWORD`, `ARGUS_API_KEY` and `SLPA_WEBHOOK_SECRET` this way, so `audit.apiKey` and `integrations.slpaWebhookSecret` take over from `ARGUS_API_KEY` and `SLPA_WEBHOOK_SECRET`.
- `storage.allowedUploadTypes` and `storage.maxUploadBytes` are required for the `local` and `s3` backends ([#578](https://github.com/OpenNSW/nsw-srilanka/pull/578)). Copy both from the example to keep 0.1.0's policy: PDF, JPEG, PNG, GIF, WebP and `.xlsx`, up to 32 MiB. Leave `storage.local.routePrefix` unset, or set it to `/api/v1/storage`.
- Every field of each notification provider must be set; an empty one stops the API at startup ([#582](https://github.com/OpenNSW/nsw-srilanka/pull/582)).
- The 0.1.0 production checklist still applies under the new names: `server.logLevel: info`, `db.postgres.sslMode: require`, a pinned `artifactLoader.github.ref`, every calling client in `authn.clientIDs`, and an `audit` section with `baseURL` and `apiKey`.

#### Helm values ([#571](https://github.com/OpenNSW/nsw-srilanka/pull/571), [#577](https://github.com/OpenNSW/nsw-srilanka/pull/577), [#463](https://github.com/OpenNSW/nsw-srilanka/pull/463))

- Upgrade an existing deployment to chart 0.2.0 in the same change as the values below. Chart 0.2.0 deploys the 0.2.0 images, which do not start with 0.1.0's values, and the 0.1.0 chart does not read the new ones.
- Move the API settings from `backend.env` to `backend.config`, which the chart mounts as `config.yaml`, setting `CONFIG_PATH` for you. Keep in `backend.env` only `APP_ENV` and the secrets `backend.config` references, as `valueFrom` entries. An existing `backend.env.CONFIG_PATH` is honoured.
- If `backend.migration.enabled` is set, move `backend.migration.*` to a top-level `migration:` and give it a config of its own:
  - `migration.config.db` with the migrator's connection, including `driver: postgres`. The chart refuses to render without it.
  - `migration.env` with the secrets that config references, such as `DB_PASSWORD` from the same Secret key as `backend.env`. The 0.1.0 `DB_*` entries under `backend.migration.env` are no longer read.
  - `migration.podSecurityContext`, `securityContext`, `imagePullSecrets` and `resources`, if the Job needs them. It no longer takes them from `backend`.
  - Optionally, a separate migrator account that owns the schema. The grants are under "Database accounts" in the chart README.
- Rename `frontend.env` to `frontend.config` and drop the `VITE_` prefix from every key in it, for example `VITE_API_BASE_URL` to `API_BASE_URL`.

#### Trader Portal settings ([#463](https://github.com/OpenNSW/nsw-srilanka/pull/463), [#543](https://github.com/OpenNSW/nsw-srilanka/pull/543))

- The `tnsw-web` image no longer reads container environment variables. Its settings come from `config.js`, mounted at `/usr/share/nginx/html/config.js`: Helm renders it from `frontend.config`; anywhere else, write one from [`config.example.js`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.2.0/portals/apps/trader-app/public/config.example.js) and mount it.
- Roles are read from an ID-token claim, not from groups. The old keys are ignored:

  | 0.1.0 key                       | 0.2.0 key                 | Default     |
  | ------------------------------- | ------------------------- | ----------- |
  | `VITE_IDP_TRADER_GROUP_NAME`    | `IDP_TRADER_ROLE_NAME`    | `Trader`    |
  | `VITE_IDP_CHA_GROUP_NAME`       | `IDP_CHA_ROLE_NAME`       | `CHA`       |
  | `VITE_IDP_NSW_ADMIN_GROUP_NAME` | `IDP_NSW_ADMIN_ROLE_NAME` | `NSW Admin` |

  The new `IDP_ROLE_CLAIM_NAME` (default `roles`) names the claim, which must be an array.
  - With ThunderID and its default role names, nothing more is needed as long as `IDP_SCOPES` includes `role`, as the defaults do.
  - If you set a `*_GROUP_NAME`, set the matching `*_ROLE_NAME` to the IdP's **role** name.
  - If your IdP emits only groups, set `IDP_ROLE_CLAIM_NAME` to `groups` and put the group names in the `*_ROLE_NAME` keys.

  Users with no matching role see the "Access Restricted" screen.

#### Migrations

- `000017_create_refid_tables` ([#549](https://github.com/OpenNSW/nsw-srilanka/pull/549)) and `000018_create_agency_workflow` ([#554](https://github.com/OpenNSW/nsw-srilanka/pull/554)) only create new tables, and both can be rolled back. Run them before the API starts; the Helm hook and Docker Compose do. `tnsw-migrate` now reads its connection from `config.yaml` too, so use the 0.2.0 image.

#### GovPay+ needs a private key ([#525](https://github.com/OpenNSW/nsw-srilanka/pull/525))

GovPay+ now encrypts every call to TNSW's public key. Without the matching private key the API still starts, but answers every GovPay+ call with 500, and GovPay+ keeps retrying.

1. Generate a key pair for each environment, and give GovPay+ the public key.
2. In `payment_methods.json`, add `"private_key": "file:/certs/govpay/go_private.pem"` to the `govpay` method's `config`, next to `webhook_client_id`. It must be a `file:` or `env:` reference; an inline PEM is refused at startup.
3. Mount the key at `/certs/govpay`. With Helm, create the `govpay-go-private-key` Secret (`--from-file=go_private.pem=...`) and mount it through `backend.volumes` and `backend.volumeMounts`, as `values-example.yaml` does. With Docker Compose, put it at `./certs/govpay/go_private.pem`.

A key that is configured but can't be read or parsed stops the API at startup.

#### SLPA client secret ([#589](https://github.com/OpenNSW/nsw-srilanka/pull/589))

- In your `services.json`, change the `slpa` entry's `client_secret` from `env:SLPA_CLIENT_SECRET` to `env:M2M_NSW_TO_SLPA_SECRET`, and put SLPA's CMS client secret in `M2M_NSW_TO_SLPA_SECRET`. Otherwise the API refuses to start, naming the unset variable. With Helm, `M2M_NSW_TO_SLPA_SECRET` already comes from the `m2m-slpa-secret` key, which must hold SLPA's CMS client secret.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to a commit SHA of one-trade-artifacts that includes [one-trade-artifacts#121](https://github.com/OpenNSW/one-trade-artifacts/pull/121), such as [`3134c1f`](https://github.com/OpenNSW/one-trade-artifacts/commit/3134c1f7f8ddf544f82da90bb012065e1133eb00) (7 October 2026). With older definitions, CusDec declarations go to Customs with an empty `totalCustomsValuation` and empty item `customsValue` ([#552](https://github.com/OpenNSW/nsw-srilanka/pull/552)).
- Task screens now head each section with its `title` from the workflow's render configuration, and show no heading where there is none ([#539](https://github.com/OpenNSW/nsw-srilanka/pull/539)). Every section in one-trade-artifacts `tnsw/` has one; add it to any artifacts of your own.

#### Stored files ([#578](https://github.com/OpenNSW/nsw-srilanka/pull/578))

- Before upgrading, look in stored form data for file keys that don't match `^[0-9a-fA-F-]{36}(\.[a-zA-Z0-9]+)?$`, such as `<uuid>.final-v2`. ePhyto and CusDec can no longer attach those files. They were already unreachable over HTTP.
- With the `local` storage backend, upload and download links issued before the upgrade stop working. They normally last only minutes.

#### Local Docker Compose

- Run `make setup` to seed `configs/config.docker.yaml` and `portals/apps/trader-app/public/config.js`. A local `configs/config.yaml` from before this release has no `db` section; regenerate it from the example.
- Update your git-ignored `configs/services*.json` from the examples: the `cda` and `customs` entries now request `nsw:workflow:inject` for `https://api.cda.nsw-agency.local` and `https://api.customs.nsw-agency.local`.
- Reset the local IdP once (`idp/sample-resources.down.sh`, then the seed), or recreate the `thunderid-*` volumes. The seed skips what already exists, so an older IdP keeps the old CDA and Customs clients, roles and scopes.

### Added

- **Agency mode.** The same images can run as a government agency's own backend and portal: set `mode: agency` in `config.yaml` and `APP_MODE: "agency"` in the portal's config. TNSW hands work to the agency through `POST /api/v1/inject`, and officers work through a case list. See [`docs/agency.md`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.2.0/docs/agency.md). Local Docker Compose now runs CDA and Customs this way, with CDA's documents served from TNSW's storage. TNSW deployments need no changes. ([#554](https://github.com/OpenNSW/nsw-srilanka/pull/554), [#566](https://github.com/OpenNSW/nsw-srilanka/pull/566), [#583](https://github.com/OpenNSW/nsw-srilanka/pull/583), [#588](https://github.com/OpenNSW/nsw-srilanka/pull/588))
- **Reference ID generator.** A `REFID_GENERATOR` workflow step fills one or more reference IDs, such as `TNSW-CMB-20260930-000042`, from formats defined under `refid.issuers` in `config.yaml`. With no issuers, any step that uses it fails. ([#549](https://github.com/OpenNSW/nsw-srilanka/pull/549), [#562](https://github.com/OpenNSW/nsw-srilanka/pull/562))
- **Proxy storage.** `storage.type: proxy` serves files from another service that owns them, named by `storage.proxy.service` in the services registry. Its credentials need that service's storage read, write and delete scopes, and the service's storage public URL must be absolute. ([#548](https://github.com/OpenNSW/nsw-srilanka/pull/548))

### Changed

- Task screens show their sections in the order the workflow definition gives, instead of a fixed order. ([#539](https://github.com/OpenNSW/nsw-srilanka/pull/539))
- The Trader Portal's demo auto-fill button is off unless `SHOW_AUTOFILL_BUTTON` is `"true"`. ([#463](https://github.com/OpenNSW/nsw-srilanka/pull/463))
- CusDec submissions no longer send a top-level `submitter`; it stays inside `properties`. ([#538](https://github.com/OpenNSW/nsw-srilanka/pull/538))

### Fixed

- CusDec declarations carry the declaration's total customs valuation and each item's customs value, taken from the form's valuation fields. Needs one-trade-artifacts#121; see the upgrade notes. ([#552](https://github.com/OpenNSW/nsw-srilanka/pull/552))

### Security

- GovPay+ calls are decrypted, and their responses encrypted, as the GovPay+ specification (§3) requires. A call whose `TransactionKey` does not decrypt with TNSW's private key is rejected with 401. ([#525](https://github.com/OpenNSW/nsw-srilanka/pull/525))
- File storage is upgraded to OpenNSW/core storage v0.3.0, and the allowed upload types and size limit are now set in `config.yaml`. ([#578](https://github.com/OpenNSW/nsw-srilanka/pull/578))
- The Trader Portal's `fast-uri` dependency is raised to 3.1.8 for [GHSA-qw65-cvwx-89v3](https://github.com/advisories/GHSA-qw65-cvwx-89v3) and [GHSA-58mr-gqgx-xq4g](https://github.com/advisories/GHSA-58mr-gqgx-xq4g). ([#541](https://github.com/OpenNSW/nsw-srilanka/pull/541))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- ThunderID 1.0.0-beta2 is a beta release.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment's engine status directly, at `/admin/consignments/<consignment ID>`.

## [0.1.0] - 2026-09-29

This is the first tagged release of TNSW, and the baseline that later releases are compared against. It describes what you are deploying rather than the ~400 commits that led here, which are in the [full history](https://github.com/OpenNSW/nsw-srilanka/commits/v0.1.0). Development moved to this repository on 2 June 2026 ([#1](https://github.com/OpenNSW/nsw-srilanka/pull/1)); pull request numbers in earlier commits refer to its predecessor, [OpenNSW/nsw](https://github.com/LSFLK-Archive/2026NSW-nsw).

### What's in this release

| Component                            | Artifact                                                |
| ------------------------------------ | ------------------------------------------------------- |
| Backend API, including the `otc` CLI | `ghcr.io/opennsw/tnsw-api:0.1.0`                        |
| Trader Portal                        | `ghcr.io/opennsw/tnsw-web:0.1.0`                        |
| Schema migrator (16 migrations)      | `ghcr.io/opennsw/tnsw-migrate:0.1.0`                    |
| Helm chart `lk-tnsw`                 | `oci://ghcr.io/opennsw/charts/lk-tnsw`, version `0.1.0` |

- Every image is built for `linux/amd64` and `linux/arm64`, with an SBOM and SLSA provenance attached.
- The API is built on [OpenNSW/core](https://github.com/OpenNSW/core) at `v0.0.0-20260924113947-9d0f524e49ee`.
- The Helm chart is released with the app, at the same version: chart 0.1.0 deploys the 0.1.0 images unless you set other image tags.

### What it does

- **Trader Portal:** traders and customs house agents (CHAs) sign in through ThunderID, create and track consignments, complete the tasks each agency sets, and pay fees.
- **Agency workflows** for FCAU, CDA, SLTB, NPQS, Customs and SLPA. The workflow definitions are not part of the images: the API loads them at run time from [OpenNSW/one-trade-artifacts](https://github.com/OpenNSW/one-trade-artifacts) (`tnsw/`), as configured by the `ARTIFACT_*` settings.
- **Integrations.** Each needs an entry in `services.json` and its own credentials:
  - Sri Lanka Customs (ASYCUDA), through SLC Edge interface spec v1.7: customs declarations, Cargo Dispatch Notes, and status callbacks.
  - The Sri Lanka Ports Authority (SLPA) Cargo Management System: cargo declaration, export service order, container consolidation, gate passes, invoice and payment. SLPA's callbacks are verified with an HMAC-SHA256 signature.
  - The IPPC ePhyto Hub, over SOAP with mutual TLS.
  - GovPay+, for fee payments.
- **Administration:** NSW admins can inspect the workflow engine state of a consignment or task, and resolve workflow steps parked for admin intervention. The `otc` CLI in the API image manages company records.
- **Access control:** API routes need an access token carrying the right scopes. The exceptions are `/health`, the SLPA callback (signed instead) and local-storage file links (signed URLs). The platform and the agencies call each other with OAuth2 client credentials.

### Requirements

TNSW depends on the services below, and the Helm chart deploys none of them. This release was tested with the versions in `compose.yml`.

- PostgreSQL 16.
- Temporal 1.28. The namespace named by `TEMPORAL_NAMESPACE` (default `default`) must exist before the API starts.
- ThunderID 1.0.0-beta2, as the identity provider.
- S3-compatible object storage for uploaded documents.
- Argus, for audit logging.
- Outbound HTTPS to GitHub (for the workflow definitions) and to every agency endpoint.

### Deploying

- **Migrations:** run `tnsw-migrate` (it runs `migrate up`) before the API starts. Docker Compose does this for you; with Helm, set `backend.migration.enabled=true` to run it as a pre-install and pre-upgrade hook.
- **Image tags:** leave `backend.image.tag` and `frontend.image.tag` unset to deploy this release's images; set them only to run others.
- **Config files:** the image holds only the `*.example.json` templates. Mount your own `services.json`, `payment_methods.json`, `notification.json` and `catalog.json` into `/app/configs` (with Helm, through `backend.volumes` and `backend.volumeMounts`). The API refuses to start without them. Load company records with `otc company apply -f <file>`.
- **Environment:** start from [`.env.example`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.1.0/.env.example).
- **Trader Portal:** its `VITE_*` settings are read when the container starts and default to `localhost`, so set every one of them. `VITE_IDP_SCOPES` must include the `nsw:*` scopes. Branding, including the copyright notice and footer links, is read from `/configs/branding.json` (with Helm, from `frontend.branding`).
- **Health:** `GET /health` returns 200 with the running version, or 503 naming the failing components (`database`, `authn`).

### Production checklist

- Leave `APP_ENV` unset. The insecure TLS options are honoured only when it is `development`; with any other value the API refuses to start while they are set.
- Set `SERVER_LOG_LEVEL=info`. At `debug`, the value in `.env.example`, the API logs full SLC Edge request and response bodies, declaration data included.
- Keep `DB_SSLMODE=require`, the default, and make sure PostgreSQL accepts TLS connections. `.env.example` sets `disable` for local use.
- Pin `ARTIFACT_GITHUB_REF` to a commit SHA of one-trade-artifacts that includes [one-trade-artifacts#104](https://github.com/OpenNSW/one-trade-artifacts/pull/104), such as [`ac9fd5f`](https://github.com/OpenNSW/one-trade-artifacts/commit/ac9fd5f9f870d0505433cfa501a25b3faef3ce87) (27 September 2026). The Trader Portal's search dropdowns don't work with older workflow definitions. With the default, `main`, workflow changes take effect as soon as they are pushed, with no release.
- Set every secret that `services.json` references with `env:`. The API refuses to start if one is unset or empty.
- Set `SLPA_WEBHOOK_SECRET`. The API refuses to start without it.
- Set `AUTH_CLIENT_IDS` to every client that calls the API, including `SLCE_TO_NSW` (Customs callbacks) and `GOVPAY_TO_NSW` (payment callbacks). Tokens from any other client are rejected. The default in `compose.yml` lists all nine.
- Use S3 storage, or set `STORAGE_LOCAL_PUT_SECRET`, which otherwise defaults to a development value.
- Set `VITE_SHOW_AUTOFILL_BUTTON=false`. The portal image shows a demo auto-fill button by default.
- Set `ARGUS_SERVICE_URL` and `ARGUS_API_KEY`, then confirm that audit events arrive in Argus.

### Known limitations

- Workflow definitions are not versioned with the release. Pinning `ARTIFACT_GITHUB_REF` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With `ARGUS_SERVICE_URL` unset, the API runs without audit and only logs "Audit client disabled" at startup.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- ThunderID 1.0.0-beta2 is a beta release.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment's engine status directly, at `/admin/consignments/<consignment ID>`.

[Unreleased]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/OpenNSW/nsw-srilanka/releases/tag/v0.1.0
