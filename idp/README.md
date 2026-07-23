# Identity Provider (IdP) Setup

## Overview

We use [ThunderID](https://thunderid.dev/) (`ghcr.io/thunder-id/thunderid`) as the
Identity Provider for this project — a lightweight, developer-friendly identity and
access management solution. This directory runs the stock ThunderID image and layers
on the project's sample resources via a bootstrap script.

> ThunderID is the renamed successor of Asgardeo Thunder (`asgardeo/thunder`). The
> binary, image, and install paths moved from `thunder` / `/opt/thunder` to
> `thunderid` / `/opt/thunderid` (rename landed in v0.37.0).

## Getting Started

### Quick Start (with defaults)

Start the IdP with default credentials (`admin` / `1234`):

```bash
docker compose up
```

A full `docker compose up` runs four IdP services in order:

1. **`thunderid-db-init`** — seeds the shared SQLite databases from the image.
2. **`thunderid-setup`** — one-shot container that starts the server with security
   disabled, runs the bootstrap scripts (incl. `bootstrap/02-admin-cli.sh`), then exits.
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

   `deployment.yaml` is templated from these vars at server startup — e.g.
   `{{.PUBLIC_URL}}` ← `PUBLIC_URL`, and `{{- range .CORS_ORIGINS }}` aggregates the
   indexed `CORS_ORIGINS_0..N` entries into the allowed-origins list.

3. Start the IdP:

   ```bash
   docker compose up
   ```

### Developer Console Access

Once running, open the developer console at `https://localhost:8090/console`:

- **Default credentials**: `admin` / `1234`
- **Custom credentials**: the values from your `.env`

> ⚠️ **Security Warning**: change the default password for any non-local environment.

## Bootstrap Scripts

`thunderid-setup` runs the numbered scripts in `/opt/thunderid/bootstrap` (built-in +
custom, sorted by name; `common.sh` — which provides `api_call`, `log_*`, … — is sourced,
not executed). The image ships `01-default-resources.sh`, `common.sh`, and the `flows/`,
`themes/`, `i18n/` assets (used **as-is**); we mount one custom script,
[`idp/bootstrap/02-admin-cli.sh`](bootstrap/02-admin-cli.sh), into that directory via `compose.yml`.

- **`01-default-resources.sh`** (image default, not overridden) — default OU, `Person`
  user type, default agent type, admin user, system resource server + permissions,
  `Administrators` group, `Administrator` role, default flows, the `Console` application,
  themes, and i18n translations.
- **`bootstrap/02-admin-cli.sh`** (project, mounted into `thunderid-setup`) — **local-dev only.**
  Creates the `admin-cli` machine client (`client_id` `ADMIN_CLI`, secret `ADMIN_CLI_SECRET`) in
  the `default` OU and assigns the built-in `Administrator` role to it. A `client_credentials`
  call then yields a `system`-scoped **management token** (see *Seeding* below) — the
  programmatic alternative to copying a token out of the console. It **fails closed** if
  `ADMIN_CLI_SECRET` is unset (no silent `1234`; override with `ALLOW_DEFAULT_SECRETS=1`). Do
  not deploy this script to UAT/prod — use an interactively-obtained admin token there instead.

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
     https://localhost:8090/oauth2/token | jq -r .access_token)
   ```

   (`${ADMIN_CLI_SECRET:-1234}` uses your `idp/.env` value if exported, else the dev default.)

2. Run the seed with that token:

   ```bash
   API_BASE=https://localhost:8090 AUTH_TOKEN="$TOKEN" ./idp/sample-resources.sh
   ```

### By hand — UAT / production (via an admin token)

`admin-cli` is a **local-dev convenience only** — it is created solely by the compose
`thunderid-setup` service ([`bootstrap/02-admin-cli.sh`](bootstrap/02-admin-cli.sh)) so the
dev seed can mint a token non-interactively. **Do not deploy that script to UAT/prod.**
There, the privileged default-secret client is never provisioned; instead:

1. Deploy a **stock ThunderID** with only the image's default resources (admin user, default
   OU, etc.) — i.e. run the image's built-in bootstrap without mounting `02-admin-cli.sh`.
   Set a **strong `ADMIN_PASSWORD`** — that admin is your management entry point.
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
  it on a non-localhost target (e.g. throwaway CI). `02-admin-cli.sh` fails the same way for an
  unset `ADMIN_CLI_SECRET` (gated only by `ALLOW_DEFAULT_SECRETS`, since it has no `API_BASE`).
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

(Remember to add the new port to `CORS_ORIGINS_*` in `idp/.env` and, for the agency to call
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

| Resource server (`identifier`) | Backend | Scopes (`<resource>:<action>`) |
| --- | --- | --- |
| `NSW_API` | [OpenNSW/nsw](https://github.com/OpenNSW/nsw) `backend/` | `nsw:consignment:{read,write}`, `nsw:task:{read,write}`, `nsw:{hscode,company,cha}:read`, `nsw:storage:{read,write,delete}` |
| `AGENCY_API` | [OpenNSW/nsw-agency](https://github.com/OpenNSW/nsw-agency) `backend/` | `agency:application:{read,review,feedback,inject}`, `agency:consignment:read`, `agency:storage:{read,write}` |

Scopes are namespaced (`nsw:*` / `agency:*`) so each maps to exactly one audience.

**How tokens get their scopes + audience** — in ThunderID, a token's scopes (and
therefore its `aud`) come from a **role grant on the principal**, not from the app's
requestable `scopes` list. So every caller is granted the relevant scopes via a role:

| Caller | Grant | Token `aud` |
| --- | --- | --- |
| TraderApp users | `Trader` / `CHA` role (via group) → `NSW_API` scopes | `NSW_API` |
| `*_TO_NSW` M2M clients | **`AgencyM2M` role assigned to the application** (`type: app`) → `NSW_API` scopes | `NSW_API` |
| OGA portal users | `OGA Reviewer` role (via `OGA Reviewers` group) → `AGENCY_API` scopes | `AGENCY_API` |
| `NSW_TO_*` M2M clients | **`NswM2M` role assigned to the application** (`type: app`) → `agency:application:inject` | `AGENCY_API` |

> Because each caller's role sets the correct audience, the backends can enable
> audience validation (`jwt.WithAudience("NSW_API")` / `"AGENCY_API"`). Without a role
> grant, ThunderID falls back to `aud = client_id` and emits no scopes — which is why
> M2M clients need the `AgencyM2M` role assigned to the application itself.

## Notes

- The developer console and login screens show the stock **ThunderID** branding (the
  product name is `brand.product_name` in the image's `apps/{console,gate}/config.js`,
  not an env var or API — left at the image default).
- All data is persisted in the `thunderid-db` (and `consent-db`) Docker volumes. To
  reset, `docker compose down -v` and `up` again.
- Role assignment is **group-based**: users inherit effective roles from group
  membership (`Traders` → `Trader`, `CHA` → `CHA`).
