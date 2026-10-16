# Identity Provider (IdP) Setup

## Overview

We use [ThunderID](https://thunderid.dev/) (`ghcr.io/thunder-id/thunderid`) as the
Identity Provider for this project — a lightweight, developer-friendly identity and
access management solution. This directory runs the stock ThunderID image and layers
on the project's resources, declared in [`bootstrap/`](bootstrap/).

> ThunderID is the renamed successor of Asgardeo Thunder (`asgardeo/thunder`). The
> binary, image, and install paths moved from `thunder` / `/opt/thunder` to
> `thunderid` / `/opt/thunderid` (rename landed in v0.37.0).

> **Pinned version: `1.0.1`** (`compose.yml`). Note the OCI tag has no `v`
> prefix even though the upstream git tag does. 1.0.0 changed enough to be worth
> knowing about if you are reading older notes:
>
> - The distribution layout flattened: `deployment.yaml` sits at the install root,
>   the SQLite files under `database/`, key material under `config/certs/`.
> - Key material is no longer baked into the image — `setup.sh` generates it per
>   deployment, which is why `thunderid-certs` / `thunderid-secrets` volumes exist.
> - Bootstrap resources are declarative YAML, not shell scripts.
> - CORS moved out of `deployment.yaml` into a runtime `server_config` resource.
> - Token audiences bind to exactly one resource server (see _API authorization_).

## Getting Started

### Quick Start (with defaults)

Start the IdP with default credentials (`admin` / `1234`):

```bash
docker compose up
```

A full `docker compose up` runs three IdP services in order:

1. **`thunderid-db-init`** — seeds the shared SQLite databases from the image.
2. **`thunderid-setup`** — one-shot container that generates this deployment's key
   material (TLS cert, JWT signing keys, the AES key that encrypts stored client
   secrets, and the Direct Auth Secret), then runs the in-process bootstrap over the
   declarative documents in `/opt/thunderid/bootstrap/` — the image's own defaults
   plus every document in [`bootstrap/`](bootstrap/) — and exits.
3. **`thunderid`** — the long-running server (listens on `https://localhost:8090`).

Everything the platform needs locally (OUs, user types, resource servers, roles,
groups, portals, M2M clients, sample users) exists once `thunderid-setup` finishes.

### Custom Configuration (optional)

1. Copy the example environment file:

   ```bash
   cp .env.example .env
   ```

2. Edit `.env`. Note that variable names are **unprefixed** (the `THUNDER_` prefix
   was dropped in the migration to ThunderID):

   ```bash
   ADMIN_USERNAME=admin
   ADMIN_PASSWORD=your-secure-password
   PUBLIC_URL=https://localhost:8090
   PORT=8090
   ```

   `ADMIN_PASSWORD` is load-bearing, not cosmetic: the bootstrap has no default for
   it. Leave it unset and `setup.sh` generates one and prints it once — recover it
   with `docker compose logs thunderid-setup`.

   Both `deployment.yaml` and the declarative documents under `bootstrap/` are
   templated from these vars — `{{.PUBLIC_URL}}` ← `PUBLIC_URL`. Two things to watch:

   - An unset variable is a **hard error**, so every placeholder needs a value.
   - Substitution runs over the whole file including comments, so never write
     placeholder syntax in a comment unless that variable is genuinely set.

   The CORS allow-list is the `cors` server config in
   [`bootstrap/10-nsw-platform.yaml`](bootstrap/10-nsw-platform.yaml), not an env var.

3. Start the IdP:

   ```bash
   docker compose up
   ```

### Developer Console Access

Once running, open the developer console at `https://localhost:8090/console`:

- **Credentials**: the `ADMIN_USERNAME` / `ADMIN_PASSWORD` values from your `.env`
  (`idp/.env.example` ships `admin` / `1234` for local dev)
- If `ADMIN_PASSWORD` is unset, `setup.sh` generates one and prints it **once** —
  recover it with `docker compose logs thunderid-setup`. It is regenerated on every
  re-run, so set it explicitly if you want a stable password.

> ⚠️ **Security Warning**: change the default password for any non-local environment.

## Bootstrap Resources

`thunderid-setup` runs the server's **in-process bootstrap**, which reads every
`.yaml` / `.yml` / `.json` file under `/opt/thunderid/bootstrap` recursively, in
sorted filename order, and applies them as a single declarative import. The image
ships `01-default-resources.yaml` and `02-server-configurations.yaml`; `compose.yml`
mounts our [`bootstrap/`](bootstrap/) directory beside them, as
`/opt/thunderid/bootstrap/nsw/`.

> **Changed in 1.0.0.** This used to be numbered _shell scripts_ sourcing the image's
> `common.sh`. That mechanism is gone: a `.sh` file in this directory is now **skipped
> silently** — no error, no log line. If you are porting an old bootstrap script, it
> has to become a declarative document. Import runs with `upsert=true` (re-running is
> idempotent) and `continueOnError=false` (a bad document fails the whole stage).

- **`01-default-resources.yaml`** (image default, not overridden) — default OU,
  `Person` user type, default agent type, admin user, System resource server +
  permissions, `Administrators` group, `Administrator` role, the default flows (all
  with handle `default-flow`, distinguished by `flowType`), the `Console` application,
  six themes, and i18n translations.
- **`02-server-configurations.yaml`** (image default) — server-level flow defaults.

Ours, all **local-dev only** (see _UAT / production_ below):

| File                                                             | Contents                                                                                                                                                         |
| ---------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`03-nsw-resources.yaml`](bootstrap/03-nsw-resources.yaml)       | The `admin-cli` machine client (`ADMIN_CLI`, secret `ADMIN_CLI_SECRET`) and the `NSW Admin CLI` role granting it `system`, for a management token                |
| [`10-nsw-platform.yaml`](bootstrap/10-nsw-platform.yaml)         | User types, the Government Organization and Private Sector OUs, NSW API, `Trader`/`CHA`/`NSW Admin` roles and groups, `TraderApp`, SLCE and GovPay clients, CORS |
| `20-agency-<handle>.yaml` (`cda`, `efc`, `fcau`, `npqs`, `sltb`) | One agency each: its OU and API resource server, `<A> Officer` role and `<A> Officers` group, portal app, `<A>_TO_NSW` and `NSW_TO_<A>` clients and roles        |
| [`20-agency-slpa.yaml`](bootstrap/20-agency-slpa.yaml)           | SLPA, still on the legacy shared `Agency API` resource server with `OGA Reviewer(s)` and `NswM2M`                                                                |
| [`90-sample-data.yaml`](bootstrap/90-sample-data.yaml)           | Sample company OUs (ADAM, EDWARD), the sample users, and their group memberships                                                                                 |

