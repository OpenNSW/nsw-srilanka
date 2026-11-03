# Changelog

All notable changes to TNSW, the Sri Lanka instance of the National Single Window platform, are recorded here for the people who deploy and run it. Each GitHub Release repeats its section from this file, then adds the image digests and the list of pull requests merged since the previous release.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Until 1.0.0, a minor release (0.x.0) may contain breaking changes; they are always listed under **Upgrade notes**. Release dates are the day of release in Sri Lanka time (UTC+05:30).

## [Unreleased]

## [0.13.0] - 2026-10-31

This release has no required upgrade steps, but we recommend two hardening steps: XML leaves the allowed upload types, and CDA, EFC, FCAU and SLTB give up the internal upload scope they never use. It also adds an optional `artifacts.fileCacheBytes` setting. The artifacts pin can move on its own this time: its changes are to forms, which the 0.12.0 build already runs.

### Upgrade notes

#### Take XML out of the allowed upload types (recommended) ([#1008](https://github.com/OpenNSW/nsw-srilanka/pull/1008))

No form stores an XML file any more: the CusDec imports parse the file in the browser and upload nothing, and the SLTB application in the pinned artifacts fills its draft CusDec on the form. Nothing fails if XML stays, but we recommend removing it, so that nothing can store a type no form needs. To remove it, in this order:

1. **The pinned artifacts first** (see below). The SLTB application in the earlier artifacts requires an XML upload, so removing XML first blocks every SLTB application with a 415. Let SLTB applications still on the earlier form finish, including any sent back for resubmission.
2. **TNSW.** Remove `text/xml` and `application/xml` from `storage.allowedUploadTypes` (Helm: `backend.config.storage.allowedUploadTypes`). Their `storage.uploadTypeExtensions` entries can go too; startup accepts them either way.
3. **Each agency.** Remove both types from `storage.proxy.allowedUploadTypes`.

After this, an upload of an XML file gets a 415, including to a file field that accepts any type. Files already stored as XML still download.

#### Take `nsw:storage:internalwrite` from CDA, EFC, FCAU and SLTB (recommended) ([#1001](https://github.com/OpenNSW/nsw-srilanka/pull/1001))

Only NPQS generates documents, so only its agency-to-TNSW client needs the scope of the internal upload route. CDA, EFC, FCAU and SLTB never call the route, and a client should hold only the scopes it uses. Nothing fails if they keep it, but we recommend removing it. The example IdP bootstrap and agency configs no longer grant it to them, so a new deployment gets this by default. To take it from a deployment that granted it as the 0.10.0 notes said, in this order:

1. **CDA, EFC, FCAU and SLTB first.** In each agency's `config.yaml`, remove `nsw:storage:internalwrite` from the scopes of the `tnsw` service, then restart the agency so it fetches a new token. An agency that asks for a scope its client no longer holds can fail its token request, so do this before the IdP. Keep `storage.proxy.internalUploadPath`: it is still required.
2. **The IdP, after the agencies.** A live IdP does not read `idp/bootstrap/`, so change it by hand. Remove `nsw:storage:internalwrite` from the `CdaToNswM2M`, `EfcToNswM2M`, `FcauToNswM2M` and `SltbToNswM2M` roles, and from the scopes of the `CDA_TO_NSW_M2M`, `EFC_TO_NSW_M2M`, `FCAU_TO_NSW_M2M` and `SLTB_TO_NSW_M2M` apps.

