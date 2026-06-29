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

The stack runs three services in order:

1. **`thunderid-db-init`** — seeds the shared SQLite databases from the image.
2. **`thunderid-setup`** — one-shot container that starts the server with security
   disabled, runs the bootstrap scripts, then exits.
3. **`thunderid`** — the long-running server (listens on `https://localhost:8090`).

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

`thunderid-setup` auto-discovers and runs numbered scripts in `/opt/thunderid/bootstrap`
(sorted by name; `common.sh` — which provides `api_call`, `log_*`, `create_flow`, … — is
sourced, not executed). The image ships `01-default-resources.sh`, `common.sh`, and the
`flows/`, `themes/`, `i18n/` assets, all of which we use **as-is**.

- **`01-default-resources.sh`** (image default, not overridden) — default OU, `Person`
  user type, default agent type, admin user, system resource server + permissions,
  `Administrators` group, `Administrator` role, default flows, the `Console` application,
  themes, and i18n translations.

The project's sample resources are **no longer auto-seeded at bootstrap** — they are
applied by hand with `idp/sample-resources.sh` (see *Seeding sample resources manually*
below). That script creates:
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

## Seeding sample resources manually

The project sample resources (OUs, users, groups, roles, SPA + M2M apps) are seeded by
`idp/sample-resources.sh`, run by hand against a running deployment. The script is a
generic **engine**: it reads declarative JSON config from [`idp/resources/`](resources/)
(see *Resource configuration* below) and reuses idempotent helpers, so it is **idempotent**
— safe to re-run against a partially-seeded deployment (existing entities are detected via
HTTP 409 and reused). It is self-contained apart from one tool: **`jq`** must be on `PATH`
(it parses and merges the config).

Against the local compose stack (bootstrap runs security-disabled, self-signed TLS):

```bash
docker compose up -d                 # wait until `thunderid` is healthy
API_BASE=https://localhost:8090 ALLOW_NO_AUTH=1 ./idp/sample-resources.sh
```

Against a real deployment (management APIs require auth):

```bash
API_BASE=https://idp.example.com \
AUTH_TOKEN=<bearer-token-for-the-management-API> \
./idp/sample-resources.sh
```

- `API_BASE` defaults to `https://localhost:8090`.
- `AUTH_TOKEN` is required for non-localhost targets; it is sent as `Authorization: Bearer …`.
- `INSECURE=0` enforces TLS certificate validation (default `1` skips it for self-signed
  localhost certs).
- Values in `idp/.env` are loaded automatically and take precedence over the command line.
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

Each file's top-level keys are entity-type buckets (`resourceServers`,
`organizationUnits`, `userTypes`, `groups`, `roles`, `roleAssignments`, `users`,
`applications`, `appRoleAssignments`, `agencies`); the engine merges every file by
concatenating same-named arrays, so a domain file only carries its domain's entities and
file placement is just for humans. Cross-references use **logical keys** (an OU's `parent`,
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