Each file names what it needs: the platform file references nothing of ours, each agency
file needs only the platform, and the sample data needs everything. So a deployment can
mount the platform with any subset of agencies.

`admin-cli` grants management scope from a file-supplied secret, so never mount
`03-nsw-resources.yaml` into a shared or UAT/prod deployment. Its role is a _new_ role
rather than a re-declaration of the built-in `Administrator` — see the rules below.

### Writing bootstrap documents

The format is ThunderID's declarative resource format: one document per resource,
`---`-separated, each with a `resource_type`. What the 1.0.0-beta2 importer does, as
tested against this image:

- **Placeholders.** `{{ .NAME }}` resolves from `idp/.env` (the `env_file` in `compose.yml`)
  before the YAML is parsed, comments included, so never write placeholder syntax in a
  comment. An unset variable fails the whole bootstrap, the image's own documents
  included. A `{{- range .NAME}}` list reads `NAME_0`, `NAME_1`, … The documents use only
  `M2M_CLIENT_SECRET` and `SAMPLE_USER_PASSWORD`.
- **References are by ID.** A child OU's `parent`, a role's `resourceServerId`, role
  `assignments` and group `members` all take IDs. Only the owning OU can be named by
  handle (`ouHandle`), and auth flows by `*FlowHandle`. `parentHandle` is **ignored
  without error**: the OU lands at the root.
