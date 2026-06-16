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
`flows/`, `themes/`, `i18n/` assets, all of which we use **as-is**. This repo adds a
single project script via a mount:

```yaml
- ./02-sample-resources.sh:/opt/thunderid/bootstrap/02-sample-resources.sh
```

- **`01-default-resources.sh`** (image default, not overridden) — default OU, `Person`
  user type, default agent type, admin user, system resource server + permissions,
  `Administrators` group, `Administrator` role, default flows, the `Console` application,
  themes, and i18n translations.
- **`02-sample-resources.sh`** (this repo) — project sample resources:
  - **Private Sector** OU with **ADAM PVT LTD** and **EDWARD PVT LTD** child OUs
  - **Government Organization** OU with **NPQS / FCAU / CDA / SLPA** child OUs
  - **`Private_User`** and **`Government_User`** user types
  - **`Traders`** and **`CHA`** groups; **`Trader`** and **`CHA`** roles (assigned to the
    matching groups — role inheritance is group-based)
  - **`OGA Reviewers`** group + **`OGA Reviewer`** role (government reviewers); **`AgencyM2M`**
    role (machine clients) — see *API authorization* below
  - **`NSW_API`** and **`AGENCY_API`** OAuth2 resource servers (scopes + token audiences)
  - Sample users: `suresh`, `ramesh`, `gomesh` (ADAM), `naresh` (EDWARD), and
    `npqs_user` / `fcau_user` / `cda_user` / `slpa_user` (government OUs)
  - **SPA applications** and **M2M applications** (see below)

## Applications created

| App | Client ID | Local URL |
| --- | --- | --- |
| TraderApp | `TRADER_PORTAL_APP` | http://localhost:5173 |
| NPQSPortalApp | `OGA_PORTAL_APP_NPQS` | http://localhost:5174 |
| FCAUPortalApp | `OGA_PORTAL_APP_FCAU` | http://localhost:5175 |
| CDAPortalApp | `OGA_PORTAL_APP_CDA` | http://localhost:5176 |
| SLPAPortalApp | `OGA_PORTAL_APP_SLPA` | http://localhost:5177 |

M2M (client-credentials) apps for external services calling NSW APIs:
`NPQS_TO_NSW`, `FCAU_TO_NSW`, `CDA_TO_NSW`, `SLPA_TO_NSW` (auth method:
`client_secret_basic`).

## API authorization (OAuth2)

Each protected backend is registered as a **resource server** whose `identifier`
becomes the access-token **audience** (`aud`):

| Resource server (`identifier`) | Backend | Scopes (`<resource>:<action>`) |
| --- | --- | --- |
| `NSW_API` | [OpenNSW/nsw](https://github.com/OpenNSW/nsw) `backend/` | `nsw:consignment:{read,write}`, `nsw:task:{read,write}`, `nsw:{hscode,company,cha}:read`, `nsw:storage:{read,write,delete}` |
| `AGENCY_API` | [OpenNSW/nsw-agency](https://github.com/OpenNSW/nsw-agency) `backend/` | `agency:application:{read,review,feedback}`, `agency:consignment:read`, `agency:storage:{read,write}` |

Scopes are namespaced (`nsw:*` / `agency:*`) so each maps to exactly one audience.

**How tokens get their scopes + audience** — in ThunderID, a token's scopes (and
therefore its `aud`) come from a **role grant on the principal**, not from the app's
requestable `scopes` list. So every caller is granted the relevant scopes via a role:

| Caller | Grant | Token `aud` |
| --- | --- | --- |
| TraderApp users | `Trader` / `CHA` role (via group) → `NSW_API` scopes | `NSW_API` |
| `*_TO_NSW` M2M clients | **`AgencyM2M` role assigned to the application** (`type: app`) → `NSW_API` scopes | `NSW_API` |
| OGA portal users | `OGA Reviewer` role (via `OGA Reviewers` group) → `AGENCY_API` scopes | `AGENCY_API` |

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