An agency that later generates documents is given the scope then, as the [onboarding guide](https://github.com/OpenNSW/nsw-srilanka/blob/v0.13.0/docs/guides/onboarding-agency-mode-agencies.md) describes.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`a7b8620`](https://github.com/OpenNSW/one-trade-artifacts/commit/a7b8620303ffaca73f7192f9b9ee12e73abdeb3a) (31 October 2026), for TNSW and for each agency. It changes forms only, so it needs no matching image version:
  - [one-trade-artifacts#306](https://github.com/OpenNSW/one-trade-artifacts/pull/306): the SLTB application carries the CusDec form, which the trader fills by importing a CusDec XML. The import is parsed and not stored, and the officer sees the same form. This is what lets XML leave the allowed upload types;
  - [one-trade-artifacts#300](https://github.com/OpenNSW/one-trade-artifacts/pull/300): importing an ASYCUDA XML on the CusDec form fills each item's HS code with all 10 digits;
  - [one-trade-artifacts#301](https://github.com/OpenNSW/one-trade-artifacts/pull/301): the CusDec form hides the currency when a valuation amount is 0;
  - [one-trade-artifacts#298](https://github.com/OpenNSW/one-trade-artifacts/pull/298): the CDA coconut certificate application asks for the contract number again, next to the contact number.
- Let SLTB applications submitted on the earlier form finish before you move the pin: the new form replaces the uploaded XML with a CusDec section, which those applications don't have ([one-trade-artifacts#306](https://github.com/OpenNSW/one-trade-artifacts/pull/306)).

### Added

- **Artifact file cache size.** The optional `artifacts.fileCacheBytes` (Helm: `backend.config.artifacts.fileCacheBytes`) sets how many bytes of artifact files the server keeps in memory once loaded. 0 or unset keeps core's default of 64 MiB, a positive value sets the size, and a negative value turns the cache off. Only versioned manifests are cached, so a source with only `manifest.json`, which the pinned artifacts are, is fetched every time whatever this is set to. ([#987](https://github.com/OpenNSW/nsw-srilanka/pull/987))

### Security

- CDA, EFC, FCAU and SLTB no longer hold `nsw:storage:internalwrite` in the example IdP bootstrap and agency configs, so that only a client that generates documents can write to the internal upload route. We recommend taking it from a live deployment: see the upgrade notes. ([#1001](https://github.com/OpenNSW/nsw-srilanka/pull/1001))
- The example configs no longer allow XML uploads. We recommend removing it from a live deployment: see the upgrade notes. ([#1008](https://github.com/OpenNSW/nsw-srilanka/pull/1008))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit records are log lines exported over OTLP at `info`, with no separate store and no delivery guarantee, and without `otel.endpoint` they are discarded. Requests to consignments and tasks are audited. Payments, the storage module's own events and SLC Edge callbacks are not, and startup logs a warning that payment and storage audit are not wired.
- The API can authenticate to Temporal only with a client certificate, not with an API key.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal's own text is in English, Sinhala and Tamil. The text of forms and workflows comes from the artifacts, which are in English only.
- A user with neither the trader nor the CHA catalog role, such as an NSW admin, gets a 403 on the consignment list, which the Trader Portal shows as "Could not load consignments." with a Try Again button. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- A temporary development aid can copy every SLC Edge callback to another deployment. It is off unless `services` lists an entry with the id `slce-debug-forward`, and it logs a warning at startup when it is on. Never define one in production: the aid will be removed before production use. ([#870](https://github.com/OpenNSW/nsw-srilanka/pull/870))

## [0.12.0] - 2026-10-30

This release reads each task's `read.roles` from its task template, so the image version and the artifacts pin must change in the same deployment: the pinned artifacts and this build depend on each other in three places. One migration runs and records each consignment's trader and CHA in a new `case_parties` table. File tokens change, so agency work that holds files must be finished or cleared first, and audit records now go to OTLP only and no longer to the pod log. It also fills each Customs dispatch note form in from the CusDec, checks file fields' rules on submit, and fixes the ePhyto commodity name and packaging.

### Upgrade notes

#### Change the image version and the artifacts pin together ([#979](https://github.com/OpenNSW/nsw-srilanka/pull/979), [#976](https://github.com/OpenNSW/nsw-srilanka/pull/976), [#942](https://github.com/OpenNSW/nsw-srilanka/pull/942))

- Move the image or chart version and `artifactLoader.github.ref` in one deployment, for TNSW and for each agency. Each of these breaks if only one of them moves:
  - Task read rules ([#979](https://github.com/OpenNSW/nsw-srilanka/pull/979)): a task's `read.roles` now sits on its task template, and the build reads it there. With the old build or the old artifacts no template declares `read.roles`, so TNSW applies no precedence: a user who is both a trader and a CHA on a consignment sees both views of a task at once.
  - ePhyto packaging ([#976](https://github.com/OpenNSW/nsw-srilanka/pull/976)): the build reads the packages of an NPQS commodity from the form's `packages` list, which only the pinned artifacts hold. The old form stores `packages_count`, which the build no longer reads. With either one alone, the package rows are left out of the certificate.
  - SLPA ECDN ([#942](https://github.com/OpenNSW/nsw-srilanka/pull/942)): the pinned ECDN form holds the CusDec number as Customs writes it (`CBEX1/2026/E/67948`). Only this build reduces it to the serial letter and number (`E67948`) that the SLPA CMS takes.
- The Customs dispatch note fill-in ([#957](https://github.com/OpenNSW/nsw-srilanka/pull/957)) works in either order: without one half, the forms open empty as before.

#### Upgrade TNSW before the agencies, and finish or clear agency work that holds files ([#949](https://github.com/OpenNSW/nsw-srilanka/pull/949))

- TNSW file tokens now end with the file's extension (`<token>.pdf`), and a token without it is refused. Tokens issued before the upgrade stop working. An agency stores TNSW's tokens, not file keys, so a file already held in agency task data stops opening. Let agency work that holds files finish before you upgrade, or cancel it, or reset that task data.
- Upgrade TNSW first. An agency checks a file field's `accept` rules against the extension on TNSW's tokens, which tokens from a 0.11.0 TNSW don't have.
- TNSW's own task data holds stored values, not tokens, so it is unaffected. A trader or officer with a page open across the upgrade holds old tokens and has to reload it.

#### Migration ([#958](https://github.com/OpenNSW/nsw-srilanka/pull/958))

One migration runs before the API starts; the Helm hook and Docker Compose run it. Use the 0.12.0 `tnsw-migrate` image.

- `000033_create_case_parties` creates `case_parties`, adds `cases.created_by`, and fills both from `consignments`: a `trader` party for every consignment, a `cha` party where a CHA company was chosen, and `created_by` from the trader's user. The trader and CHA columns of `consignments` stay and are still written. Rolling back drops `case_parties` and `cases.created_by`.
- The consignment list and detail, and task authorization, now read `case_parties`. During a rolling deploy, old pods write only the `consignments` columns: a consignment an old pod creates has no `trader` party, and a CHA chosen on one has no `cha` party, so the new pods miss them. Stop the old pods before the new ones serve traffic, or run the two backfill inserts in [`migrations/000033_create_case_parties.sql`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.12.0/migrations/000033_create_case_parties.sql) again after the deploy; each is an `INSERT ... ON CONFLICT DO NOTHING`.

#### Audit records go to OTLP only ([#925](https://github.com/OpenNSW/nsw-srilanka/pull/925))

- Audit records (`category=audit`) no longer reach the pod log. They are exported over OTLP only. Wherever you need the audit trail, set `otel.endpoint` and keep `server.logLevel` at `info` or `debug`. With export off, audit records are not kept, and startup logs a warning.
- Security log lines no longer carry `requestBody`, `responseBody`, `query` or `remoteAddr`. Change any OpenObserve query or other log search that used them. `category=audit` holds who did what.

#### Add `tel` and the CDA registration number to the company records ([one-trade-artifacts#293](https://github.com/OpenNSW/one-trade-artifacts/pull/293), [one-trade-artifacts#283](https://github.com/OpenNSW/one-trade-artifacts/pull/283))

- The pinned CDA applications fill in the trader's details from the company record: the contact number from `company.data.tel`, and the exporter's name, address and CDA registration number from `company.name`, `company.data.address` and `company.data.agencies.cda.registration_number`. The king coconut form also uses `company.data.contact_email`, which 0.11.0 asked for. A company without a value leaves the field empty for the trader to fill in. Add the keys to each company in your `configs/companies.json`, then re-apply it with `otc company apply -f configs/companies.json`; `data` is replaced as a whole, so apply the complete record.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`cdeecbd`](https://github.com/OpenNSW/one-trade-artifacts/commit/cdeecbd58f10e3640298bceccf6622ffd70dee6d) (30 October 2026), for TNSW and for each agency. It holds the artifact changes this release depends on:
  - [one-trade-artifacts#292](https://github.com/OpenNSW/one-trade-artifacts/pull/292): `read` on the task templates ([#979](https://github.com/OpenNSW/nsw-srilanka/pull/979));
  - [one-trade-artifacts#287](https://github.com/OpenNSW/one-trade-artifacts/pull/287): several package types on one NPQS commodity, as `packages` ([#976](https://github.com/OpenNSW/nsw-srilanka/pull/976));
  - [one-trade-artifacts#280](https://github.com/OpenNSW/one-trade-artifacts/pull/280): the SLPA ECDN waits for the assessed CusDec and holds its number as Customs writes it ([#942](https://github.com/OpenNSW/nsw-srilanka/pull/942));
  - [one-trade-artifacts#285](https://github.com/OpenNSW/one-trade-artifacts/pull/285): the dispatch note forms are filled in with the CusDec's shipper and consignee ([#957](https://github.com/OpenNSW/nsw-srilanka/pull/957));
  - [one-trade-artifacts#286](https://github.com/OpenNSW/one-trade-artifacts/pull/286): every file field states the file types it accepts, which this build now checks ([#949](https://github.com/OpenNSW/nsw-srilanka/pull/949)).
- It also lets NPQS issue the ePhyto certificate to non-ePhyto countries too ([one-trade-artifacts#284](https://github.com/OpenNSW/one-trade-artifacts/pull/284)), makes SLTB wait for the assessed CusDec before the officer's second step ([one-trade-artifacts#270](https://github.com/OpenNSW/one-trade-artifacts/pull/270)), starts the SLPA ECDN flow for every HS code ([one-trade-artifacts#297](https://github.com/OpenNSW/one-trade-artifacts/pull/297)), and requires the commodity description on the NPQS apply form ([one-trade-artifacts#273](https://github.com/OpenNSW/one-trade-artifacts/pull/273)).
- Finish or clear NPQS consignments whose application was reviewed before the upgrade, or re-review their applications. `ephyto_required` is replaced by `ephyto_country`, and `ephyto_reference_number` by `certificate_number`. A consignment reviewed before has no `ephyto_country`, so after issuance it parks at the gateway that chooses the path ([one-trade-artifacts#284](https://github.com/OpenNSW/one-trade-artifacts/pull/284)).
- Flows already started keep the definition they started with, so a consignment started before the upgrade doesn't wait for the assessed CusDec at SLTB ([one-trade-artifacts#270](https://github.com/OpenNSW/one-trade-artifacts/pull/270)).

### Added

- **Dispatch note forms open filled in.** Each Create CDN form opens with the CusDec's exporter as the shipper and the consignee's name, address and country. `CDN_SPLIT_BUILDER` takes an optional `prefill` object from the artifacts and gives every container's item a copy, so the artifacts decide which fields are filled in. ([#957](https://github.com/OpenNSW/nsw-srilanka/pull/957))
- **File fields' rules are checked on submit.** The `x-file` rules of a file field (`accept`, `maxFiles`) were enforced only by the Trader Portal's file control, so an API call could attach any allowed file to any file field, in any number. A submission that breaks them gets a 400 that names the path and the rule, never the value. Drafts and read-only values aren't checked, and only steps that list `payload_schema` are. ([#949](https://github.com/OpenNSW/nsw-srilanka/pull/949))

### Changed

- The SLPA ECDN takes the CusDec number as Customs writes it. A number typed by hand in four parts (`CBEX1-2026-E-1050`) is sent as its serial letter and number (`E1050`), and any other as letters and digits only. There is nothing to configure. ([#942](https://github.com/OpenNSW/nsw-srilanka/pull/942))
- A completed task no longer offers "Continue to next task", in the Trader Portal and the agency portals. Users return through the breadcrumb or with "Back to consignment" or "Back to case", which a completed task always shows. ([#955](https://github.com/OpenNSW/nsw-srilanka/pull/955))
- The example configs, the agency configs and `values-example.yaml` list their sections in a new order, most important first. No key changed, and a `config.yaml` or Helm values file in the old order still loads. ([#954](https://github.com/OpenNSW/nsw-srilanka/pull/954))

### Fixed

- The ePhyto mapping reads the NPQS commodity name from its dropdown answer. It was always empty, so a commodity with no description sent an empty trade line description, and the Hub refused the submission with "each tradeLine.description is mandatory". ([#927](https://github.com/OpenNSW/nsw-srilanka/pull/927))
- The ePhyto certificate carries one package element for each row of a commodity's `packages` (count and IPPC type), in the trader's order. Free-text and bulk arrangements are still sent as the package description. ([#976](https://github.com/OpenNSW/nsw-srilanka/pull/976))
- Audit events for task workflows, and for the admin actions on a flow (engine status, resolve and force-park), name the consignment as their target and no longer a workflow ID. When the consignment can't be resolved the target is empty, and the metadata keeps the `workflowId` and a `targetResolutionError`. With auditing on, resolve and force-park make one extra best-effort workflow status query. ([#964](https://github.com/OpenNSW/nsw-srilanka/pull/964), [#965](https://github.com/OpenNSW/nsw-srilanka/pull/965))
- The Trader Portal's page background now extends behind the top bar. ([#961](https://github.com/OpenNSW/nsw-srilanka/pull/961))

### Security

- Security log lines no longer record request or response bodies, query strings or remote addresses. A form's fields can't be redacted one by one, so none are logged. ([#925](https://github.com/OpenNSW/nsw-srilanka/pull/925))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit records are log lines exported over OTLP at `info`, with no separate store and no delivery guarantee, and without `otel.endpoint` they are discarded. Requests to consignments and tasks are audited. Payments, the storage module's own events and SLC Edge callbacks are not, and startup logs a warning that payment and storage audit are not wired.
- The API can authenticate to Temporal only with a client certificate, not with an API key.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal's own text is in English, Sinhala and Tamil. The text of forms and workflows comes from the artifacts, which are in English only.
- A user with neither the trader nor the CHA catalog role, such as an NSW admin, gets a 403 on the consignment list, which the Trader Portal shows as "Could not load consignments." with a Try Again button. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- A temporary development aid can copy every SLC Edge callback to another deployment. It is off unless `services` lists an entry with the id `slce-debug-forward`, and it logs a warning at startup when it is on. Never define one in production: the aid will be removed before production use. ([#870](https://github.com/OpenNSW/nsw-srilanka/pull/870))

## [0.11.0] - 2026-10-29

This release moves the catalog and the payment methods into `config.yaml`, and makes every task, flow and agency workflow resolve its templates through a manifest version, so work started before the upgrade must be finished or cancelled first. Five migrations run, storage needs a new required setting, and the image version and the artifacts pin must change in the same deployment. It also adds claiming tasks in the agency portals, Sinhala and Tamil in the Trader Portal, a notice when Customs cancels a declaration, and the chosen CHA's details in the agency flows.

### Upgrade notes

#### Finish or cancel open work first ([#871](https://github.com/OpenNSW/nsw-srilanka/pull/871), [#872](https://github.com/OpenNSW/nsw-srilanka/pull/872), [#873](https://github.com/OpenNSW/nsw-srilanka/pull/873), [#946](https://github.com/OpenNSW/nsw-srilanka/pull/946))

- Let open consignments, cases and tasks finish before you upgrade, or cancel them. This is not backwards compatible, and on dev and staging it means all of them. Tasks, case flows and agency workflows created before the upgrade have no manifest version, and tasks have no recorded task template, so after the upgrade:
  - a task's next step or activation fails with `NoManifestVersion`, and the task no longer renders;
  - a case flow can't be retried, and the consignment view shows its template ID in place of the flow's name;
  - a retried agency inject fails, and the case view falls back to each task's own render title.

#### Change the image version and the artifacts pin together ([#938](https://github.com/OpenNSW/nsw-srilanka/pull/938), [#819](https://github.com/OpenNSW/nsw-srilanka/pull/819))

- Move the image or chart version and `artifactLoader.github.ref` in one deployment, for TNSW and for each agency. The pinned trade flow requires the chosen CHA's details from the CHA step, which only this build outputs: on 0.10.0 every new consignment would park at Persist Selected CHA, before HS codes are picked. The pinned artifacts also hold the `customs-cusdec-cancelled` flow, which this build opens when Customs cancels a declaration.

#### Migrations ([#649](https://github.com/OpenNSW/nsw-srilanka/pull/649), [#871](https://github.com/OpenNSW/nsw-srilanka/pull/871), [#872](https://github.com/OpenNSW/nsw-srilanka/pull/872), [#873](https://github.com/OpenNSW/nsw-srilanka/pull/873), [#946](https://github.com/OpenNSW/nsw-srilanka/pull/946))

Five migrations run before the API starts; the Helm hook and Docker Compose run them. Use the 0.11.0 `tnsw-migrate` image.

- `000028_task_records_claims` adds `claimed_by` and `claimed_at` to `task_records_v2`.
- `000029_task_manifest_version` adds `task_records_v2.manifest_version`.
- `000030_case_flow_manifest_version` adds `case_flows.manifest_version`.
- `000031_agency_workflow_manifest_version` adds `agency_workflow.manifest_version`.
- `000032_task_records_task_template_id` adds `task_records_v2.task_template_id` and drops `task_records_v2.render_config`, with no backfill. Rolling back rebuilds the table, and `render_config` comes back empty.

Rolling back the first four drops the columns they add.

#### Move the catalog into `config.yaml` ([#930](https://github.com/OpenNSW/nsw-srilanka/pull/930))

- Add a top-level `catalog` section to `config.yaml` (Helm: `backend.config.catalog`), for TNSW and for every agency, holding the `roles` and `clients` of your `catalog.json`. Startup fails without it. Copy the shape from [`configs/config.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.11.0/configs/config.example.yaml) or [`values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.11.0/deployments/helm/values-example.yaml).
- Remove `server.catalogConfigPath`, which is no longer read, and drop `catalog.json` from the ConfigMap or volume mounted at `/app/configs`.

#### Move the payment methods into `config.yaml` ([#936](https://github.com/OpenNSW/nsw-srilanka/pull/936))

- **TNSW:** move the contents of `payment_methods.json` into a top-level `payment:` section of `config.yaml` (Helm: `backend.config.payment`), keeping the same `methods` list. Remove `server.paymentMethodsConfigPath`, which is no longer read, and drop the file mount. The GovPay+ private key mount at `/certs/govpay` is unchanged.
- Quote the GovPay+ ids, as in `subinst_id: "004"`. Unquoted, YAML reads `004` as the number 4, and the gateway refuses the mapping at startup.
- **Agencies:** remove `paymentMethodsConfigPath` and the `payment_methods.json` ConfigMap or mount. They need no `payment` section.
- The pinned artifacts use the same seven fees as 0.10.0, so the `payees` you already have are enough.

#### Set `storage.uploadTypeExtensions` ([#931](https://github.com/OpenNSW/nsw-srilanka/pull/931))

- A deployment with its own storage (`storage.type` of `local` or `s3`, which is TNSW) must set `storage.uploadTypeExtensions` (Helm: `backend.config.storage.uploadTypeExtensions`), or it fails to start. It lists the file name extensions each type may have, and needs an entry for every type in `allowedUploadTypes` and `allowedInternalUploadTypes`. Copy it from the example config or `values-example.yaml`.
- Agencies in proxy mode need nothing. A client that uploads a file whose name ends in an extension its type doesn't have now gets a 400. The Trader Portal and the agency portals take the type from the name, so they don't send such a pair.

#### Add `contact_email` to the company records ([#937](https://github.com/OpenNSW/nsw-srilanka/pull/937))

- The application forms in the pinned artifacts fill in the trader's email from `company.data.contact_email`, and leave the field empty until a company has one ([one-trade-artifacts#277](https://github.com/OpenNSW/one-trade-artifacts/pull/277)). Add the key to each company in your `configs/companies.json`, then re-apply it with `otc company apply -f configs/companies.json`. `data` is replaced as a whole, so apply the complete record. `make setup` doesn't overwrite an existing `configs/companies.json`.
- The same forms fill in the exporter's name, address and TIN from the company's `name`, `data.address` and `data.tin_no`, and the CusDec's declarant code from the chosen CHA's TIN.

#### Login page and footer (optional) ([#909](https://github.com/OpenNSW/nsw-srilanka/pull/909))

- To get the new login layout, split the login copy in `frontend.branding`: put the lead sentence in `tagline` and the rest in `description`. Remove `copyrightNotice` to drop the centered footer notice. Without a `tagline`, the layout is unchanged.
- The color scheme picker in the footer is now off by default. To keep it, set `frontend.config.SHOW_COLOR_SCHEME_SWITCHER: "true"`.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`83033e3`](https://github.com/OpenNSW/one-trade-artifacts/commit/83033e312f70dbe45f1d7c93c8b7ff8c90c773b6) (28 October 2026), for TNSW and for each agency. It holds the artifact changes this release depends on:
  - [one-trade-artifacts#279](https://github.com/OpenNSW/one-trade-artifacts/pull/279): the chosen CHA seeded into the agency flows ([#938](https://github.com/OpenNSW/nsw-srilanka/pull/938));
  - [one-trade-artifacts#211](https://github.com/OpenNSW/one-trade-artifacts/pull/211): the `customs-cusdec-cancelled` flow ([#819](https://github.com/OpenNSW/nsw-srilanka/pull/819)). Without it, a cancellation can't open its notice and is answered with a 500, and SLC Edge retries it until its retries run out.
- It also removes the NPQS certificate's hand signature step ([one-trade-artifacts#272](https://github.com/OpenNSW/one-trade-artifacts/pull/272)): the officer no longer prints, signs and uploads the certificate on the ePhyto path.

#### Local Docker Compose ([#930](https://github.com/OpenNSW/nsw-srilanka/pull/930), [#936](https://github.com/OpenNSW/nsw-srilanka/pull/936), [#931](https://github.com/OpenNSW/nsw-srilanka/pull/931), [#871](https://github.com/OpenNSW/nsw-srilanka/pull/871))

- `make setup` doesn't overwrite an existing `configs/config.yaml` or `configs/config.docker.yaml`, so copy the `catalog` and `payment` sections and `storage.uploadTypeExtensions` into them from the examples. Then delete `configs/catalog.json` and `configs/payment_methods.json`, which are no longer read.
- Clear your local consignments, cases and tasks, as above: recreating the database volume does it.

### Added

- **Claiming tasks in the agency portals.** An officer can claim a task so others see who is working on it, release it, or take over another officer's claim, for example when the holder is away. A claim is a hint, not a lock: submitting a step never checks it. Claims are on in agency mode and off in TNSW, where the claim routes answer 404. Claim and release use `nsw:task:write`, which officers already hold, so the IdP and the portal config don't change. Agency portals need the 0.11.0 `tnsw-web` image for the claim buttons and banner. ([#649](https://github.com/OpenNSW/nsw-srilanka/pull/649))
- **Manifest versions.** The artifact manifests are loaded as a set, so a source can publish `manifest_v1.json`, `manifest_v2.json` and so on. A flow, case flow or agency inject starts on the version that is current then and records it on its row, and each task, step, render and notification in it resolves its templates through that version. A source with only `manifest.json` loads as version 1, so a deployment without versioned manifests sees no change. The optional `artifacts.manifestVersion` pins the version new flows start on, for a rollback; the default 0 takes the highest published. ([#660](https://github.com/OpenNSW/nsw-srilanka/pull/660), [#871](https://github.com/OpenNSW/nsw-srilanka/pull/871), [#872](https://github.com/OpenNSW/nsw-srilanka/pull/872), [#873](https://github.com/OpenNSW/nsw-srilanka/pull/873))
- **Sinhala and Tamil in the Trader Portal.** The language switcher offers both, and the portal's own text and the task action buttons are translated. The text of forms and workflows comes from the artifacts. ([#928](https://github.com/OpenNSW/nsw-srilanka/pull/928))
- **A notice when Customs cancels a declaration.** When SLC Edge sends `CUSDEC_CANCELLED`, TNSW opens a notice on the consignment telling the trader the declaration is cancelled and can no longer be proceeded with, which the trader acknowledges. Nothing else is cancelled. A redelivery opens nothing twice. ([#819](https://github.com/OpenNSW/nsw-srilanka/pull/819))
- **The chosen CHA's details in the agency flows.** The CHA the trader picks reaches every agency flow as `_iter.input.cha`. The first use is the CusDec's declarant code. ([#938](https://github.com/OpenNSW/nsw-srilanka/pull/938))
- **Extra token claims.** `authn.extraUserClaims` and `authn.extraClientClaims` list further claims that reach handlers on the principal, for access rules that differ per deployment. Both are optional. Add the claim to the access token's attributes in the IdP as well, because the config only reads it. Prefer `optional`: a `required` claim rejects with a 401 every token that lacks it. Startup fails if a listed name is blank or reserved. ([#709](https://github.com/OpenNSW/nsw-srilanka/pull/709))
- **Force-park for admins.** An admin can force-park a waiting `TASK` node, which closes its task, or a step inside a task workflow, which leaves the task open, and then resolve it with `RETRY`, `COMPLETE` or `ABORT`. It needs `nsw:consignment:adminwrite`, which the admin resolve routes already use. When a step parks for admin intervention, its task row is held (`ON_HOLD`) and the trader and agency portals show a banner. ([#920](https://github.com/OpenNSW/nsw-srilanka/pull/920), [#921](https://github.com/OpenNSW/nsw-srilanka/pull/921))

### Changed

- A task's render config is resolved from the artifacts, in the task's manifest version, and is no longer copied onto its row. ([#946](https://github.com/OpenNSW/nsw-srilanka/pull/946))
- The file name of an upload must end in an extension its type has, or the upload gets a 400; a name with none is given the type's first. The stored file keeps the extension, and a download is named by it. ([#931](https://github.com/OpenNSW/nsw-srilanka/pull/931))
- The login page leads with the tagline, and the footer is a single "Powered by" line that shows the version only when the build sets one. See the upgrade notes. ([#909](https://github.com/OpenNSW/nsw-srilanka/pull/909))

### Fixed

- Exported spans carry the correlation id the API returns as `X-Trace-ID`, so a search for it finds the span as well as the log lines. A caller's `traceparent` is linked, not adopted. A leftover top-level `metrics` section in `config.yaml` is ignored and leaves export off: rename it to `otel`. ([#866](https://github.com/OpenNSW/nsw-srilanka/pull/866))
- The Trader Portal takes every color from Radix Themes. ([#950](https://github.com/OpenNSW/nsw-srilanka/pull/950))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit records are log lines written at `info`, with no separate store and no delivery guarantee. Requests to consignments and tasks are audited. Payments, the storage module's own events and SLC Edge callbacks are not, and startup logs a warning that payment and storage audit are not wired.
- The API can authenticate to Temporal only with a client certificate, not with an API key.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal's own text is in English, Sinhala and Tamil. The text of forms and workflows comes from the artifacts, which are in English only.
- A user with neither the trader nor the CHA catalog role, such as an NSW admin, gets a 403 on the consignment list, which the Trader Portal shows as "Could not load consignments." with a Try Again button. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- A temporary development aid can copy every SLC Edge callback to another deployment. It is off unless `services` lists an entry with the id `slce-debug-forward`, and it logs a warning at startup when it is on. Never define one in production: the aid will be removed before production use. ([#870](https://github.com/OpenNSW/nsw-srilanka/pull/870))

## [0.10.0] - 2026-10-26

This release changes how files are stored and opened. Every deployment that stores files must set `storage.inlineTypes`, machine clients get their own upload route and scope, and each agency's storage settings gain two required keys. The upgrade has a set order: the IdP first, then TNSW, then each agency, one at a time. It also sends ePhyto certificates from LK to the importing country, with replacement certificates and a date of inspection; raises Customs dispatch notes for the containers on the latest declaration, ends the branch of a cancelled note and fills in the boat note from Customs' data; and keeps a payment reference payable for a year. The image version and the artifacts pin must change in the same deployment.

### Upgrade notes

#### Upgrade in this order: the IdP, then TNSW, then each agency ([#893](https://github.com/OpenNSW/nsw-srilanka/pull/893), [#881](https://github.com/OpenNSW/nsw-srilanka/pull/881), [#898](https://github.com/OpenNSW/nsw-srilanka/pull/898))

Machine clients now upload on their own route, `POST /api/v1/internal/storage`, with their own scope, `nsw:storage:internalwrite`, and an agency relays what TNSW reports about a file. So the deployments depend on each other. Finish each step before you start the next. Every step is safe on its own: officers' uploads keep working throughout. An agency that moves before the IdP grant or before TNSW fails to store the files it generates, and a missing scope can fail its token request. Until TNSW is upgraded, View downloads every saved file for an agency's officers.

1. **The IdP, first.** Add the `internalwrite` action to the `storage` resource of the NSW API, and grant `nsw:storage:internalwrite` to the `<Agency>ToNswM2M` role and to the requestable scopes of the `<AGENCY>_TO_NSW_M2M` app of each agency (CDA, EFC, FCAU, NPQS and SLTB). Keep `nsw:storage:write`: officers' uploads still use it. A live IdP does not read `idp/bootstrap/`, so change it by hand.
2. **TNSW, second.** Deploy the 0.10.0 image and the pinned artifacts in the same deployment (see below). Set `storage.inlineTypes`, which is required (see below). Set `storage.allowedInternalUploadTypes` to the extra types an agency's own files may have, such as `npqs: [text/html]` for NPQS, which generates HTML documents, and add any other agency that does. Each name must be a client in `catalog.json`, or startup fails. The new route is live, and `POST /api/v1/storage` is unchanged.
3. **Each agency, last and one at a time.** Deploy the 0.10.0 image and the pinned artifacts in the same deployment. In its `config.yaml`, add `storage.proxy.internalUploadPath: /api/v1/internal/storage` (startup fails without it) and keep `uploadPath: /api/v1/storage`. Add `nsw:storage:internalwrite` to the scopes of the `tnsw` service, next to `nsw:storage:write`. Set `storage.proxy.allowedUploadTypes` (startup fails without it) to what officers may upload, for example TNSW's `allowedUploadTypes`, and no HTML or SVG type. Then restart the agency so it fetches its token again.

#### Change the image version and the artifacts pin together ([#882](https://github.com/OpenNSW/nsw-srilanka/pull/882), [#710](https://github.com/OpenNSW/nsw-srilanka/pull/710), [#894](https://github.com/OpenNSW/nsw-srilanka/pull/894), [#838](https://github.com/OpenNSW/nsw-srilanka/pull/838), [#883](https://github.com/OpenNSW/nsw-srilanka/pull/883))

- Move the image or chart version and `artifactLoader.github.ref` in one deployment, for TNSW and for each agency. This build adds the task types and ePhyto behavior that the pinned artifacts use, and 0.9.0 can't run them: the `CUSTOMS_CUSDEC_RETRIEVE` task type and the step that fills in the boat note fail as an unknown task type, and an ePhyto submission would go out with no From and no To, because the new artifacts no longer send the `hub_destination` the old build addresses it by.
- A deployment that moves the image alone still runs. Its ePhyto certificates carry no Date of Inspection until the artifacts arrive.

#### Set `storage.inlineTypes` ([#881](https://github.com/OpenNSW/nsw-srilanka/pull/881), [#898](https://github.com/OpenNSW/nsw-srilanka/pull/898))

- Add `storage.inlineTypes` to the `config.yaml` (Helm: `backend.config.storage`) of every deployment that uses the `local` or `s3` backend, or startup fails. Copy it from [`configs/config.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.10.0/configs/config.example.yaml). It lists the media types that a download opens in the browser; every other file is saved. Keep `text/html` in it, or the documents TNSW generates are saved instead of opened.
- Startup refuses a configuration in which a type that clients may upload (`allowedUploadTypes`) is also in `inlineTypes`, unless it is PDF, PNG, JPEG, GIF, WebP or plain text. A browser would run script in an HTML file from the origin it is served on, so `text/html` stays out of `allowedUploadTypes`.
- Agencies in proxy mode set nothing: they relay what TNSW reports. A download of a file that is not in the bucket is now a 404. S3 needs no new permission.
- The Trader Portal and the agency portals now treat a saved file as one to download unless the backend reports `disposition: inline`. That is why TNSW is upgraded before the agencies.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`aced4ec`](https://github.com/OpenNSW/one-trade-artifacts/commit/aced4ec7e08be89b6a775dcc2fbb4765b7bf8e2a) (26 October 2026), for TNSW and for each agency. It holds the artifact changes this release depends on:
  - [one-trade-artifacts#258](https://github.com/OpenNSW/one-trade-artifacts/pull/258): the NPQS certificate flow, which maps `date_of_inspection` ([#882](https://github.com/OpenNSW/nsw-srilanka/pull/882));
  - [one-trade-artifacts#194](https://github.com/OpenNSW/one-trade-artifacts/pull/194): the replacement of an issued NPQS certificate ([#710](https://github.com/OpenNSW/nsw-srilanka/pull/710));
  - [one-trade-artifacts#267](https://github.com/OpenNSW/one-trade-artifacts/pull/267): the ePhyto certificate sent to the importing country ([#894](https://github.com/OpenNSW/nsw-srilanka/pull/894));
  - [one-trade-artifacts#219](https://github.com/OpenNSW/one-trade-artifacts/pull/219): the acknowledgment step that tells the trader Customs cancelled a dispatch note ([#829](https://github.com/OpenNSW/nsw-srilanka/pull/829));
  - [one-trade-artifacts#229](https://github.com/OpenNSW/one-trade-artifacts/pull/229): the dispatch note branches sized from the latest declaration ([#838](https://github.com/OpenNSW/nsw-srilanka/pull/838));
  - [one-trade-artifacts#260](https://github.com/OpenNSW/one-trade-artifacts/pull/260): the boat note filled in from Customs ([#883](https://github.com/OpenNSW/nsw-srilanka/pull/883)).
- The fees in the pinned artifacts are the same seven as in 0.9.0, so `payment_methods.json` needs no change.
- It also changes how NPQS numbers a certificate ([one-trade-artifacts#266](https://github.com/OpenNSW/one-trade-artifacts/pull/266)): the officer no longer types a certificate number, and every certificate gets the generated reference as its number. The change spans the `npqs` and `tnsw` bundles, so move TNSW and the NPQS agency to this pin together.

#### Local Docker Compose ([#881](https://github.com/OpenNSW/nsw-srilanka/pull/881), [#893](https://github.com/OpenNSW/nsw-srilanka/pull/893), [#892](https://github.com/OpenNSW/nsw-srilanka/pull/892))

- `make setup` doesn't overwrite an existing `configs/config.yaml` or `configs/config.docker.yaml`, so add `storage.inlineTypes` and `storage.allowedInternalUploadTypes` to them from the examples.
- The local IdP bootstrap no longer holds SLPA's entities: the SLPA OU, the `Agency API` resource server, the `OGA Reviewer` and `NswM2M` roles, the `OGA Reviewers` group, the SLPA portal app, the `SLPA_TO_NSW` and `NSW_TO_SLPA` clients and the `slpa_officer` user. SLPA calls TNSW on `POST /webhooks/slpa`, signed with an HMAC, and TNSW calls SLPA's own OAuth2 server, so nothing used them. Run `docker compose down` and remove the `thunderid-*` volumes to drop them, and remove the `slpa` client from a `configs/catalog.json` copied from the example. In `configs/config.docker.yaml`, the `slpa` service now points at the same placeholder host as in `config.example.yaml`, in place of a local mock.

### Added

- **An internal upload route for machine clients.** `POST /api/v1/internal/storage` takes the files a deployment generates itself, such as the HTML documents an agency's `HTML_DOCUMENT_GENERATOR` stores, and needs the scope `nsw:storage:internalwrite`. It accepts `allowedUploadTypes` plus the extra types granted to the calling client in `storage.allowedInternalUploadTypes`. An agency's officers keep uploading on `POST /api/v1/storage`, so TNSW checks their uploads like a trader's. ([#893](https://github.com/OpenNSW/nsw-srilanka/pull/893))
- **ePhyto certificates from LK to the importing country.** Each certificate is addressed from `LK` to the importing country the application names, by its ISO code. Before, every certificate went `LK2` to `LK2`, and its import country always read `LK`. ([#894](https://github.com/OpenNSW/nsw-srilanka/pull/894))
- **Replacement ePhyto certificates, and a date of inspection.** TNSW can send a replacement certificate to the IPPC Hub, naming the certificate it replaces. The certificate's Date of Inspection now comes from a `date_of_inspection` task input, which the NPQS workflow fills with the date the officer recorded at the inspection, and not from the proposed date. ([#710](https://github.com/OpenNSW/nsw-srilanka/pull/710), [#882](https://github.com/OpenNSW/nsw-srilanka/pull/882))
- **Dispatch notes for the containers on the latest declaration.** A new `CUSTOMS_CUSDEC_RETRIEVE` task type fetches the declaration as it stands in ASYCUDA World, so the export flow raises one dispatch note per container Customs holds, after Customs has amended the declaration. It uses the existing `customs-asycuda` service. ([#838](https://github.com/OpenNSW/nsw-srilanka/pull/838))
- **Cancelled dispatch notes.** When Customs cancels a dispatch note (`CDN_CANCELLED`), TNSW marks it cancelled and ends that container's branch. The other containers and the declaration carry on. ([#829](https://github.com/OpenNSW/nsw-srilanka/pull/829))
- **A boat note filled in from Customs.** Before the boat note application opens, TNSW retrieves the declaration and each dispatch note, and fills in the boat note with one container row for each dispatch note Customs holds that is not cancelled. ([#883](https://github.com/OpenNSW/nsw-srilanka/pull/883))
- **A login tagline.** The Trader Portal's login page can show its branding text as separate paragraphs, from `frontend.branding` alone: a blank line in `description` starts a new paragraph, or `tagline` shows a second, emphasized paragraph under it. A deployment that sets neither looks the same as before. ([#895](https://github.com/OpenNSW/nsw-srilanka/pull/895))

### Changed

- A download reports the type and disposition that storage holds. `GET /api/v1/storage/{ref}` returns `mime_type` and `disposition` (`inline` or `attachment`) from storage, and no longer guesses the type from the extension, which now only names a saved file. See the upgrade notes. ([#881](https://github.com/OpenNSW/nsw-srilanka/pull/881))
- A payment reference stays payable for a year instead of a day. A reference that was not paid within a day used to be refused as not payable and could no longer be settled. References created before the upgrade keep their one-day expiry. ([#897](https://github.com/OpenNSW/nsw-srilanka/pull/897))
- The Trader Portal's form controls use `@opennsw/jsonforms-renderers` 0.17, which opens a saved file in the browser only when its `disposition` is `inline`. ([#898](https://github.com/OpenNSW/nsw-srilanka/pull/898))

### Fixed

- A failed consignment list shows an error with Try Again, where it used to look like a trader with no consignments. A failed task load does the same, and a failed create shows its error next to the button. ([#869](https://github.com/OpenNSW/nsw-srilanka/pull/869))
- Editable and read-only form fields are easier to tell apart, in the Trader Portal and in the agency portals. ([#887](https://github.com/OpenNSW/nsw-srilanka/pull/887))

### Security

- A file opens in the browser only when its type is in `storage.inlineTypes`. Startup refuses a configuration in which a type that clients may upload is also listed there, because a browser runs script in such a file from the origin it is served on. ([#881](https://github.com/OpenNSW/nsw-srilanka/pull/881))
- An agency's officers' uploads are checked against TNSW's upload types like a trader's. Only a machine client that holds `nsw:storage:internalwrite` and is named in `storage.allowedInternalUploadTypes` can store a type a browser opens as a document, such as HTML. ([#893](https://github.com/OpenNSW/nsw-srilanka/pull/893))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit records are log lines written at `info`, with no separate store and no delivery guarantee. Requests to consignments and tasks are audited. Payments, the storage module's own events and SLC Edge callbacks are not, and startup logs a warning that payment and storage audit are not wired.
- The API can authenticate to Temporal only with a client certificate, not with an API key.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- A user with neither the trader nor the CHA catalog role, such as an NSW admin, gets a 403 on the consignment list, which the Trader Portal shows as "Could not load consignments." with a Try Again button. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- A temporary development aid can copy every SLC Edge callback to another deployment. It is off unless `services` lists an entry with the id `slce-debug-forward`, and it logs a warning at startup when it is on. Never define one in production: the aid will be removed before production use. ([#870](https://github.com/OpenNSW/nsw-srilanka/pull/870))

## [0.9.0] - 2026-10-23

This release changes how a deployment is configured and audited. Outbound services move into `config.yaml`, where `services` is now required; Argus is gone, so audit records go to the log; and the agency bundles and the default consignment flow lose their version labels, so the image version and the artifacts pin must change in the same deployment. One migration runs. The consignment list now follows the roles in the token, and TNSW opens a flow on the case when Customs amends a declaration or a dispatch note. The Trader Portal groups a consignment's tasks into Active and Done tabs, titles each task page after its step, and serves agency cases under `/cases`.

### Upgrade notes

#### Change the image version and the artifacts pin together ([#864](https://github.com/OpenNSW/nsw-srilanka/pull/864), [#817](https://github.com/OpenNSW/nsw-srilanka/pull/817), [#822](https://github.com/OpenNSW/nsw-srilanka/pull/822))

- Move the image or chart version and `artifactLoader.github.ref` in one deployment, for TNSW and for each agency. This build reads the agency bundles as `cda`, `efc`, `fcau`, `npqs` and `sltb`, and the default consignment flow as `trade-export`. The pinned artifacts ([one-trade-artifacts#242](https://github.com/OpenNSW/one-trade-artifacts/pull/242)) name them that way, and 0.8.0 can't find them.
- Wherever an agency's `artifactLoader` names its folder (the local `root`, or the S3 or R2 `prefix`), drop the `-v2`: `cda-v2` becomes `cda`, and so on. Before the next sync of the artifacts to an object store, clear its old `cda/`, `efc/`, `fcau/`, `npqs/` and `sltb/` prefixes, which still hold the v1 agency files that [one-trade-artifacts#216](https://github.com/OpenNSW/one-trade-artifacts/pull/216) removed.
- Consignments and cases started before this release name the flow `trade-export-v1` and the old task codes, which the new artifacts no longer have. They do not carry over: let them finish before you upgrade, or discard them.

#### Declare the outbound services in `config.yaml` ([#859](https://github.com/OpenNSW/nsw-srilanka/pull/859))

- `services` is now required in `config.yaml` (Helm: `backend.config.services`), for TNSW and for each agency. The server stops at startup with an empty list. The services file, `services.json`, and the `server.servicesConfigPath` key are no longer read. Copy the list from [`configs/config.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.9.0/configs/config.example.yaml) or [`values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.9.0/deployments/helm/values-example.yaml), and move each entry across. `id`, `url`, `timeout` and `auth.type` keep their names. The rest change:

  | `services.json`                               | `config.yaml`                                   |
  | --------------------------------------------- | ----------------------------------------------- |
  | `auth.options.token_url`                      | `auth.oauth2.tokenURL`                          |
  | `auth.options.client_id`                      | `auth.oauth2.clientID`                          |
  | `auth.options.client_secret`, as `env:NAME`   | `auth.oauth2.clientSecret`, as `"{{env:NAME}}"` |
  | `auth.options.scopes`                         | `auth.oauth2.scopes`                            |
  | `auth.options.endpoint_params`                | `auth.oauth2.endpointParams`                    |
  | `auth.options.insecure_skip_tls_verify`       | `auth.oauth2.insecureSkipTLSVerify`             |
  | `tls.client_cert_file`, `tls.client_key_file` | `tls.clientCertFile`, `tls.clientKeyFile`       |

  A bearer or API key service uses `auth.bearer` or `auth.apiKey` in place of `auth.oauth2`.

- A secret is a quoted `"{{env:NAME}}"` or `"{{file:/path}}"` placeholder, and an unset one stops the process. Supply those variables or mounted files to every process that reads the YAML, including the database migration and seed jobs under Docker Compose. With Helm, the database migration reads its own database-only config, so it needs none of them.
- `insecureSkipTLSVerify` still needs `APP_ENV=development`.

#### Remove Argus and the `audit` section ([#854](https://github.com/OpenNSW/nsw-srilanka/pull/854), [#847](https://github.com/OpenNSW/nsw-srilanka/pull/847))

- Remove the `audit:` section from `config.yaml`, and `ARGUS_API_KEY` from the environment and from the Helm values (the `argus-api-key` key of `nsw-secrets`). The API no longer builds an Argus client, and Argus is no longer a dependency. A leftover `audit:` section is ignored, but a leftover `"{{env:ARGUS_API_KEY}}"` placeholder stops startup once nothing sets the variable, because placeholders are resolved before the file is decoded.
- Audit records are now structured log lines with `category=audit`, on the log stream of the API: filter that stream for them. They are written at `info`, so `server.logLevel` of `warn` or `error` drops them, and startup logs that once. Keep `info` or `debug` wherever you need the audit trail.

#### The consignment list follows the roles in the token ([#876](https://github.com/OpenNSW/nsw-srilanka/pull/876))

- Remove `IDP_ROLE_CLAIM_NAME`, `IDP_TRADER_ROLE_NAME`, `IDP_CHA_ROLE_NAME` and `IDP_NSW_ADMIN_ROLE_NAME` from the Trader Portal's `frontend.config` or `config.js`. A key left in is ignored. Keep the `role` scope in `IDP_SCOPES`, because the backend reads the roles from the token.
- Every signed-in user now reaches the portal, and the backend decides what they see. `GET /api/v1/consignments` returns the consignments where the caller's company is the trader (trader catalog role), the CHA (CHA catalog role), or either (both roles). A user with neither role gets a 403, which the portal shows as an empty list, in place of the unauthorized screen. A `?role=` sent by an old client is ignored.
- `POST /api/v1/consignments` now requires the trader catalog role. A CHA-only user who could create a consignment before now gets a 403.
- A user with both roles sees one combined list, with a Role column, in place of the role switcher.

#### Migration ([#814](https://github.com/OpenNSW/nsw-srilanka/pull/814))

- `000027_slce_link_dispatches_to_workflows` runs before the API starts, after `000025` and `000026`; the Helm hook and Docker Compose run it. Use the 0.9.0 `tnsw-migrate` image. It adds `case_id` and `workflow_id` columns to `cusdec_declarations` and `dispatch_notes`, fills them once from the task data, and adds a row for any dispatch still waiting for its first callback, so declarations and dispatch notes already in progress keep receiving their SLC Edge callbacks. It also drops two indexes. Rolling back drops the columns and recreates the indexes.

#### Keep the SLC Edge debug forward off in production ([#870](https://github.com/OpenNSW/nsw-srilanka/pull/870))

- This release includes a development aid that copies every SLC Edge callback to another deployment, for debugging. It is off unless `services` lists an entry with the id `slce-debug-forward`, and it logs a warning at startup when it is on. Leave that entry out in production, and remove it if you added one. The aid is temporary and will be removed before production use.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`28b4037`](https://github.com/OpenNSW/one-trade-artifacts/commit/28b40374c271802077adf5bb4ded97d8d172f06a) (23 October 2026), for TNSW and for each agency. It holds the artifact changes this release depends on:
  - [one-trade-artifacts#242](https://github.com/OpenNSW/one-trade-artifacts/pull/242): the bundles and the trade flow without version labels ([#864](https://github.com/OpenNSW/nsw-srilanka/pull/864));
  - [one-trade-artifacts#210](https://github.com/OpenNSW/one-trade-artifacts/pull/210): the `customs-cusdec-amended` flow ([#817](https://github.com/OpenNSW/nsw-srilanka/pull/817)). Without the flow, an amended declaration is answered with a 500 and SLC Edge retries it;
  - [one-trade-artifacts#215](https://github.com/OpenNSW/one-trade-artifacts/pull/215): the `customs-cdn-amended` flow ([#822](https://github.com/OpenNSW/nsw-srilanka/pull/822)). The same holds for an amended dispatch note.
- It also lists `payload_schema` on every step where a person fills in a form ([one-trade-artifacts#256](https://github.com/OpenNSW/one-trade-artifacts/pull/256)), so a submission that breaks its form's schema now gets a 400 on each of them. This build holds dates to the limits a form sets from a sibling date.

#### Local Docker Compose ([#864](https://github.com/OpenNSW/nsw-srilanka/pull/864), [#859](https://github.com/OpenNSW/nsw-srilanka/pull/859), [#854](https://github.com/OpenNSW/nsw-srilanka/pull/854))

- Pull one-trade-artifacts, then drop the `-v2` from any `CDA_ARTIFACT_LOCAL_ROOT`, `EFC_ARTIFACT_LOCAL_ROOT`, `FCAU_ARTIFACT_LOCAL_ROOT`, `NPQS_ARTIFACT_LOCAL_ROOT` or `SLTB_ARTIFACT_LOCAL_ROOT` in your `.env`.
- `make setup` doesn't overwrite an existing `configs/config.yaml` or `configs/config.docker.yaml`, so update them from the examples: add the `services` list, and remove `server.servicesConfigPath` and the `audit:` section. A leftover `"{{env:ARGUS_API_KEY}}"` stops startup, because Compose no longer sets that variable. The `configs/services*.json` files are no longer read.
- Remove `ARGUS_PORT` and `ARGUS_API_KEY` from your `.env`. `docker compose up` no longer builds or starts Argus.
- Remove the four `IDP_*_ROLE` keys from your `portals/apps/trader-app/public/config.js`.

### Added

- **Amended declarations and dispatch notes.** When Customs amends a registered declaration (`CUSDEC_AMENDED`) or a dispatch note, TNSW opens a new flow on its case, seeded with the amended data, for the trader to review and acknowledge. Without the flow template in the artifacts, the amendment is answered with a 500 and SLC Edge retries it. ([#817](https://github.com/OpenNSW/nsw-srilanka/pull/817), [#822](https://github.com/OpenNSW/nsw-srilanka/pull/822))
- **Date limits in form validation.** The `payload_schema` check enforces a date's minimum and maximum when the limit is read from another date in the form, such as "can't be later than the sailing date", as the Trader Portal does. ([#862](https://github.com/OpenNSW/nsw-srilanka/pull/862))
- **More audit events.** Successful reads and denied list requests are recorded, a file-resolution denial keeps the command or the draft flag, and a step with no `authz` rule is recorded as `step_not_guarded`. ([#586](https://github.com/OpenNSW/nsw-srilanka/pull/586), [#854](https://github.com/OpenNSW/nsw-srilanka/pull/854))
- **Tasks in Active and Done tabs.** The consignment page groups a consignment's tasks by state. Active holds the tasks that need action, each with its Continue button, then the ones waiting on others with the date they were sent. Done holds finished tasks, newest first, with when each was completed and a View link. A finished consignment opens on Done. This replaces the Action Required and In Review list and the Process History toggle. ([#879](https://github.com/OpenNSW/nsw-srilanka/pull/879))

### Changed

- Audit records go to the log instead of Argus. See the upgrade notes. ([#847](https://github.com/OpenNSW/nsw-srilanka/pull/847), [#854](https://github.com/OpenNSW/nsw-srilanka/pull/854))
- A declaration or dispatch note records its case and workflow when SLC Edge accepts it, and a callback reads the waiting task through the task store by those, instead of searching task data. ([#814](https://github.com/OpenNSW/nsw-srilanka/pull/814))
- The consignment list shows the caller's role on each row, and each listed consignment carries `callerRoles`. ([#876](https://github.com/OpenNSW/nsw-srilanka/pull/876))
- In agency mode (`APP_MODE: "agency"`) the portal says Cases instead of Consignments, and serves them at `/cases`, `/cases/:id` and `/cases/:id/tasks/:taskId` in place of the paths under `/consignments`, so update any saved or shared link to an agency portal page. An agency case shows no empty EXPORT or IMPORT badge. ([#879](https://github.com/OpenNSW/nsw-srilanka/pull/879))

### Fixed

- A refused upload names its file type in the 415, such as `invalid or prohibited file type: application/vnd.ms-excel`. ([#858](https://github.com/OpenNSW/nsw-srilanka/pull/858))
- The Trader Portal's task page is titled after its step, with a breadcrumb, and the consignment page has the same header: the name, the consignment ID with a copy button, and the date created. ([#861](https://github.com/OpenNSW/nsw-srilanka/pull/861), [#868](https://github.com/OpenNSW/nsw-srilanka/pull/868))
- The Trader Portal's buttons, checkboxes, focus rings and form controls use the color scheme's brand color, and its status badges and grey labels now reach a 4.5:1 text contrast. The main button was a bright blue at 3.26:1. ([#880](https://github.com/OpenNSW/nsw-srilanka/pull/880))

### Security

- Creating a consignment requires the trader role in the API. Until now any signed-in user could create one, and hiding the button in the portal was the only guard. ([#876](https://github.com/OpenNSW/nsw-srilanka/pull/876))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit records are log lines written at `info`, with no separate store and no delivery guarantee. Requests to consignments and tasks are audited. Payments, the storage module's own events and SLC Edge callbacks are not, and startup logs a warning that payment and storage audit are not wired.
- The API can authenticate to Temporal only with a client certificate, not with an API key.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- A user with neither the trader nor the CHA catalog role, such as an NSW admin, gets a 403 on the consignment list, which the Trader Portal shows as an empty list. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- Files of tasks started before their `render.json` declared `files` no longer open: those tasks hold raw storage keys, which are refused. ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628))
- A temporary development aid can copy every SLC Edge callback to another deployment. It is off unless `services` lists an entry with the id `slce-debug-forward`, and it logs a warning at startup when it is on. Never define one in production: the aid will be removed before production use. ([#870](https://github.com/OpenNSW/nsw-srilanka/pull/870))

## [0.8.0] - 2026-10-22

This release refuses any task step whose workflow artifact declares no `authz` rule, so the artifacts pin and each agency's catalog must move with it. It also renames the `metrics` config section to `otel` and exports logs and traces as well as metrics, can connect to Temporal over TLS and mutual TLS, logs every HTTP request as a security event, and resumes the CusDec export release step that never resumed before. No migrations run.

### Upgrade notes

#### Move the artifacts pin with this release ([#848](https://github.com/OpenNSW/nsw-srilanka/pull/848))

- This build refuses every action on a task step that declares no `authz` rule, and no setting turns that off. Only one-trade-artifacts from [one-trade-artifacts#227](https://github.com/OpenNSW/one-trade-artifacts/pull/227) on declare one on every step. Upgrade TNSW and each agency with those artifacts, in the same deployment as the image or chart version.
- Pin `artifactLoader.github.ref` to one-trade-artifacts [`e98cce1`](https://github.com/OpenNSW/one-trade-artifacts/commit/e98cce1b84c4a515086b8742419e9cb171d6ea51) (22 October 2026), for TNSW and for each agency. It holds:
  - [one-trade-artifacts#227](https://github.com/OpenNSW/one-trade-artifacts/pull/227): a rule on every step, and `read.roles` on every TNSW task. The rules apply to tasks already in progress. `read.roles` applies only to tasks started after the move.
  - [one-trade-artifacts#228](https://github.com/OpenNSW/one-trade-artifacts/pull/228): the FCAU item ID and the SLPA eCDN container number stop being read-only, ahead of the read-only check ([#843](https://github.com/OpenNSW/nsw-srilanka/pull/843)).
  - [one-trade-artifacts#209](https://github.com/OpenNSW/one-trade-artifacts/pull/209): the KCFN recommendation letter becomes a list of files. A KCFN case already past the issue-letter step fails at upload-docs, so finish or restart those cases before you upgrade.
  - [one-trade-artifacts#230](https://github.com/OpenNSW/one-trade-artifacts/pull/230) and [#231](https://github.com/OpenNSW/one-trade-artifacts/pull/231): SLTB's lab test report is uploaded and verified at the Export Division, and the final decision no longer issues a tea export certificate. Applications already past lab testing finish on the workflow version they started on.
  - Form fixes for CusDec, CDN, NPQS, FCAU and CDA ([#223](https://github.com/OpenNSW/one-trade-artifacts/pull/223), [#233](https://github.com/OpenNSW/one-trade-artifacts/pull/233), [#234](https://github.com/OpenNSW/one-trade-artifacts/pull/234), [#235](https://github.com/OpenNSW/one-trade-artifacts/pull/235), [#236](https://github.com/OpenNSW/one-trade-artifacts/pull/236), [#160](https://github.com/OpenNSW/one-trade-artifacts/pull/160), [#218](https://github.com/OpenNSW/one-trade-artifacts/pull/218), [#237](https://github.com/OpenNSW/one-trade-artifacts/pull/237)).
- For local development, pull your artifacts checkout or move its ref past one-trade-artifacts#227. An older one refuses every step.

#### Map TNSW's client in each agency's catalog ([#845](https://github.com/OpenNSW/nsw-srilanka/pull/845))

- Add `"clients": { "tnsw": "<client>" }` to each agency's `catalog.json`, naming the client TNSW calls that agency as (for example `NSW_TO_SLTB`). [`configs/agency/catalog.example.json`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.8.0/configs/agency/catalog.example.json) shows the entry. Without it, the pinned artifacts' rules refuse TNSW's replies to the agency steps that wait for them. The clients already exist, so the IdP needs no change.

#### Rename `metrics` to `otel` in `config.yaml` ([#671](https://github.com/OpenNSW/nsw-srilanka/pull/671))

- Rename the `metrics` section of `backend.config` to `otel`. The API ignores the old key without an error, so a deployment that keeps it stops exporting.
- `otel.endpoint` is now the OTLP/HTTP base URL, such as `http://otel-collector:4318`, and the API appends `/v1/metrics`, `/v1/logs` and `/v1/traces` to it. An old endpoint ending in `/v1/metrics` still works: the suffix is stripped.
- The example auth header moves from `METRICS_AUTH_HEADER` to `OTEL_AUTH_HEADER`, read from an optional `otel-auth-header` Secret. [`values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.8.0/deployments/helm/values-example.yaml) shows the Secret, the `backend.env` entry and the `otel` block. A deployment with export off needs no change.

#### Temporal over TLS (optional) ([#846](https://github.com/OpenNSW/nsw-srilanka/pull/846))

- Nothing changes unless you set `temporal.tls`. To connect to a Temporal server that requires TLS or client certificates:
  1. Create a `temporal-client-tls` Secret with `ca.crt`, `tls.crt` and `tls.key`. A cert-manager `Certificate` Secret has these keys.
  2. Mount it at `/certs/temporal` through `backend.volumes` and `backend.volumeMounts`.
  3. Set `caFile`, `certFile` and `keyFile` under `temporal.tls` in `backend.config`. Also set `serverName` if the server's certificate doesn't name `temporal.host`.
- The files are read once at startup, so restart the backend after the certificate is renewed.

### Added

- **Logs and traces over OTLP.** With `otel` configured, the API exports logs and traces as well as metrics, for example to OpenObserve. ([#671](https://github.com/OpenNSW/nsw-srilanka/pull/671))
- **Temporal over TLS and mutual TLS**, configured under `temporal.tls`. ([#846](https://github.com/OpenNSW/nsw-srilanka/pull/846))
- **A security log of every HTTP request.** Every request except `GET /health` adds one info line to the process log, with `category=security`: the caller, the time, a text request body of up to 4 KiB, and the response. ([#844](https://github.com/OpenNSW/nsw-srilanka/pull/844))
- **Read-only values are enforced.** On a step that lists the `payload_schema` extension, a submission or draft that changes a value the schema marks `readOnly` is refused. The pinned artifacts list the extension on the NPQS application step only. ([#843](https://github.com/OpenNSW/nsw-srilanka/pull/843))

### Changed

- A task step with no `authz` rule refuses every caller instead of letting anyone act. See the upgrade notes. ([#848](https://github.com/OpenNSW/nsw-srilanka/pull/848))
- An agency's `catalog.json` can map `tnsw` under `clients`, so its rules can name TNSW. ([#845](https://github.com/OpenNSW/nsw-srilanka/pull/845))
- The Trader Portal's top bar fits small screens, and the notifications bell, which did nothing, is hidden. ([#842](https://github.com/OpenNSW/nsw-srilanka/pull/842))

### Fixed

- An `EXPORT_RELEASED` event from Customs now resumes the "Wait on Export Release" step. That step runs in a parallel branch, and before this fix the event never reached it. Payment and warranting events are looked up the same way. A declaration whose release was already lost to this bug is `RELEASED`, so Customs resending the event changes nothing; complete its waiting step by hand. ([#849](https://github.com/OpenNSW/nsw-srilanka/pull/849))
- The ePhyto certificate sent to the IPPC Hub carries the codes of the NPQS application's dropdown answers again. Since 0.7.0's artifacts, those answers have been stored with their labels. ([#695](https://github.com/OpenNSW/nsw-srilanka/pull/695))

### Security

- The security log records request bodies and responses, and responses can carry personal data such as email addresses and phone numbers. Signed URLs and other credential fields are redacted. Restrict who can read the API's logs, and expect more log volume. ([#844](https://github.com/OpenNSW/nsw-srilanka/pull/844))
- Every task step now says who may act on it, and a step without a rule is closed rather than open. ([#848](https://github.com/OpenNSW/nsw-srilanka/pull/848))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API can authenticate to Temporal only with a client certificate, not with an API key.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- Files of tasks started before their `render.json` declared `files` no longer open: those tasks hold raw storage keys, which are refused. ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628))

## [0.7.0] - 2026-10-22

This release identifies each fee by the organization and service it pays, and reads the GovPay+ ids for them from `payment_methods.json`. It also settles a GovPay+ update that carries no status, matches SLC Edge callbacks by `edge_id` only, validates task form submissions against their JSON Schema, and shows long application forms in the Trader Portal one step at a time. Two migrations run, `payment_methods.json` needs a `payees` block, and the image version and the artifacts pin must change in the same deployment.

### Upgrade notes

#### Change the image version and the artifacts pin together ([#834](https://github.com/OpenNSW/nsw-srilanka/pull/834), [#821](https://github.com/OpenNSW/nsw-srilanka/pull/821))

- Move the image or chart version and `artifactLoader.github.ref` in one deployment. This build needs artifacts that name each fee's payee: with older ones, every PAYMENT step fails asking for `org_id` and `service_id`. The pinned artifacts, in turn, need this build: they enable the `payload_schema` check on the NPQS application step, which 0.6.0 rejects every submission on, and they carry no `gateway_metadata`, which 0.6.0 reads for payments.

#### Map the GovPay+ ids in `payment_methods.json` ([#834](https://github.com/OpenNSW/nsw-srilanka/pull/834))

- Add a `payees` block to the `govpay` method, with this environment's GovPay+ sub-institution and service ids: one entry per organization, with its `subinst_id` under `defaults` and a `service_id` for each fee under `services`. [`configs/payment_methods.example.json`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.7.0/configs/payment_methods.example.json) maps the seven current fees (FCAU `app_fee`, NPQS `certificate_fee` and `treatment_fee`, SLTB `levy_fee` and `lab_fee`, CDA `app_fee` and `kcfn_fee`). The ids differ per environment, so use your own.
- The backend refuses to start if a mapping lacks `subinst_id` or `service_id`. A fee whose payee isn't mapped fails its PAYMENT step, naming the payee. A GovPay+ call that carries other ids than the payee's mapping gets a 404, like an unknown reference.
- A PAYMENT step uses its `selected_method` if that method maps the payee, and otherwise the only method that does. With several and no selection it fails, listing them: there is no fallback to `govpay`.
- Agencies need no change: their `payment_methods.json` has no methods.

#### Migrations

Two migrations run before the API starts; the Helm hook and Docker Compose run them. Use the 0.7.0 `tnsw-migrate` image.

- `000025_slce_edgeid_only` ([#813](https://github.com/OpenNSW/nsw-srilanka/pull/813)) renames the `edgeId` key to `edge_id` in the customs data (`cig` and `cig_cdn`) of `task_records_v2`, and drops the two indexes that served it. SLC Edge callbacks now find the waiting task by `edge_id` only, so a task already waiting needs this rename. Rolling back recreates the indexes but does not rename the key back.
- `000026_payment_payee` ([#834](https://github.com/OpenNSW/nsw-srilanka/pull/834)) adds `org_id` and `service_id` to `payment_transactions`, and fills them in for existing payments from the GovPay+ ids or task code they stored. A pending payment for any of the seven current fees keeps working; any other pending payment can no longer be presented or settled. Rolling back drops the columns.

#### Catalogs can't define `system` ([#824](https://github.com/OpenNSW/nsw-srilanka/pull/824))

- Remove any role or client named `system` from TNSW's `catalog.json` and from each agency's. The API now refuses to start with one, because `system` is reserved for the service's own completions. The example catalogs have none.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`b7798b9`](https://github.com/OpenNSW/one-trade-artifacts/commit/b7798b98e5b419fa6d3884abc6ac6a57deb07391) (21 October 2026). It holds every artifact change this release depends on:
  - [one-trade-artifacts#225](https://github.com/OpenNSW/one-trade-artifacts/pull/225): each PAYMENT step names its payee ([#834](https://github.com/OpenNSW/nsw-srilanka/pull/834));
  - [one-trade-artifacts#201](https://github.com/OpenNSW/one-trade-artifacts/pull/201): the GovPay+ ids leave the PAYMENT steps ([#834](https://github.com/OpenNSW/nsw-srilanka/pull/834));
  - [one-trade-artifacts#208](https://github.com/OpenNSW/one-trade-artifacts/pull/208): the previous SLC Edge attempt's id is read from where it is stored ([#813](https://github.com/OpenNSW/nsw-srilanka/pull/813));
  - [one-trade-artifacts#220](https://github.com/OpenNSW/one-trade-artifacts/pull/220): server-side validation of the NPQS application ([#821](https://github.com/OpenNSW/nsw-srilanka/pull/821));
  - [one-trade-artifacts#203](https://github.com/OpenNSW/one-trade-artifacts/pull/203): the long application forms split into steps ([#805](https://github.com/OpenNSW/nsw-srilanka/pull/805)).
- It also holds the FCAU per-item inspection ([one-trade-artifacts#226](https://github.com/OpenNSW/one-trade-artifacts/pull/226)), which has no compatibility with FCAU applications already in progress. Let them finish before you upgrade.

#### Local Docker Compose ([#827](https://github.com/OpenNSW/nsw-srilanka/pull/827), [#834](https://github.com/OpenNSW/nsw-srilanka/pull/834))

- The local IdP now serves plain HTTP, and is published on `127.0.0.1` only. Your git-ignored copies keep the old settings, so update them by hand:
  - `idp/.env`: `PUBLIC_URL`, `PORT` and `HOSTNAME` are no longer read and can be removed.
  - `configs/config*.yaml`, `configs/services*.json` and `portals/apps/trader-app/public/config.js`: change the IdP URLs (`localhost:8090`, `thunderid:8090`) to `http://`, and remove the insecure-TLS flags.
  - Run `docker compose up` so `thunderid-setup` re-imports the Console with `http://` redirect URIs. The token issuer becomes `http://localhost:8090`, so sign in again.
- To pay fees on the local stack, copy the `payees` block of `configs/payment_methods.example.json` into your git-ignored `configs/payment_methods.json`.

### Added

- **Form validation on the server.** A step template can list the `payload_schema` extension, with the form's id as its `form` property. A submission is then checked against that form's JSON Schema, so a call that skips the browser is held to the same rules as the Trader Portal. An invalid payload gets a 400 with a fixed message, and the failing fields are logged at warn level. Draft saves are not checked. Nothing is validated until a step lists the extension: the pinned artifacts list it on the NPQS application step. ([#821](https://github.com/OpenNSW/nsw-srilanka/pull/821))
- **A reserved `system` principal.** The payment service, the CusDec and CDN webhooks, and the SLPA order and invoice events complete task steps as `system`, and a payment step's completion carries the command `submit`. A step's `authz` rule can now admit them by naming it, such as `"QUEUED_EXTERNALLY": { "submit": ["system"] }`. No request can act as `system`. ([#824](https://github.com/OpenNSW/nsw-srilanka/pull/824), [#825](https://github.com/OpenNSW/nsw-srilanka/pull/825))
- **Stepper forms.** The Trader Portal shows a form whose `Categorization` has `options.variant: "stepper"` one step at a time, with Submit on the last, review step. The pinned artifacts split the long application forms this way. ([#805](https://github.com/OpenNSW/nsw-srilanka/pull/805))

### Changed

- Fees are identified by the organization and service they pay (`org_id` and `service_id` in the PAYMENT step) instead of a task code, and the GovPay+ ids are mapped in `payment_methods.json`. See the upgrade notes. ([#834](https://github.com/OpenNSW/nsw-srilanka/pull/834))
- The Trader Portal's form controls use `@opennsw/jsonforms-renderers` 0.16. Removing a list item or an uploaded file asks first, and the Remove button of an upload moves into a More actions menu next to View and Download. A list configured with `options.display: "cards"` shows each item as a card that opens in a dialog, with optional running totals above it; the pinned artifacts configure cards on the CusDec, NPQS, CDA, SLPA eCDN and FCAU application forms. ([#840](https://github.com/OpenNSW/nsw-srilanka/pull/840))

### Fixed

- A GovPay+ update that carries no status settles the payment. The update carries back only the returned parameters (`refNo`, `amount` and `currency`), so it used to be rejected as an unsupported status and the payment stayed unsettled. The amount and currency are still checked against the recorded ones. ([#831](https://github.com/OpenNSW/nsw-srilanka/pull/831))
- A saved form's files are named and opened correctly. `GET /api/v1/storage/{ref}` now also returns the file's `extension` and `mime_type`, which the 0.16 renderers use for the download name and the View button. An agency relays them once TNSW runs this version too. ([#835](https://github.com/OpenNSW/nsw-srilanka/pull/835))
- On a task page, the footer no longer covers the form's action bar. ([#839](https://github.com/OpenNSW/nsw-srilanka/pull/839))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- Files of tasks started before their `render.json` declared `files` no longer open: those tasks hold raw storage keys, which are refused. ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628))
- On the CDA application form, an `.xls` upload to the DC bags field is named `DC BAGS DETAILS _1.xlsx.xls`, because the artifact's `x-file.fileName` ends in `.xlsx` while the field also accepts `.xls`. Only the displayed and downloaded name is affected. The fix is in the artifact, not in TNSW.

## [0.6.0] - 2026-10-20

This release lets an agency's review show the officer what they reviewed while it waits for a trader's resubmission, and has the Trader Portal warn before a form with unsaved answers is left. Deploy it on TNSW and on every agency before, or together with, moving the artifacts pin.

### Upgrade notes

#### Reply steps send only a `payload` input ([#804](https://github.com/OpenNSW/nsw-srilanka/pull/804))

- Deploy 0.6.0 on TNSW and on each agency (CDA, EFC, FCAU, NPQS and SLTB) before, or together with, the artifacts that use it. With an older build, a "needs more info" reply reaches TNSW with its fields nested under `payload`, and TNSW's step parks for an admin.
- No config, migration or IdP change.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`9beaf5f`](https://github.com/OpenNSW/one-trade-artifacts/commit/9beaf5f471a6c7bfefac4a018a0294ecc0c0eb21) (20 October 2026). It holds the artifact change this release depends on, [one-trade-artifacts#202](https://github.com/OpenNSW/one-trade-artifacts/pull/202): the agency reviews that map their reply as a `payload` input ([#804](https://github.com/OpenNSW/nsw-srilanka/pull/804)).

### Added

- **Reply payload.** A reply step (an `EXTERNAL_REVIEW` step with a `replyToken`) that has a `payload` input now sends only that input, plus its `callbackToken`, as `AUTH_API_CALL` sends its `payload` input. A `payload` that is not an object fails the step. Without a `payload` input, a reply sends all its inputs except `replyToken`, as before, and dispatches are unchanged. The pinned artifacts use it in the CDA, EFC, FCAU, NPQS and SLTB reviews that wait for a resubmission, so the officer sees the submission they reviewed and their own review form, read-only, next to the waiting status. ([#804](https://github.com/OpenNSW/nsw-srilanka/pull/804))

### Fixed

- The Trader Portal asks "Leave without saving?" before a form with unsaved answers is left, whether by in-app navigation, reload or closing the tab. Forms with a Save as Draft button also save a draft every 30 seconds while answers are unsaved. Forms without one get the warning but are not saved automatically. ([#801](https://github.com/OpenNSW/nsw-srilanka/pull/801))
- Keyboard and screen reader users can open a consignment from the Consignments list: the name and ID cell is now a link. ([#789](https://github.com/OpenNSW/nsw-srilanka/pull/789))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- Files of tasks started before their `render.json` declared `files` no longer open: those tasks hold raw storage keys, which are refused. ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628))
- On the CDA application form, an `.xls` upload to the DC bags field is named `DC BAGS DETAILS _1.xlsx.xls`, because the artifact's `x-file.fileName` ends in `.xlsx` while the field also accepts `.xls`. Only the displayed and downloaded name is affected. The fix is in the artifact, not in TNSW.

## [0.5.0] - 2026-10-19

This release lets a trader save an application as a draft: the form is saved without being submitted, and picked up again later with its values and files in place, so a long application no longer has to be finished in one sitting. It also enforces file tokens: a raw storage key no longer opens or submits a file, uploads return a token, and OGAs are sent tokens issued to their client instead of keys. Alongside these come certificate withdrawal on the IPPC ePhyto Hub, file links in Markdown, and a corrected CusDec valuation and remittance mapping. Deploy it with the pinned artifacts below: they add the Save as Draft buttons and declare every task's file fields, which the token enforcement depends on.

### Upgrade notes

#### File tokens are enforced ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628), [#627](https://github.com/OpenNSW/nsw-srilanka/pull/627), [#626](https://github.com/OpenNSW/nsw-srilanka/pull/626), [#697](https://github.com/OpenNSW/nsw-srilanka/pull/697))

- `GET /api/v1/storage/{key}` now answers 403 to anything but a token issued to the caller, raw keys included, for users and machine clients alike. Both task submit routes refuse a declared file field that holds anything but a token (400).
- Deploy with the artifacts pinned below, which declare each task's file fields ([one-trade-artifacts#143](https://github.com/OpenNSW/one-trade-artifacts/pull/143)). Tasks started before their `render.json` declared `files` still hold raw keys, so their files no longer open.
- In TNSW's `catalog.json`, every service an `EXTERNAL_REVIEW` step pushes files to needs its machine client under `clients`. Add `"efc": "EFC_TO_NSW"`, as [`catalog.example.json`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.5.0/configs/catalog.example.json) now does. A step that would send a file to a service with no client fails instead of sending it.
- Agencies running this codebase (CDA, EFC, FCAU, NPQS and SLTB) need no change: they store the tokens TNSW sends and pass them back. An OGA on its own legacy service now receives tokens where it received keys. It must open them on `GET /api/v1/storage/{token}` as the client its catalog entry names, such as `FCAU_TO_NSW`; the raw keys of older pushes are refused.
- `POST /api/v1/storage` returns a token as `key`, and no longer returns `id`. A user's upload token can only be submitted as a form's file value and does not expire; it can't open the file (403), so the file opens once the form is saved and the task is read again. A machine client's upload token can also open the file.
- File tokens issued before the upgrade carry no scopes. For up to `storage.tokenTTLSeconds` after it, opening or submitting one gets a 403 that tells the user to reload the task.

#### Accept XML uploads ([#811](https://github.com/OpenNSW/nsw-srilanka/pull/811))

- Add `text/xml` and `application/xml` to `storage.allowedUploadTypes` in TNSW's `config.yaml` (Helm: `backend.config.storage.allowedUploadTypes`), as [`values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.5.0/deployments/helm/values-example.yaml) does. The SLTB application in the pinned artifacts requires a draft CusDec `.xml` file ([one-trade-artifacts#204](https://github.com/OpenNSW/one-trade-artifacts/pull/204)), which is refused with a 415 without them.
- XML is accepted to download, not to display. The list is not read when `storage.type` is `proxy`, so agencies need no change.

#### CusDec valuation and remittance ([#700](https://github.com/OpenNSW/nsw-srilanka/pull/700))

- This version reads `valuation.totalAmountInvoiced` and `financial.remittanceCurrencyCode` from the CusDec form in [one-trade-artifacts#183](https://github.com/OpenNSW/one-trade-artifacts/pull/183), included in the pin below. With an older form, Box 22 goes out as 0 and the remittance has no currency.
- A CusDec draft saved on the old form needs both new fields filled in before it is submitted.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`a32169c`](https://github.com/OpenNSW/one-trade-artifacts/commit/a32169c35bff59b6c7f137e02d4dcd7c305bdcc1) (19 October 2026). It holds every artifact change this release depends on:
  - [one-trade-artifacts#143](https://github.com/OpenNSW/one-trade-artifacts/pull/143): each task's declared file fields, and `files:` links in Markdown ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628), [#629](https://github.com/OpenNSW/nsw-srilanka/pull/629));
  - [one-trade-artifacts#193](https://github.com/OpenNSW/one-trade-artifacts/pull/193): Save as Draft on application forms ([#707](https://github.com/OpenNSW/nsw-srilanka/pull/707));
  - [one-trade-artifacts#184](https://github.com/OpenNSW/one-trade-artifacts/pull/184): the NPQS certificate withdrawal ([#701](https://github.com/OpenNSW/nsw-srilanka/pull/701));
  - [one-trade-artifacts#183](https://github.com/OpenNSW/one-trade-artifacts/pull/183): the CusDec form ([#700](https://github.com/OpenNSW/nsw-srilanka/pull/700));
  - [one-trade-artifacts#204](https://github.com/OpenNSW/one-trade-artifacts/pull/204): the SLTB draft CusDec XML upload ([#811](https://github.com/OpenNSW/nsw-srilanka/pull/811)).
- Change the image version and `artifactLoader.github.ref` in the same deployment. A deployment that follows a moving ref such as `main` must deploy this release first, because the artifacts call routes and operations it adds.

### Added

- **Drafts.** A trader can save an application with Save as Draft and come back to it later; nothing is submitted until they submit. Any step can be saved this way on `PUT /api/v1/tasks/{id}/steps/{stepId}/draft`: its file fields are resolved as a submission's are, and the step and the task's version stay as they are. The pinned artifacts add the button to the application forms ([one-trade-artifacts#193](https://github.com/OpenNSW/one-trade-artifacts/pull/193)). ([#707](https://github.com/OpenNSW/nsw-srilanka/pull/707))
- **Certificate withdrawal on the IPPC ePhyto Hub.** The NPQS ePhyto step can withdraw a certificate it delivered, through a new `withdraw` operation. ([#701](https://github.com/OpenNSW/nsw-srilanka/pull/701))
- **File links in Markdown.** A Markdown template can link to one of the task's files as `[label](files:{{.namespace.field}})`, and the Trader Portal opens it through the storage route. ([#629](https://github.com/OpenNSW/nsw-srilanka/pull/629))

### Changed

- A reader of a task gets file tokens that can be submitted back only when they may act on its active step, by the step's `authz` rules. Anyone else gets tokens that only open the files. ([#698](https://github.com/OpenNSW/nsw-srilanka/pull/698))
- The CusDec sent to verify carries no files. ([#700](https://github.com/OpenNSW/nsw-srilanka/pull/700))

### Fixed

- CusDec Box 22 (total amount invoiced) and the remittance currency are mapped from the form. ([#700](https://github.com/OpenNSW/nsw-srilanka/pull/700))
- The Trader Portal shows "In Progress" in one color, and a task waiting for input as "In Progress" rather than "Pending User". ([#791](https://github.com/OpenNSW/nsw-srilanka/pull/791))

### Security

- A raw storage key no longer opens or submits a file: every file is reached through a token issued to its caller, with explicit scopes, `read` to open and `submit` to send it back. ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628), [#697](https://github.com/OpenNSW/nsw-srilanka/pull/697))
- OGAs receive tokens issued to their machine client that open a file but can't submit it, instead of storage keys. ([#626](https://github.com/OpenNSW/nsw-srilanka/pull/626))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- Files of tasks started before their `render.json` declared `files` no longer open: those tasks hold raw storage keys, which are refused. ([#628](https://github.com/OpenNSW/nsw-srilanka/pull/628))
- On the CDA application form, an `.xls` upload to the DC bags field is named `DC BAGS DETAILS _1.xlsx.xls`, because the artifact's `x-file.fileName` ends in `.xlsx` while the field also accepts `.xls`. Only the displayed and downloaded name is affected. The fix is in the artifact, not in TNSW.

## [0.4.0] - 2026-10-16

This release gives every deployment a file-token key set, delivers a trader's resubmission to the agency step that is waiting for it, and lets an agency catalog map roles besides `officer`. It also adds HTML document generation for workflows and reference formats for SLTB and NPQS, and stops checking a client ID allow list when it validates tokens. Every deployment needs one new secret, and TNSW's client for each agency needs one more scope.

### Upgrade notes

#### Set a file-token key set on every deployment ([#625](https://github.com/OpenNSW/nsw-srilanka/pull/625))

- Add `storage.tokenKeyset` to the `config.yaml` of TNSW and of each agency (CDA, EFC, FCAU, NPQS and SLTB). The server refuses to start without it, whatever `storage.type` is.
- The value is a comma-separated list of `kid:key` pairs, each key 32 random bytes in base64. Reference it as `"{{env:STORAGE_TOKEN_KEYSET}}"` and set `STORAGE_TOKEN_KEYSET=<kid>:$(openssl rand -base64 32)` from a Secret.
- Give every deployment a key set of its own. Never share one between deployments, and never use the development value in `.env.example`.
- With Helm, add a `storage-token-keyset` key to `nsw-secrets`, a `STORAGE_TOKEN_KEYSET` entry in `backend.env` that reads it, and `tokenKeyset` under `backend.config.storage`, as [`values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/deployments/helm/values-example.yaml) does.
- To rotate, put the new pair first and keep the old ones. Tokens given to agencies never expire, so dropping a key breaks the files they hold.
- `storage.tokenTTLSeconds` is optional. It sets how long a user's file token lasts, and defaults to 3600 (one hour).
- Local: add `STORAGE_TOKEN_KEYSET` to your `.env`, and copy `tokenKeyset` from the examples into your git-ignored `configs/config.yaml` and `configs/config.docker.yaml`. The committed agency configs and Compose already carry development keys.

#### Let TNSW reply to agencies ([#635](https://github.com/OpenNSW/nsw-srilanka/pull/635))

- In the IdP, give each client TNSW uses to call an agency (`NSW_TO_CDA`, `NSW_TO_EFC`, `NSW_TO_FCAU`, `NSW_TO_NPQS` and `NSW_TO_SLTB`) the `nsw:task:write` scope on that agency's own API, as well as `nsw:workflow:inject`. On a managed IdP, add it by hand to each client and its `NswTo<Agency>M2M` role. The local bootstrap in `idp/bootstrap/` grants it, and applies it on the next `docker compose up`.
- In TNSW's `services.json`, set each agency entry's `scopes` to `["nsw:workflow:inject", "nsw:task:write"]`, as `configs/services*.example.json` now do. That includes your git-ignored local copies.
- Deploy this release and the IdP change before the artifacts that use replies. Without the scope, an agency refuses TNSW's reply, and the step is retried until it is granted.

#### Token validation no longer checks client IDs ([#704](https://github.com/OpenNSW/nsw-srilanka/pull/704))

- `authn.clientIDs` is no longer read. Remove it from `config.yaml` (Helm: `backend.config.authn`) for TNSW and every agency. A key left in is ignored and does not stop the server.
- A token's audience alone now binds it to the service, so the API accepts a token from any client the IdP issues one to for the deployment's audience, not only the listed ones. In ThunderID, check that only the intended applications are granted access to the NSW API and to each agency API.

#### Agency catalogs ([#677](https://github.com/OpenNSW/nsw-srilanka/pull/677))

- In agency mode, every role in the agency's catalog now owns an injected workflow, not only `officer`, and `officer` is no longer required in the catalog. What a role may do comes from each task's own rules: a step with no `authz` block is not checked on writes, and a render config with no `read.roles` can be read by every catalog role.
- If your agency catalog maps roles besides `officer`, make sure each agency task declares who may act, as the agency folders of one-trade-artifacts do from [one-trade-artifacts#169](https://github.com/OpenNSW/one-trade-artifacts/pull/169). See "Roles per task" in [`docs/agency.md`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/docs/agency.md).

#### Reference formats for SLTB and NPQS ([#688](https://github.com/OpenNSW/nsw-srilanka/pull/688), [#687](https://github.com/OpenNSW/nsw-srilanka/pull/687))

Add these before you publish the SLTB and NPQS artifacts that use them. No migration is needed: they use the existing reference ID tables.

- **TNSW:** add the `SLTB` issuer (`blend_sheet_no`, `levy_payment_ref` and `lab_payment_ref`) and the `NPQS` issuer (`npqs_payment_ref`) to `refid` in `config.yaml` (Helm: `backend.config.refid`). Copy them from [`configs/config.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/configs/config.example.yaml) or [`values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/deployments/helm/values-example.yaml).
- **SLTB agency:** add the `refid` section of [`configs/agency/sltb/config.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/configs/agency/sltb/config.yaml). Its sample tasks draw each sample's reference from it, such as `2026/09/0001`.
- **NPQS agency:** add the `refid` section of [`configs/agency/npqs/config.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/configs/agency/npqs/config.yaml), which generates the application, lab sample, ePhyto and re-export certificate references. Deploy the NPQS agency and TNSW configurations together.
- **Local:** copy the new issuers into your git-ignored `configs/config*.yaml`.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`5d62d77`](https://github.com/OpenNSW/one-trade-artifacts/commit/5d62d77929fc343adeb08d1ae62d0591648fcc8b) (16 October 2026). It holds every artifact change this release depends on:
  - [one-trade-artifacts#146](https://github.com/OpenNSW/one-trade-artifacts/pull/146): the agency reviews that receive a trader's resubmission by reply token ([#635](https://github.com/OpenNSW/nsw-srilanka/pull/635));
  - [one-trade-artifacts#169](https://github.com/OpenNSW/one-trade-artifacts/pull/169): who may act on every agency task ([#677](https://github.com/OpenNSW/nsw-srilanka/pull/677));
  - [one-trade-artifacts#172](https://github.com/OpenNSW/one-trade-artifacts/pull/172) and [one-trade-artifacts#174](https://github.com/OpenNSW/one-trade-artifacts/pull/174): the NPQS and SLTB reference numbers ([#687](https://github.com/OpenNSW/nsw-srilanka/pull/687), [#688](https://github.com/OpenNSW/nsw-srilanka/pull/688));
  - [one-trade-artifacts#141](https://github.com/OpenNSW/one-trade-artifacts/pull/141): the boat note's printable document ([#581](https://github.com/OpenNSW/nsw-srilanka/pull/581)).

### Added

- **File tokens.** For tasks whose render config declares file fields, `GET /api/v1/tasks/{id}` returns a token in place of each stored file key. A token works only for the caller it was issued to (403 for anyone else) and only within `storage.tokenTTLSeconds` (410 after). `GET /api/v1/storage/{key}` and both task submit routes (`POST /api/v1/tasks/{id}/steps/{stepId}` and `POST /api/v1/callbacks/{token}`) take it back. The submit routes now read the task first, so a missing task is 404 and a step that is no longer active is 409. A value that isn't a token is still served and stored as a raw key, so nothing changes until the artifacts declare their files. ([#625](https://github.com/OpenNSW/nsw-srilanka/pull/625))
- **A trader's resubmission reaches the agency.** After an officer asks for more information, the resubmission used to be dropped, leaving TNSW waiting. Now only the first round goes through inject, and each side calls back on the other's waiting step by the token that step handed over. A reply to a step that is no longer waiting gets a 409 and is not retried: the step parks for an admin. Needs the IdP scope above and one-trade-artifacts#146. ([#635](https://github.com/OpenNSW/nsw-srilanka/pull/635))
- **Agency catalog roles.** An agency catalog may map extra roles: add the IdP role and a catalog entry. See the upgrade notes. ([#677](https://github.com/OpenNSW/nsw-srilanka/pull/677))
- **HTML documents.** A workflow can generate a print-ready document from an HTML template with the `HTML_DOCUMENT_GENERATOR` task plugin, such as the boat note. No config change: add an `html_template` and a subtask template that uses the plugin to the artifacts. ([#581](https://github.com/OpenNSW/nsw-srilanka/pull/581))
- **Reference numbers for SLTB and NPQS.** SLTB blend sheet numbers, sample references and payment references, and NPQS application, lab sample, certificate and payment references, are generated from `refid` formats. See the upgrade notes. ([#688](https://github.com/OpenNSW/nsw-srilanka/pull/688), [#687](https://github.com/OpenNSW/nsw-srilanka/pull/687))
- An onboarding guide for agencies that run in agency mode: [`docs/guides/onboarding-agency-mode-agencies.md`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.4.0/docs/guides/onboarding-agency-mode-agencies.md). ([#691](https://github.com/OpenNSW/nsw-srilanka/pull/691))

### Changed

- Token validation no longer checks a client ID allow list: the audience alone binds a token to the service. This is `core/authn` v0.4.0. See the upgrade notes. ([#704](https://github.com/OpenNSW/nsw-srilanka/pull/704))
- The Trader Portal's forms use `@opennsw/jsonforms-renderers` 0.13. Layout rules are now applied, so the SLTB final-decision officer form's Certificate Issuance group stays hidden until `approve` is chosen. Time pickers store `HH:MM:SS` instead of `HH:MM`, so check anything that parses `proposed_inspection_time` in the NPQS forms. Fields that are required only through a condition, and the rows of required arrays, show the required marker. Number inputs with a minimum block `-`, `+` and `e`. A file field's `x-file.fileName` is now used as the file's name. ([#702](https://github.com/OpenNSW/nsw-srilanka/pull/702))

### Fixed

- GovPay+ requests and webhooks are validated against the GovPay+ contract's field types and lengths. A reference number (`refNo`) may contain printable punctuation such as hyphens, up to 20 characters, and is kept exactly as sent, so the hyphenated NPQS payment references pass. A request that breaks the contract is now rejected: `transactionID` must be letters and digits (up to 20), `subinstId` and `serviceid` digits only (up to 3), and `serviceName` is required (up to 50 printable characters). ([#686](https://github.com/OpenNSW/nsw-srilanka/pull/686))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.
- File tokens are not enforced yet. The storage route and the task submit routes still accept a raw storage key, so a file can still be fetched by its key, not only by a token issued to the caller.
- On the CDA application form, an `.xls` upload to the DC bags field is named `DC BAGS DETAILS _1.xlsx.xls`, because the artifact's `x-file.fileName` ends in `.xlsx` while the field also accepts `.xls`. Only the displayed and downloaded name is affected. The fix is in the artifact, not in TNSW.

## [0.3.0] - 2026-10-14

This release runs each consignment as a case with several flows, moves workflow steps onto core's guarded task steps, and generates payment references from `refid` formats. Work in flight at the upgrade does not carry over, systems that review TNSW's tasks call back on a new route, and the Customs agency becomes the Export Facilitation Center (EFC). It also adds flows that traders start on a consignment (the first being the SLPA sundry payment), OpenTelemetry metrics, and NPQS, SLTB and FCAU as agencies in local Docker Compose.

### Upgrade notes

#### Drain in-flight work first ([#584](https://github.com/OpenNSW/nsw-srilanka/pull/584), [#643](https://github.com/OpenNSW/nsw-srilanka/pull/643), [#644](https://github.com/OpenNSW/nsw-srilanka/pull/644))

- Let open consignments, tasks and payments finish, or discard them, and stop running Temporal workflows before deploying. They are not migrated.
  - Existing task rows have no active step, and running workflows were started by 0.2.0's workflow code.
  - A top-level workflow started on 0.2.0 has no `case_flows` row, so its completion is rejected.
  - A payment started on 0.2.0 has no callback token, which is how a settled payment now completes its workflow step.
- Deploy `tnsw-api` and `tnsw-web` 0.3.0 together: the admin API's engine status and resolve routes moved, and only the 0.3.0 Trader Portal calls the new ones (see below).

#### Migrations

Six migrations run before the API starts; the Helm hook and Docker Compose run them. Use the 0.3.0 `tnsw-migrate` image.

- `000019_task_records_guarded_steps` ([#584](https://github.com/OpenNSW/nsw-srilanka/pull/584)) adds `parent_step_id`, `active_step_id` and `seq` to `task_records_v2`, and drops `parent_run_id`, `parent_node_id`, `task_run_id` and `subtask_node_id`. Rolling back brings those four columns back empty.
- `000020_agency_workflow_callback_token` ([#584](https://github.com/OpenNSW/nsw-srilanka/pull/584)) adds `agency_workflow.callback_token`.
- `000021_payment_callback_token` ([#584](https://github.com/OpenNSW/nsw-srilanka/pull/584)) adds `payment_transactions.callback_token` and drops `payment_transactions.task_id` with its index. Rolling back brings `task_id` back empty; the task ID is kept in the checkout metadata (`gateway_metadata.task_id`).
- `000022_create_case_flows` ([#642](https://github.com/OpenNSW/nsw-srilanka/pull/642)) creates `case_flows`.
- `000023_consignments_on_cases` ([#643](https://github.com/OpenNSW/nsw-srilanka/pull/643)) copies each consignment's name and state to a new `cases` row, making `INITIALIZED` `IN_PROGRESS`, makes `consignments.id` a foreign key to `cases.id`, and drops `consignments.name` and `state`. Rolling back copies them back, but an `INITIALIZED` consignment returns as `IN_PROGRESS`.
- `000024_task_records_case_id` ([#646](https://github.com/OpenNSW/nsw-srilanka/pull/646)) renames `task_records_v2.root_workflow_id` to `case_id`.

#### Reviewers call back on a callback token ([#584](https://github.com/OpenNSW/nsw-srilanka/pull/584))

Every system TNSW dispatches a task to (OGA portals, agencies) must change how it returns its decision:

- Store the `callbackToken` from each dispatch, and call back on `POST {serviceUrl}/{callbackToken}` (`/api/v1/callbacks/{token}`) instead of `/api/v1/tasks/{taskId}`.
- Take the new token on every re-dispatch, for example after an amendment.
- The body (`{command, payload}`) and the scope (`nsw:task:write`) are unchanged.

Agencies run from this repository do this from 0.3.0, so upgrade them together with TNSW.

#### Admin API routes ([#644](https://github.com/OpenNSW/nsw-srilanka/pull/644))

- `/api/v1/admin/consignments/{id}/engine-status` and `/api/v1/admin/consignments/{id}/steps/{stepId}/resolve` move to `/api/v1/admin/workflows/{id}/…`, addressed by the workflow ID, and the engine status's `consignment_id` becomes `workflow_id`. `GET /api/v1/admin/consignments/{id}` lists the consignment's `flows` with their workflow IDs. Update any client of the admin API other than the Trader Portal.

#### Payment references come from `refid` ([#632](https://github.com/OpenNSW/nsw-srilanka/pull/632))

- In `config.yaml` (Helm: `backend.config.refid`), define the `TNSW` issuer's `payment_ref` format and the `CDA` issuer's `fee_payment_ref` format. Copy both from [`configs/config.example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.3.0/configs/config.example.yaml) or [`deployments/helm/values-example.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.3.0/deployments/helm/values-example.yaml). Without them every payment fails with `refid: unknown issuer`.
- Set each CDA exporter's `data.agencies.cda.registration_number` to its 4-digit CDA Exporter Identification Number, such as `0002`. A CDA fee payment fails for an exporter without one, and for one still holding a value like `CDA-EXP-000002`.

#### Storage delete route removed ([#622](https://github.com/OpenNSW/nsw-srilanka/pull/622))

- `DELETE /api/v1/storage/{key}` now answers 405, and the `nsw:storage:delete` scope is gone.
- Remove `nsw:storage:delete` from any config that requests it, such as a portal's `IDP_SCOPES` or an M2M client's `scopes` in `services.json`, and then from the IdP. A token request naming a scope the IdP doesn't know may be refused.

#### Customs agency renamed to EFC ([#676](https://github.com/OpenNSW/nsw-srilanka/pull/676))

Deploy with workflow artifacts that include [one-trade-artifacts#164](https://github.com/OpenNSW/one-trade-artifacts/pull/164) (see below). TNSW's boat-note approval step then hands work to the `efc` service, and the agency loads `efc-v2/`.

1. **IdP:** create the EFC entities in place of the Customs ones, as declared in [`idp/bootstrap/20-agency-efc.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.3.0/idp/bootstrap/20-agency-efc.yaml), and give the `NSW_TO_EFC` and `EFC_TO_NSW` clients the old Customs clients' secrets or new ones. Then remove the Customs entities.
2. **TNSW `services.json`:** rename the `customs` entry to `efc`, with `client_id: NSW_TO_EFC`, `client_secret: env:M2M_NSW_TO_EFC_SECRET` and `endpoint_params.resource: https://api.efc.nsw-agency.local`.
3. **TNSW `config.yaml`:** in `authn.clientIDs`, replace `CUSTOMS_TO_NSW` with `EFC_TO_NSW`.
4. **Secrets:** rename `M2M_NSW_TO_CUSTOMS_SECRET` to `M2M_NSW_TO_EFC_SECRET`, `M2M_CUSTOMS_TO_NSW_SECRET` to `M2M_EFC_TO_NSW_SECRET`, and `CUSTOMS_ARTIFACT_LOCAL_ROOT` to `EFC_ARTIFACT_LOCAL_ROOT`. With Helm, rename the `nsw-secrets` key `m2m-customs-secret` to `m2m-efc-secret`, as `values-example.yaml` now references it.
5. **Agency deployment:** its config moves to `configs/agency/efc/`, with audience `https://api.efc.nsw-agency.local`, clients `OGA_PORTAL_APP_EFC` and `NSW_TO_EFC`, and artifacts `efc-v2`. The new database `efc_db` and Temporal namespace `efc` start empty, so boat-note reviews still open in `customs_db` are not carried over. Let them finish first, or point `db.name` and `temporal.namespace` at the old names.
6. **Officer portal:** set `IDP_CLIENT_ID` to `OGA_PORTAL_APP_EFC`, with `resource=https://api.efc.nsw-agency.local`. Officers need the `EFC Officer` role.

#### IdP provisioning ([#681](https://github.com/OpenNSW/nsw-srilanka/pull/681))

- `idp/sample-resources.sh`, `sample-resources.down.sh` and `idp/resources/` are replaced by the declarative documents in [`idp/bootstrap/`](https://github.com/OpenNSW/nsw-srilanka/tree/v0.3.0/idp/bootstrap). They carry local-development values: never mount them in UAT or production.
- There is no longer a script to provision a managed IdP. Create its resources in the ThunderID console or with `POST /import`, using the documents as the reference.
- The shared `AgencyM2M` role is replaced by one role per agency client, `<Agency>ToNswM2M`, with the same scopes. Nothing in TNSW checks the role name, so an existing IdP may keep `AgencyM2M`.

#### Workflow artifacts

- Pin `artifactLoader.github.ref` to one-trade-artifacts [`14e7d9e`](https://github.com/OpenNSW/one-trade-artifacts/commit/14e7d9e05c0b80fc0ef3811c65e3485d0765b20f) (14 October 2026). It holds every artifact change this release depends on:
  - [one-trade-artifacts#138](https://github.com/OpenNSW/one-trade-artifacts/pull/138): CusDec supporting documents as scanned and metadata lists, which the API now reads ([#590](https://github.com/OpenNSW/nsw-srilanka/pull/590));
  - [one-trade-artifacts#140](https://github.com/OpenNSW/one-trade-artifacts/pull/140): the CDA containerization date check ([#597](https://github.com/OpenNSW/nsw-srilanka/pull/597));
  - [one-trade-artifacts#142](https://github.com/OpenNSW/one-trade-artifacts/pull/142): the `input_mapping` every `BATCH_SPLIT` must now declare ([#648](https://github.com/OpenNSW/nsw-srilanka/pull/648));
  - [one-trade-artifacts#144](https://github.com/OpenNSW/one-trade-artifacts/pull/144): CDA application and certificate reference numbers ([#633](https://github.com/OpenNSW/nsw-srilanka/pull/633));
  - [one-trade-artifacts#164](https://github.com/OpenNSW/one-trade-artifacts/pull/164): the EFC rename ([#676](https://github.com/OpenNSW/nsw-srilanka/pull/676));
  - [one-trade-artifacts#163](https://github.com/OpenNSW/one-trade-artifacts/pull/163): the SLPA sundry payment flow ([#673](https://github.com/OpenNSW/nsw-srilanka/pull/673)).
- Before the CDA agency loads the #144 artifacts, add the `refid` section of [`configs/agency/cda/config.yaml`](https://github.com/OpenNSW/nsw-srilanka/blob/v0.3.0/configs/agency/cda/config.yaml) to its `config.yaml`, and run it on 0.3.0: its certificate formats use the new `param` segment ([#633](https://github.com/OpenNSW/nsw-srilanka/pull/633)).

#### CusDec assessments need SLC Edge v1.9 ([#591](https://github.com/OpenNSW/nsw-srilanka/pull/591))

- TNSW now reads the v1.9 `CUSDEC_INTEGRATED` assessment. A v1.7-shaped result (`taxes` / `amountToPay`) is still accepted, but its amount to pay reads as **0**. Upgrade only once Sri Lanka Customs' SLC Edge sends v1.9.

#### Local Docker Compose

- **Recreate the IdP volumes once** ([#681](https://github.com/OpenNSW/nsw-srilanka/pull/681)). An IdP seeded by `sample-resources.sh` holds the same entities under other IDs, so `thunderid-setup` fails and `thunderid` won't start. Run `docker compose down`, remove the `<project>_thunderid-db`, `<project>_thunderid-certs` and `<project>_thunderid-secrets` volumes, then `docker compose up`. This also moves the stack onto ThunderID 1.0.1 from a fresh database; `thunderid-db-init` seeds only an empty volume, so a kept 1.0.0-beta2 volume would be reused as is ([#684](https://github.com/OpenNSW/nsw-srilanka/pull/684)).
- **`idp/.env`:** `CORS_ORIGINS_*`, `*_REDIRECT_URIS`, `API_BASE`, `AUTH_TOKEN`, `INSECURE` and the per-user and per-client secret overrides are no longer read. Every M2M client's secret is `M2M_CLIENT_SECRET`, and each backend's `M2M_*_SECRET` must match it ([#681](https://github.com/OpenNSW/nsw-srilanka/pull/681)).
- **NPQS, SLTB and FCAU now run from this repository**, on ports 8081, 8086 and 8082. Stop nsw-agency's services on those ports. In your git-ignored `configs/services.json` and `services.docker.json`, give the `npqs`, `sltb` and `fcau` entries `"scopes": ["nsw:workflow:inject"]` and `"resource": "https://api.<agency>.nsw-agency.local"`, as in the examples, and rename `customs` to `efc` as above. Each agency reads `../one-trade-artifacts/<agency>-v2`; set `NPQS_ARTIFACT_LOCAL_ROOT`, `SLTB_ARTIFACT_LOCAL_ROOT` or `FCAU_ARTIFACT_LOCAL_ROOT` to use another clone ([#659](https://github.com/OpenNSW/nsw-srilanka/pull/659), [#663](https://github.com/OpenNSW/nsw-srilanka/pull/663), [#678](https://github.com/OpenNSW/nsw-srilanka/pull/678)).
- **Local config files:** copy the `refid` section of `configs/config.example.yaml` into your `configs/config*.yaml`. `make setup` doesn't overwrite an existing `configs/companies.json`, so copy the example's new keys into it, including the 4-digit CDA registration numbers ([#632](https://github.com/OpenNSW/nsw-srilanka/pull/632), [#542](https://github.com/OpenNSW/nsw-srilanka/pull/542)).

### Added

- **Flows a trader starts on a consignment.** A consignment is now a case that can run several flows besides the export flow. List the ones traders and CHAs may start under `consignment.flows` in `config.yaml`; each `templateId` must be a workflow in the artifacts. With none listed (the default), none can be started. ([#642](https://github.com/OpenNSW/nsw-srilanka/pull/642), [#643](https://github.com/OpenNSW/nsw-srilanka/pull/643), [#645](https://github.com/OpenNSW/nsw-srilanka/pull/645))
- **SLPA sundry payment.** Traders can raise a sundry order against a parent SLPA invoice, choosing from the sundry services SLPA offers. Offer it with `consignment.flows: [{templateId: slpa-sundry-payment, label: SLPA sundry payment}]`; it uses the existing `slpa` service and credentials. ([#673](https://github.com/OpenNSW/nsw-srilanka/pull/673), [#674](https://github.com/OpenNSW/nsw-srilanka/pull/674))
- **Metrics.** The API records the duration of every HTTP request and pushes OpenTelemetry metrics over OTLP/HTTP when `metrics.endpoint` and `metrics.serviceName` are set in `config.yaml`, with optional `metrics.headers` for auth. `endpoint` is a full URL including the path, such as `http://otel-collector:4318/v1/metrics`; a bare host stops the API at startup. Without the section, nothing is exported. ([#640](https://github.com/OpenNSW/nsw-srilanka/pull/640))
- The admin consignment view lists the consignment's flows, and the debugger opens each one. ([#644](https://github.com/OpenNSW/nsw-srilanka/pull/644))
- A `refid` format can embed a value the caller supplies, with a `param` segment and an optional `pattern`. CDA application and certificate reference numbers use it. ([#631](https://github.com/OpenNSW/nsw-srilanka/pull/631), [#633](https://github.com/OpenNSW/nsw-srilanka/pull/633))
- CusDec declarations send scanned supporting documents and metadata references separately, numbered as Sri Lanka Customs expects. Needs one-trade-artifacts#138. ([#590](https://github.com/OpenNSW/nsw-srilanka/pull/590))
- Local Docker Compose runs NPQS, SLTB and FCAU as agencies, alongside CDA and EFC. ([#659](https://github.com/OpenNSW/nsw-srilanka/pull/659), [#663](https://github.com/OpenNSW/nsw-srilanka/pull/663), [#678](https://github.com/OpenNSW/nsw-srilanka/pull/678))

### Changed

- Payment references are generated from `refid` formats: `TNSW` followed by eight random characters, such as `TNSW7K2QQXAB`, and for a CDA fee the exporter's registration number, the fee's categories and a sequence, such as `000201000020001`. ([#632](https://github.com/OpenNSW/nsw-srilanka/pull/632))
- A consignment's state is `IN_PROGRESS` or `FINISHED`; `INITIALIZED` is gone. A consignment finishes once all its flows have, and starting a new flow reopens it. ([#643](https://github.com/OpenNSW/nsw-srilanka/pull/643))
- The Customs agency is now the Export Facilitation Center (EFC). ([#676](https://github.com/OpenNSW/nsw-srilanka/pull/676))
- The local IdP is set up from declarative documents in `idp/bootstrap/` instead of a seed script. ([#681](https://github.com/OpenNSW/nsw-srilanka/pull/681))
- Local Docker Compose runs ThunderID 1.0.1 instead of 1.0.0-beta2. ([#684](https://github.com/OpenNSW/nsw-srilanka/pull/684))

### Fixed

- The CDA application form checks that the containerization dates are in order. Needs one-trade-artifacts#140. ([#597](https://github.com/OpenNSW/nsw-srilanka/pull/597))
- The amount to pay for a CusDec is read from SLC Edge's v1.9 assessment. See the upgrade notes. ([#591](https://github.com/OpenNSW/nsw-srilanka/pull/591))

### Security

- The unused storage delete route is removed, with the `nsw:storage:delete` scope. Every agency's M2M client held that scope, so any of them could delete a file whose key it had. ([#622](https://github.com/OpenNSW/nsw-srilanka/pull/622))
- `golang.org/x/net` is raised to v0.60.0, through OpenNSW/core. ([#648](https://github.com/OpenNSW/nsw-srilanka/pull/648))

### Known issues

- Workflow definitions are not versioned with the release. Pinning `artifactLoader.github.ref` is what ties them to a deployment. They are fetched from GitHub each time they are used, so GitHub is a run-time dependency.
- Audit logging drops events it cannot deliver.
  - With no `audit` section in `config.yaml`, the API runs without audit.
  - While Argus is unreachable, events are retried, then dropped with an error in the log.
  - Once the in-memory queue of 100 events is full, requests that record audit events wait for space, so an Argus outage can slow consignment and task requests.
- The API connects to Temporal without TLS or an API key, so Temporal must be reachable over a private network.
- `/health` does not check Temporal.
- The Helm chart has no persistent volume, so local-disk storage is lost when the pod restarts. Use S3.
- The repository has no tooling to provision a managed IdP; it is set up by hand from `idp/bootstrap/`.
- The Trader Portal is available in English only.
- With the NSW Admin role selected, the Trader Portal's Consignments list does not load: the API accepts only the trader and CHA roles there. Admins open a consignment directly, at `/admin/consignments/<consignment ID>`.

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

[Unreleased]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.13.0...HEAD
[0.13.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/OpenNSW/nsw-srilanka/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/OpenNSW/nsw-srilanka/releases/tag/v0.1.0