- **Resource order.** In a resource server, list a parent resource before its children,
  or roles granting the children's permissions fail with `ROL-1012 Invalid permissions`.
- **Re-declaring a resource** (same `id`, in a later file) is how sample data adds group
  members without the platform file depending on it. Group `members` and role
  `assignments` **merge**; but a role re-declared **without** `permissions` loses them.
  Never re-declare a role without its full permissions — which is why `admin-cli` gets
  its own `NSW Admin CLI` role instead of an assignment on `Administrator`.
- **Users.** Set a password under `credentials`, as the image does for `admin`. In
  `attributes` it works once, then fails the next run with `USR-1028 Credential update
not allowed`, because `thunderid-setup` reruns the import on every `docker compose up`.
  A `credentials` password does not satisfy a user type's `required` check, so our user
  types declare `password` as `required: false` (still `credential: true`).

### Changing resources

Edit the documents and recreate the IdP volumes, so the bootstrap runs on a fresh
database:

```bash
docker compose down
docker volume rm "$(basename "$PWD")_thunderid-db" "$(basename "$PWD")_thunderid-certs" "$(basename "$PWD")_thunderid-secrets"
docker compose up
```

An upsert updates what the documents still declare, so a plain `docker compose up`
applies additions and edits; it never removes what a document no longer declares.

A console export (`POST /export`) is in this same format and a useful starting point,
but it also carries the image's own resources (IDs `01900000-…`), random IDs, and users
without passwords; strip and fix those before committing.

To add an agency run by this backend in agency mode, copy one of the
`20-agency-*.yaml` files, change the handle, names and port, and give every document a
new `id` (and update the references to it). Then add its port to the `cors` server
config, and its clients to the backends' catalog and `services.json`.

### Management token (local dev)

The management API, the console's API, and `POST /import` need a `system`-scoped
token. Locally, mint one from `admin-cli`:

```bash
TOKEN=$(curl -k -s -u "ADMIN_CLI:${ADMIN_CLI_SECRET:-1234}" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" -d "scope=system" \
  -d "resource=https://localhost:8090/mcp" \
  https://localhost:8090/oauth2/token | jq -r .access_token)
```

The `resource` parameter is **required** here: `system` is a permission scope, so the
token must bind to a resource server, and this is the identifier of the System
resource server the image bootstraps. Omit it and the request fails with
`invalid_target`, since this deployment configures no `defaultResourceServer` (see
_API authorization_ below).

### UAT / production

The documents in `bootstrap/` are local-dev values: fixed client IDs, one shared client
secret, sample users, and localhost URIs. Do not mount them into a shared deployment.

