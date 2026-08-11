# Identity Provider (IdP) Setup

## Overview

We use [ThunderID](https://thunderid.dev/) (`ghcr.io/thunder-id/thunderid`) as the
Identity Provider for this project — a lightweight, developer-friendly identity and
access management solution. This directory runs the stock ThunderID image and layers
on the project's sample resources.

> ThunderID is the renamed successor of Asgardeo Thunder (`asgardeo/thunder`). The
> binary, image, and install paths moved from `thunder` / `/opt/thunder` to
> `thunderid` / `/opt/thunderid` (rename landed in v0.37.0).

> **Pinned version: `1.0.0-beta2`** (`compose.yml`). Note the OCI tag has no `v`
> prefix even though the upstream git tag does. 1.0.0 changed enough to be worth
> knowing about if you are reading older notes:
>
> - The distribution layout flattened: `deployment.yaml` sits at the install root,
>   the SQLite files under `database/`, key material under `config/certs/`.
> - Key material is no longer baked into the image — `setup.sh` generates it per
>   deployment, which is why `thunderid-certs` / `thunderid-secrets` volumes exist.
> - Bootstrap resources are declarative YAML, not shell scripts.
> - CORS moved out of `deployment.yaml` into a runtime `server_config` resource.
> - Token audiences bind to exactly one resource server (see *API authorization*).

## Getting Started

### Quick Start (with defaults)

Start the IdP with default credentials (`admin` / `1234`):

```bash
docker compose up
```

A full `docker compose up` runs four IdP services in order:

1. **`thunderid-db-init`** — seeds the shared SQLite databases from the image.
2. **`thunderid-setup`** — one-shot container that generates this deployment's key
   material (TLS cert, JWT signing keys, the AES key that encrypts stored client
   secrets, and the Direct Auth Secret), then runs the in-process bootstrap over the
   declarative documents in `/opt/thunderid/bootstrap/` — the image's own defaults
   plus our [`bootstrap/03-nsw-resources.yaml`](bootstrap/03-nsw-resources.yaml) —
   and exits.
3. **`thunderid`** — the long-running server (listens on `https://localhost:8090`).
4. **`thunderid-seed`** — dev-only; once `thunderid` is healthy, mints an `ADMIN_CLI`
   token and seeds the sample resources (see *Seeding sample resources*). A bare
   `docker compose up thunderid` skips it.

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

   `CORS_ORIGINS_0..N` still drives the CORS allow-list, but it is applied by the seed
   via [`resources/shared/cors.json`](resources/shared/cors.json) — not by
   `deployment.yaml` and not by the bootstrap. Indices are read until the first gap.

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
ships `01-default-resources.yaml` and `02-server-configurations.yaml`; we mount one
document, [`idp/bootstrap/03-nsw-resources.yaml`](bootstrap/03-nsw-resources.yaml),
into that directory via `compose.yml`.

> **Changed in 1.0.0.** This used to be numbered *shell scripts* sourcing the image's
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
- **`bootstrap/03-nsw-resources.yaml`** (project) — **local-dev only.** Three
  documents:
  1. the `admin-cli` machine client (`client_id` `ADMIN_CLI`, secret from
     `ADMIN_CLI_SECRET`) in the `default` OU;
  2. an `NSW Admin CLI` role granting it the `system` permission on the System
     resource server, which is what makes a `client_credentials` call yield a
     **management token** (see *Seeding* below) — the programmatic alternative to
     copying a token out of the console.

  It is deliberately limited to those two: `admin-cli` is the only resource that must
  exist before the seed can authenticate. Everything else — CORS included — is seeded by
  `idp/sample-resources.sh`, so a bad value there cannot take the image's own default
  resources down with it.

  It grants management scope from a file-supplied secret, so do not mount it into a
  shared or UAT/prod deployment — use an interactively-obtained admin token there.

  Note the role is a *new* role rather than a re-declaration of the built-in
  `Administrator`. That is deliberate: the importer's role upsert writes only the
  fields present in the document, so partially re-declaring `Administrator` would drop
  its `permissions` block and silently revoke the management scope console login needs.

The project's sample resources are not created by the bootstrap container. They are seeded
by `idp/sample-resources.sh` (see *Seeding sample resources* below): **automatically** by
the `thunderid-seed` service on a full `docker compose up` (after `thunderid` is healthy),
or **by hand** against any deployment. A bare `docker compose up thunderid` does NOT seed.
That script creates:
  - **Private Sector** OU with **ADAM PVT LTD** and **EDWARD PVT LTD** child OUs
  - **Government Organization** OU with **NPQS / FCAU / CDA / SLPA / Customs / SLTB** child OUs
  - **`Private_User`** and **`Government_User`** user types
  - **`Traders`** and **`CHA`** groups; **`Trader`** and **`CHA`** roles (assigned to the
    matching groups — role inheritance is group-based)
  - **`OGA Reviewers`** group + **`OGA Reviewer`** role (government reviewers); **`AgencyM2M`**
    and **`NswM2M`** roles (machine clients) — see *API authorization* below
  - **`NSW_API`** and **`AGENCY_API`** OAuth2 resource servers (scopes + token audiences)
  - Sample users: `suresh`, `ramesh`, `gomesh` (ADAM), `naresh` (EDWARD), and
    `npqs_officer` / `fcau_officer` / `cda_officer` / `slpa_officer` / `customs_officer` /
    `sltb_officer` (government OUs)
  - **SPA applications** and **M2M applications** (see below)

## Seeding sample resources

The project sample resources (OUs, users, groups, roles, SPA + M2M apps) are a generic
**engine** that reads declarative JSON config from [`idp/resources/`](resources/) (see
*Resource configuration* below). The script is **idempotent** (existing entities are
detected via HTTP 409 and reused) and needs **`jq`** on `PATH`. The management API requires
a bearer `AUTH_TOKEN` — **including on localhost** (the running server is not
security-disabled, only the bootstrap container is).

### Automatically (local dev)

A full `docker compose up` runs the **`thunderid-seed`** service once `thunderid` is
healthy: it mints an `ADMIN_CLI` token and runs `sample-resources.sh` against the in-network
IdP (`https://thunderid:8090`). A bare `docker compose up thunderid` brings up only the IdP
(with the `admin-cli` client created) and does **not** seed.

### By hand — local dev (via `admin-cli`)

1. Mint a management token from the `admin-cli` client created during bootstrap:

   ```bash
   TOKEN=$(curl -k -s -u "ADMIN_CLI:${ADMIN_CLI_SECRET:-1234}" \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=client_credentials" -d "scope=system" \
     -d "resource=https://localhost:8090/mcp" \
     https://localhost:8090/oauth2/token | jq -r .access_token)
   ```

   (`${ADMIN_CLI_SECRET:-1234}` uses your `idp/.env` value if exported, else the dev default.)

   The `resource` parameter is **required** here: `system` is a permission scope, so the
   token must bind to a resource server, and this is the identifier of the System
   resource server the image bootstraps. Omit it and the request fails with
   `invalid_target`, since this deployment configures no `defaultResourceServer` (see
   *API authorization* below).

2. Run the seed with that token:

   ```bash
   API_BASE=https://localhost:8090 AUTH_TOKEN="$TOKEN" ./idp/sample-resources.sh
   ```

### By hand — UAT / production (via an admin token)

`admin-cli` is a **local-dev convenience only** — it is created solely by the compose
`thunderid-setup` service, from
[`bootstrap/03-nsw-resources.yaml`](bootstrap/03-nsw-resources.yaml), so the dev seed can
mint a token non-interactively. **Do not mount that file in UAT/prod.**
There, the privileged default-secret client is never provisioned; instead:

1. Deploy a **stock ThunderID** with only the image's default resources (admin user, default
   OU, etc.) — i.e. run the image's built-in bootstrap without mounting
   `03-nsw-resources.yaml`. Set a **strong `ADMIN_PASSWORD`** — that admin is your
   management entry point. (CORS is unaffected: the seed applies it, so it works the same
   way here.)
