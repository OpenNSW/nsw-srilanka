# Changelog

All notable changes to TNSW, the Sri Lanka instance of the National Single Window platform, are recorded here for the people who deploy and run it. Each GitHub Release repeats its section from this file, then adds the image digests and the list of pull requests merged since the previous release.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Until 1.0.0, a minor release (0.x.0) may contain breaking changes; they are always listed under **Upgrade notes**.

## [Unreleased]

## [0.1.0] - 2026-09-28

This is the first tagged release of TNSW, and the baseline that later releases are compared against. It describes what you are deploying rather than the ~400 commits that led here, which are in the [full history](https://github.com/OpenNSW/nsw-srilanka/commits/v0.1.0). Development moved to this repository on 2 June 2026 ([#1](https://github.com/OpenNSW/nsw-srilanka/pull/1)); pull request numbers in earlier commits refer to its predecessor, [OpenNSW/nsw](https://github.com/LSFLK-Archive/2026NSW-nsw).

### What's in this release

| Component | Artifact |
| --- | --- |
| Backend API, including the `otc` CLI | `ghcr.io/opennsw/tnsw-api:0.1.0` |
| Trader Portal | `ghcr.io/opennsw/tnsw-web:0.1.0` |
| Schema migrator (16 migrations) | `ghcr.io/opennsw/tnsw-migrate:0.1.0` |
| Helm chart `lk-tnsw` 0.1.1 | `oci://ghcr.io/opennsw/charts/lk-tnsw` |

- Every image is built for `linux/amd64` and `linux/arm64`, with an SBOM and SLSA provenance attached.
- The API is built on [OpenNSW/core](https://github.com/OpenNSW/core) at `v0.0.0-20260924113947-9d0f524e49ee`.
- The Helm chart is released on its own `chart-v*` tags. Chart 0.1.1 targets this release (`appVersion: 0.1.0`).

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
- **Image tags:** the chart has no default. Set `backend.image.tag` and `frontend.image.tag` to `0.1.0`.
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

[Unreleased]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/OpenNSW/nsw-srilanka/releases/tag/v0.1.0