Applying the NSW and agency resources to a UAT or production IdP is tracked in
[#679](https://github.com/OpenNSW/nsw-srilanka/issues/679). Until then, deploy a stock
ThunderID (the image's own bootstrap only, with a strong `ADMIN_PASSWORD`) and create the
resources in the console, or with `POST /import` and an admin token, using these
documents as the reference. For an agency-mode agency,
[the agency onboarding guide](../docs/guides/onboarding-agency-mode-agencies.md#step-2-set-up-the-agency-in-thunderid)
walks through creating its resources in the console.

## Applications created

| App           | Client ID             | Local URL               |
| ------------- | --------------------- | ----------------------- |
| TraderApp     | `TRADER_PORTAL_APP`   | <http://localhost:5173> |
| NPQSPortalApp | `OGA_PORTAL_APP_NPQS` | <http://localhost:5174> |
| FCAUPortalApp | `OGA_PORTAL_APP_FCAU` | <http://localhost:5175> |
| CDAPortalApp  | `OGA_PORTAL_APP_CDA`  | <http://localhost:5176> |
| SLPAPortalApp | `OGA_PORTAL_APP_SLPA` | <http://localhost:5177> |
| EfcPortalApp  | `OGA_PORTAL_APP_EFC`  | <http://localhost:5178> |
| SLTBPortalApp | `OGA_PORTAL_APP_SLTB` | <http://localhost:5179> |

M2M (client-credentials) apps (auth method: `client_secret_basic`):

- **OGA → NSW** (`aud=NSW_API`, each with its own `<Agency>ToNswM2M` role, e.g.
  `CdaToNswM2M`): `NPQS_TO_NSW`, `FCAU_TO_NSW`, `CDA_TO_NSW`, `SLPA_TO_NSW`,
  `EFC_TO_NSW`, `SLTB_TO_NSW`.
- **NSW → OGA** (`aud=AGENCY_API`, `NswM2M` role): `NSW_TO_SLPA`.
- **NSW → CDA** (`aud=https://api.cda.nsw-agency.local`, `NswToCdaM2M` role): `NSW_TO_CDA`.
- **NSW → EFC** (`aud=https://api.efc.nsw-agency.local`, `NswToEfcM2M` role): `NSW_TO_EFC`.
- **NSW → NPQS** (`aud=https://api.npqs.nsw-agency.local`, `NswToNpqsM2M` role): `NSW_TO_NPQS`.
- **NSW → SLTB** (`aud=https://api.sltb.nsw-agency.local`, `NswToSltbM2M` role): `NSW_TO_SLTB`.
- **NSW → FCAU** (`aud=https://api.fcau.nsw-agency.local`, `NswToFcauM2M` role): `NSW_TO_FCAU`.

## API authorization (OAuth2)

Each protected backend is registered as a **resource server** whose `identifier`
becomes the access-token **audience** (`aud`):

| `identifier` (= token `aud`)        | Backend                                                                | Scopes (`<resource>:<action>`)                                                                                                            |
| ----------------------------------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `https://api.nsw-srilanka.local`    | [OpenNSW/nsw](https://github.com/OpenNSW/nsw) `backend/`               | `nsw:consignment:{read,write,adminread,adminwrite}`, `nsw:task:{read,write}`, `nsw:{hscode,company,cha}:read`, `nsw:storage:{read,write}` |
| `https://api.nsw-agency.local`      | [OpenNSW/nsw-agency](https://github.com/OpenNSW/nsw-agency) `backend/` | `agency:application:{read,review,feedback,inject}`, `agency:consignment:read`, `agency:storage:{read,write}`                              |
| `https://api.cda.nsw-agency.local`  | this backend as CDA (`mode: agency`, `cda-api` in `compose.yml`)       | `nsw:workflow:inject`, `nsw:consignment:read`, `nsw:task:{read,write}`, `nsw:profile:read`, `nsw:storage:{read,write}`                    |
| `https://api.efc.nsw-agency.local`  | this backend as EFC (`mode: agency`, `efc-api` in `compose.yml`)       | `nsw:workflow:inject`, `nsw:consignment:read`, `nsw:task:{read,write}`, `nsw:profile:read`, `nsw:storage:{read,write}`                    |
| `https://api.npqs.nsw-agency.local` | this backend as NPQS (`mode: agency`, `npqs-api` in `compose.yml`)     | `nsw:workflow:inject`, `nsw:consignment:read`, `nsw:task:{read,write}`, `nsw:profile:read`, `nsw:storage:{read,write}`                    |
| `https://api.sltb.nsw-agency.local` | this backend as SLTB (`mode: agency`, `sltb-api` in `compose.yml`)     | `nsw:workflow:inject`, `nsw:consignment:read`, `nsw:task:{read,write}`, `nsw:profile:read`, `nsw:storage:{read,write}`                    |
| `https://api.fcau.nsw-agency.local` | this backend as FCAU (`mode: agency`, `fcau-api` in `compose.yml`)     | `nsw:workflow:inject`, `nsw:consignment:read`, `nsw:task:{read,write}`, `nsw:profile:read`, `nsw:storage:{read,write}`                    |

> **Identifiers must be absolute URIs, and they are opaque** — nothing ever
> dereferences them; they exist to be matched and to be written into `aud`. The URI
> requirement is not cosmetic: see _How a token gets its audience_ below. The identifier
> is what callers send as `resource`, so changing one means changing them too.
>
> `authn.audience` in each backend's `config.yaml` must equal its own resource server's identifier:
>
> - **TNSW** (`api`): the `NSW_API` identifier, `https://api.nsw-srilanka.local`. It is
>   set in `configs/config.example.yaml`, `configs/config.docker.example.yaml` and
>   `deployments/helm/values-example.yaml`. Changing the identifier means changing all
>   three (and any existing local `configs/config*.yaml`).
> - **CDA** (`cda-api`): the `CDA API` identifier, `https://api.cda.nsw-agency.local`. It
>   is set in `configs/agency/cda/config.yaml` and `configs/agency/cda/config.docker.yaml`. Its
>   callers request it as `resource=`: the CDA portal in
>   `configs/agency/cda/portal-config.js`, and TNSW's `cda` entry in
>   `configs/services*.example.json`. Changing the identifier means changing all of these
>   (and any existing local `configs/services*.json`).
> - **EFC** (`efc-api`): the `EFC API` identifier,
>   `https://api.efc.nsw-agency.local`, in the same places under
>   `configs/agency/efc/` and TNSW's `efc` entry in `configs/services*.example.json`.
> - **NPQS** (`npqs-api`): the `NPQS API` identifier, `https://api.npqs.nsw-agency.local`,
>   in the same places under `configs/agency/npqs/` and TNSW's `npqs` entry in
>   `configs/services*.example.json`.
> - **SLTB** (`sltb-api`): the `SLTB API` identifier, `https://api.sltb.nsw-agency.local`,
>   in the same places under `configs/agency/sltb/` and TNSW's `sltb` entry in
>   `configs/services*.example.json`.
> - **FCAU** (`fcau-api`): the `FCAU API` identifier, `https://api.fcau.nsw-agency.local`,
>   in the same places under `configs/agency/fcau/` and TNSW's `fcau` entry in
>   `configs/services*.example.json`.

Scopes are namespaced (`nsw:*` / `agency:*`). The CDA, EFC, NPQS, SLTB and FCAU APIs reuse `nsw:*` names, since they
are the same backend; that is safe because every caller names its target with `resource`
(below), and a token is narrowed to the scopes of that one resource server.

**How a token gets its scopes.** Scopes come from a **role grant on the principal**,
not from the app's requestable `scopes` list. So every caller is granted the relevant
scopes via a role:

| Caller                 | Grant                                                                                                         |
| ---------------------- | ------------------------------------------------------------------------------------------------------------- |
| TraderApp users        | `Trader` / `CHA` role (via group) → `NSW_API` scopes                                                          |
| NSW admin users        | `NSW Admin` role (via `NSW Admins` group) → `nsw:consignment:adminread`, `nsw:consignment:adminwrite`         |
| `*_TO_NSW` M2M clients | **`<Agency>ToNswM2M` role assigned to the application** (`type: app`) → `NSW_API` scopes                      |
| OGA portal users       | `OGA Reviewer` role (via `OGA Reviewers` group) → `AGENCY_API` scopes                                         |
| `NSW_TO_*` M2M clients | **`NswM2M` role assigned to the application** (`type: app`) → `agency:application:inject`                     |
| CDA portal users       | `CDA Officer` role (via `CDA Officers` group) → CDA API scopes                                                |
| `NSW_TO_CDA`           | **`NswToCdaM2M` role assigned to the application** → `nsw:workflow:inject`, `nsw:task:write` on the CDA API   |
| EFC portal users       | `EFC Officer` role (via `EFC Officers` group) → EFC API scopes                                                |
| `NSW_TO_EFC`           | **`NswToEfcM2M` role assigned to the application** → `nsw:workflow:inject`, `nsw:task:write` on the EFC API   |
| NPQS portal users      | `NPQS Officer` role (via `NPQS Officers` group) → NPQS API scopes                                             |
| SLTB portal users      | `SLTB Officer` role (via `SLTB Officers` group) → SLTB API scopes                                             |
| `NSW_TO_NPQS`          | **`NswToNpqsM2M` role assigned to the application** → `nsw:workflow:inject`, `nsw:task:write` on the NPQS API |
| `NSW_TO_SLTB`          | **`NswToSltbM2M` role assigned to the application** → `nsw:workflow:inject`, `nsw:task:write` on the SLTB API |
| FCAU portal users      | `FCAU Officer` role (via `FCAU Officers` group) → FCAU API scopes                                             |
| `NSW_TO_FCAU`          | **`NswToFcauM2M` role assigned to the application** → `nsw:workflow:inject`, `nsw:task:write` on the FCAU API |

**How a token gets its audience — changed substantially in 1.0.0.** Pre-1.0.0 the
server inferred the audience by reverse-mapping the granted permission scopes back to
the resource servers that own them. That is gone. An access token now binds to
**exactly one** resource server, resolved in this order:

1. the RFC 8707 `resource` parameter on the request — it must be a single **absolute
   URI** with no fragment, matching a registered resource server's `identifier`;
2. otherwise a deployment-wide `defaultResourceServer` server-config — **which this
   deployment deliberately does not set** (see below);
3. if the request carries any permission scope and neither resolves, it is **rejected
   with `invalid_target`** — not merely issued without scopes.

The bound resource server's `identifier` becomes `aud`, and non-OIDC scopes are
narrowed to the permissions that resource server actually defines. OIDC scopes
(`openid`, `profile`, `email`, `group`, `ou`, `role`) are retained unchanged. A request
carrying no permission scope at all is unbound, and takes the application's
`token.accessToken.defaultAudience`, falling back to `client_id`.

### Every caller states its target explicitly

We do **not** configure `defaultResourceServer`. There is only one, deployment-wide, and
this platform has two APIs — whichever became the default, callers of the other would
silently lose their scopes. Rather than make one side implicit and the other explicit, no
caller is implicit.

The payoff is the failure mode. With a default configured, a caller that forgets
`resource` gets **HTTP 200 and a token whose scopes have been quietly stripped** — it
looks like a broken app registration and is genuinely hard to trace. With no default it
gets a hard `invalid_target` at the token endpoint, naming the problem.

> ⚠️ **Do not accept the console's offer to set a default.** Since 1.0.0-beta2 the
> resource-server creation wizard shows a _pre-ticked_ "Make this the default resource
> server" checkbox whenever the deployment has no default — which is precisely our
> configuration — and the resource-server list offers the same action. Accepting it flips
> the failure mode above for **every** caller in the deployment, silently: requests that
> should fail with `invalid_target` start returning HTTP 200 with a scopeless token
> instead. Untick it. The bootstrap is unaffected — it never sets a default.

So every permission-bearing caller sends `resource`:

| Caller                       | Sends `resource` | Where it is configured                              | Token `aud`                         |
| ---------------------------- | ---------------- | --------------------------------------------------- | ----------------------------------- |
| TraderApp users              | NSW_API          | `IDP_EXTRA_QUERY_PARAMS` (SPA `extraQueryParams`)   | `https://api.nsw-srilanka.local`    |
| OGA portal users             | AGENCY_API       | `VITE_IDP_EXTRA_QUERY_PARAMS` in OpenNSW/nsw-agency | `https://api.nsw-agency.local`      |
| `*_TO_NSW` M2M               | NSW_API          | `NSW_TOKEN_PARAMS` in OpenNSW/nsw-agency            | `https://api.nsw-srilanka.local`    |
| `NSW_TO_*` M2M               | AGENCY_API       | `endpoint_params` in `configs/services*.json`       | `https://api.nsw-agency.local`      |
| `SLCE_TO_NSW` webhook        | NSW_API          | ⚠️ external — see below                             | `https://api.nsw-srilanka.local`    |
| `admin-cli` management token | System RS        | _Management token_ above                            | `https://localhost:8090/mcp`        |
| `customs-asycuda`            | n/a              | requests no scopes, so stays unbound                | app `defaultAudience` / `client_id` |

**SPAs** send it on `/authorize` via oidc-client-ts `extraQueryParams`. That covers the
whole session: the IdP records the value on the authorization code and reuses the bound
audience on refresh, so `automaticSilentRenew` needs no extra handling.

**M2M clients** send it in the token request body, as RFC 6749 §3.2 requires. In
`configs/services*.json` that is the `endpoint_params` field of
`github.com/OpenNSW/core/remote` (v0.6.0+); nsw-agency uses `NSW_TOKEN_PARAMS`, which it
passes to `clientcredentials.Config.EndpointParams`. Both take the same query-string shape
as the SPA variable, so one syntax covers every caller:

```json
"token_url": "https://thunderid:8090/oauth2/token",
"endpoint_params": { "resource": "https://api.nsw-agency.local" }
```

> Earlier revisions appended `?resource=…` to `token_url` instead. That worked only
> because the token endpoint calls `r.ParseForm()`, which merges the URL query into the
> form — an implementation detail, not a contract. Do not reintroduce it.

> ⚠️ **`SLCE_TO_NSW` is an external integration.** Sri Lanka Customs' SLCE system calls
> us with `client_credentials` for `nsw:slce-webhooks:write`. With no default configured
> it must send `resource=https://api.nsw-srilanka.local` too, which is a coordination
> task outside this repo. Until it does, its token requests fail with `invalid_target`.
> This is the one place where the no-default choice costs us something a default would
> have absorbed.

> **Identifiers must be absolute URIs — including when they are only ever resolved
> internally.** At `/authorize` the server rewrites the request's resource to the
> resolved identifier and persists it on the authorization code; the token exchange reads
> it back and re-validates it as a `resource` parameter. So a bare name like `NSW_API`
> fails with `invalid_target: must be an absolute URI` at the _token_ endpoint.
> `client_credentials` is unaffected (no authorization code, so no round-trip), which
> makes this look like a portal bug when it is really an identifier-format constraint.

## Notes

- The developer console and login screens show the stock **ThunderID** branding (the
  product name is `brand.product_name` in the image's `apps/{console,gate}/config.js`,
  not an env var or API — left at the image default).
- Data and key material live in three Docker volumes: `thunderid-db` (the SQLite
  files), `thunderid-certs` and `thunderid-secrets` (generated by `thunderid-setup`
  and read-only at runtime). `consent-db` is gone — the standalone consent server was
  removed in 1.0.0. To reset, `docker compose down -v` and `up` again.

  > **Upgrading from 0.42.0 requires `docker compose down -v` — this is a hard gate,
  > not advice.** There is no migration tooling upstream, and the schema moved
  > underneath the same filenames: `runtimedb`'s tables collapsed into a single
  > `RUNTIME_STORE`, `runtime_persistent.db` is new, `userdb` became `entitydb`, and a
  > stale `configdb.db` keeps its name so it _is_ opened — just without the tables
  > 1.0.0 expects. The old certs must go too: they were baked into the 0.42.0 image and
  > are now generated per deployment.

- Role assignment is **group-based**: users inherit effective roles from group
  membership (`Traders` → `Trader`, `CHA` → `CHA`).