2. Obtain an admin management token **interactively** (e.g. log in to the console and copy a
   `system`-scoped token, or use the admin credentials against the token endpoint).
3. Run the seed with strong secrets set and that token:

   ```bash
   API_BASE=https://idp.example.com \
   AUTH_TOKEN="$ADMIN_TOKEN" \
   SAMPLE_USER_PASSWORD=... M2M_CLIENT_SECRET=... \
   INSECURE=0 ./idp/sample-resources.sh
   ```

### Notes on secrets & options

- **Fail-closed secrets.** Against a **non-localhost** target, an UNSET `SAMPLE_USER_PASSWORD`
  or `M2M_CLIENT_SECRET` aborts the run (naming the missing variable) rather than defaulting
  to `1234`. Localhost runs keep the `1234` dev default; set `ALLOW_DEFAULT_SECRETS=1` to allow
  it on a non-localhost target (e.g. throwaway CI).
- **`ADMIN_CLI_SECRET` must be set**, and its failure mode is blunt: the bootstrap
  documents are templated before parsing, an unresolved placeholder is a hard error, and
  the whole bundle is imported as one payload with `continueOnError=false`. So an unset
  `ADMIN_CLI_SECRET` fails the *entire* import — taking the admin user, `Console` app,
  flows and themes down with it, not just `admin-cli`. There is no `1234` fallback on this
  path (unlike the seed script's `ALLOW_DEFAULT_SECRETS` behaviour). This is why nothing
  else is bootstrapped: a bad `CORS_ORIGINS_*` only fails the seed.
- `API_BASE` defaults to `https://localhost:8090`; point it at a remote deployment as needed.
- `INSECURE=0` enforces TLS certificate validation (default `1` skips it for self-signed
  localhost certs).
- Values in `idp/.env` are loaded automatically and **take precedence over the command
  line** — so if `AUTH_TOKEN` / `API_BASE` are set in `idp/.env` they win; unset them there
  to pass values on the CLI. (This is why `thunderid-seed` runs the script from a copy with
  `.env` removed.)
- `./idp/sample-resources.sh --help` prints the full usage.

## Resource configuration (`idp/resources/`)

**What gets seeded is data, not code.** All entities live as JSON under
[`idp/resources/`](resources/), grouped by domain. Both `sample-resources.sh` (create) and
`sample-resources.down.sh` (delete) read the **same** files via the shared
[`idp/resources-lib.sh`](resources-lib.sh), so the two can never drift — adding an entity to
config covers both seeding and teardown. **Adding an agency, company, user, resource server,
role, group, or assignment is a config edit only — no script changes.**

```
idp/resources/
  _scopesets.json              named scope sets (reused by roles + apps)
  shared/
    resource-servers.json      NSW_API, AGENCY_API (+ nested resources -> actions)
    m2m-roles.json             AgencyM2M
  private-sector/
    ous.json  user-types.json  groups-roles.json  users.json  apps.json
  government/
    ous.json  user-types.json  groups-roles.json
    agencies.json              the OGA agencies (shorthand, see below)
```

Each file's top-level keys are entity-type buckets (`scopeSets`, `resourceServers`,
`organizationUnits`, `userTypes`, `groups`, `roles`, `roleAssignments`, `users`,
`applications`, `agencies`); the engine merges every file by concatenating same-named
arrays, so a domain file only carries its domain's entities and file placement is just for
humans. (An `agencies` entry is expanded at runtime into the primitive buckets — its OU,
officer `users`, portal/M2M `applications`, and the role→app assignments — so you never
author those by hand.) Cross-references use **logical keys** (an OU's `parent`,
a role's `resourceServer`, a user's `groups`, an assignment's `role`/`group`/`app`), which
the engine resolves to the server-assigned IDs at provisioning time. The `default` OU and
the Classic theme / default flows are image-provided and referenced (e.g. `"ou": "default"`)
without being created.

**Secrets never live in config.** Passwords / M2M secrets / redirect URIs are referenced by
**env-var name** — e.g. `"passwordEnv": { "override": "SAMPLE_SURESH_PASSWORD", "default":
"SAMPLE_USER_PASSWORD" }` resolves to `${SAMPLE_SURESH_PASSWORD:-${SAMPLE_USER_PASSWORD}}`
from `idp/.env` / the environment. Override those variables in `idp/.env` (see
`.env.example`); the JSON files stay committable.

### Adding an agency (the common case)

Append one block to [`idp/resources/government/agencies.json`](resources/government/agencies.json).
It expands to a child OU, a `Government_User` officer (joined to *OGA Reviewers*), a portal
SPA, the `<H>_TO_NSW` + `NSW_TO_<H>` M2M clients, and their role assignments:

```json
{
  "handle": "newoga", "name": "NEWOGA", "description": "New OGA description",
  "officer": { "username": "newoga_officer", "email": "newoga_officer@government.dev",
               "givenName": "NEWOGA", "familyName": "Officer", "phoneNumber": "+9477...",
               "passwordEnv": { "override": "SAMPLE_NEWOGA_OFFICER_PASSWORD", "default": "SAMPLE_USER_PASSWORD" } },
  "portal": { "name": "NEWOGAPortalApp", "clientId": "OGA_PORTAL_APP_NEWOGA", "port": 5180,
              "redirectUrisEnv": "NEWOGA_REDIRECT_URIS" },
  "m2m": {
    "toNsw": { "clientId": "NEWOGA_TO_NSW", "secretEnv": { "override": "M2M_NEWOGA_TO_NSW_SECRET", "default": "M2M_CLIENT_SECRET" } },
    "nswTo": { "clientId": "NSW_TO_NEWOGA", "secretEnv": { "override": "M2M_NSW_TO_NEWOGA_SECRET", "default": "M2M_CLIENT_SECRET" } }
  }
}
```

(Remember to add the new port to `CORS_ORIGINS_*` in `idp/.env` — see
[`resources/shared/cors.json`](resources/shared/cors.json) — and, for the agency to call
the NSW backend, to the backend's `AUTH_CLIENT_IDS` in `compose.yml`.)

## Applications created

| App | Client ID | Local URL |
| --- | --- | --- |
| TraderApp | `TRADER_PORTAL_APP` | http://localhost:5173 |
| NPQSPortalApp | `OGA_PORTAL_APP_NPQS` | http://localhost:5174 |
| FCAUPortalApp | `OGA_PORTAL_APP_FCAU` | http://localhost:5175 |
| CDAPortalApp | `OGA_PORTAL_APP_CDA` | http://localhost:5176 |
| SLPAPortalApp | `OGA_PORTAL_APP_SLPA` | http://localhost:5177 |
| CustomsPortalApp | `OGA_PORTAL_APP_CUSTOMS` | http://localhost:5178 |
| SLTBPortalApp | `OGA_PORTAL_APP_SLTB` | http://localhost:5179 |

M2M (client-credentials) apps (auth method: `client_secret_basic`):

- **OGA → NSW** (`aud=NSW_API`, `AgencyM2M` role): `NPQS_TO_NSW`, `FCAU_TO_NSW`,
  `CDA_TO_NSW`, `SLPA_TO_NSW`, `CUSTOMS_TO_NSW`, `SLTB_TO_NSW`.
- **NSW → OGA** (`aud=AGENCY_API`, `NswM2M` role): `NSW_TO_NPQS`, `NSW_TO_FCAU`,
  `NSW_TO_CDA`, `NSW_TO_SLPA`, `NSW_TO_CUSTOMS`, `NSW_TO_SLTB`.

## API authorization (OAuth2)

Each protected backend is registered as a **resource server** whose `identifier`
becomes the access-token **audience** (`aud`):

| `identifier` (= token `aud`) | Backend | Scopes (`<resource>:<action>`) |
| --- | --- | --- |
| `https://api.nsw-srilanka.local` | [OpenNSW/nsw](https://github.com/OpenNSW/nsw) `backend/` | `nsw:consignment:{read,write}`, `nsw:task:{read,write}`, `nsw:{hscode,company,cha}:read`, `nsw:storage:{read,write,delete}` |
| `https://api.nsw-agency.local` | [OpenNSW/nsw-agency](https://github.com/OpenNSW/nsw-agency) `backend/` | `agency:application:{read,review,feedback,inject}`, `agency:consignment:read`, `agency:storage:{read,write}` |

> **Identifiers must be absolute URIs, and they are opaque** — nothing ever
> dereferences them; they exist to be matched and to be written into `aud`. The URI
> requirement is not cosmetic: see *How a token gets its audience* below. The identifier
> is also how `idp/resources/**` names a resource server in `resourceServer:` references,
> so changing one means changing those too.
>
> `AUTH_AUDIENCE` in the backend must equal the `NSW_API` identifier — it is set in
> `.env.example`, `compose.yml`, `cmd/server/config/config.go` and
> `deployments/helm/values-example.yaml`. Changing the identifier means changing all
> four (and any existing local `.env`).

Scopes are namespaced (`nsw:*` / `agency:*`) so each maps to exactly one audience.

**How a token gets its scopes.** Scopes come from a **role grant on the principal**,
not from the app's requestable `scopes` list. So every caller is granted the relevant
scopes via a role:

| Caller | Grant |
| --- | --- |
| TraderApp users | `Trader` / `CHA` role (via group) → `NSW_API` scopes |
| `*_TO_NSW` M2M clients | **`AgencyM2M` role assigned to the application** (`type: app`) → `NSW_API` scopes |
| OGA portal users | `OGA Reviewer` role (via `OGA Reviewers` group) → `AGENCY_API` scopes |
| `NSW_TO_*` M2M clients | **`NswM2M` role assigned to the application** (`type: app`) → `agency:application:inject` |

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
> resource-server creation wizard shows a *pre-ticked* "Make this the default resource
> server" checkbox whenever the deployment has no default — which is precisely our
> configuration — and the resource-server list offers the same action. Accepting it flips
> the failure mode above for **every** caller in the deployment, silently: requests that
> should fail with `invalid_target` start returning HTTP 200 with a scopeless token
> instead. Untick it. Seeding is unaffected — `sample-resources.sh` uses the API directly,
> not the wizard.

So every permission-bearing caller sends `resource`:

| Caller | Sends `resource` | Where it is configured | Token `aud` |
| --- | --- | --- | --- |
| TraderApp users | NSW_API | `VITE_IDP_EXTRA_QUERY_PARAMS` (SPA `extraQueryParams`) | `https://api.nsw-srilanka.local` |
| OGA portal users | AGENCY_API | `VITE_IDP_EXTRA_QUERY_PARAMS` in OpenNSW/nsw-agency | `https://api.nsw-agency.local` |
| `*_TO_NSW` M2M | NSW_API | `NSW_TOKEN_PARAMS` in OpenNSW/nsw-agency | `https://api.nsw-srilanka.local` |
| `NSW_TO_*` M2M | AGENCY_API | `endpoint_params` in `configs/services*.json` | `https://api.nsw-agency.local` |
| `SLCE_TO_NSW` webhook | NSW_API | ⚠️ external — see below | `https://api.nsw-srilanka.local` |
| `admin-cli` management token | System RS | `compose.yml` (Stage D) | `https://localhost:8090/mcp` |
| `customs-asycuda` | n/a | requests no scopes, so stays unbound | app `defaultAudience` / `client_id` |

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
> fails with `invalid_target: must be an absolute URI` at the *token* endpoint.
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
  > stale `configdb.db` keeps its name so it *is* opened — just without the tables
  > 1.0.0 expects. The old certs must go too: they were baked into the 0.42.0 image and
  > are now generated per deployment.
- Role assignment is **group-based**: users inherit effective roles from group
  membership (`Traders` → `Trader`, `CHA` → `CHA`).
