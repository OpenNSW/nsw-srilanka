# Putting TNSW behind an API gateway

This guide is for the team that puts an API gateway in front of TNSW. It lists every route the backends serve, the backend and the caller of each, how each caller authenticates, and what the SLC Edge, SLPA and GovPay+ callbacks expect, so the gateway can route, authenticate and limit traffic without breaking them.

The OpenAPI specs in [`api/`](../api/) are the reference for each route's request and response. Run `make api-docs` and open <http://localhost:8088> to browse them in Swagger UI. Each operation also states, as vendor extensions, the scope it needs (`x-required-scope`), the mode that serves it (`x-deployment-modes`) and who calls it (`x-callers`).

## Backends

One image serves every backend. `mode` in its `config.yaml` decides which routes it serves:

| Backend                          | `mode`        | Local port (Docker Compose)                         | Serves                                                      |
| -------------------------------- | ------------- | --------------------------------------------------- | ----------------------------------------------------------- |
| TNSW API                         | `tnsw`        | 8080                                                | The shared routes, plus TNSW's own routes and the callbacks |
| Agency APIs                      | `agency`      | NPQS 8081, FCAU 8082, CDA 8083, EFC 8085, SLTB 8086 | The shared routes, plus `inject` and `cases`                |
| Trader Portal and agency portals | (static site) | TNSW 5173; agencies 5174–5179                       | The single-page app, which calls its API cross-origin       |

Each backend has its own host, and each portal calls its API at the `API_BASE_URL` in its `config.js`, from another origin. The Helm chart gives the backend and the portal an Ingress or Route each; the backend Service listens on port 80 and the container on `server.port`, over plain HTTP. TLS ends at the ingress or the gateway.

Route a backend's whole host to it: every path below lives under `/api/v1/`, `/webhooks/` or `/health`, and none is shared between hosts. Agency backends and TNSW call each other directly, so if they go through the gateway too, each agency host needs the same treatment as TNSW's.

## Routes

Scopes are listed without their `nsw:` prefix. "Body limit" is what the backend enforces; set the gateway's limit at or above it, and set one where the backend has none.

### Served by TNSW and by every agency

| Method | Path                                      | Scope           | Callers                       | Body limit                                |
| ------ | ----------------------------------------- | --------------- | ----------------------------- | ----------------------------------------- |
| GET    | `/health`                                 | public          | Load balancer, ops            | –                                         |
| GET    | `/api/v1/users/me`                        | `profile:read`  | Portals                       | –                                         |
| GET    | `/api/v1/tasks/{id}`                      | `task:read`     | Portals                       | –                                         |
| POST   | `/api/v1/tasks/{id}/steps/{stepId}`       | `task:write`    | Portals                       | `server.maxRequestBytes` (32 MiB shipped) |
| PUT    | `/api/v1/tasks/{id}/steps/{stepId}/draft` | `task:write`    | Portals                       | `server.maxRequestBytes`                  |
| POST   | `/api/v1/callbacks/{token}`               | `task:write`    | The other backend (see below) | `server.maxRequestBytes`                  |
| GET    | `/api/v1/static-data/{id}`                | `task:read`     | Portals                       | –                                         |
| POST   | `/api/v1/storage`                         | `storage:write` | Portals, agency APIs          | 1 MiB (metadata only)                     |
| GET    | `/api/v1/storage/{key}`                   | `storage:read`  | Portals, agency APIs          | –                                         |

`POST /api/v1/callbacks/{token}` carries replies between TNSW and the agencies. On TNSW, an agency calls it with its `<AGENCY>_TO_NSW` client to send a decision back; on an agency, TNSW calls it with `NSW_TO_<AGENCY>` to answer a request for more information. The `{token}` in the path is a credential: don't log it. The backend's own security log masks it.

`POST /api/v1/storage` returns an upload URL, and `GET /api/v1/storage/{key}` a download URL: with `storage.type: s3` the browser sends the file to, and reads it from, the bucket directly, not through the API. If the bucket sits behind the gateway too, allow `PUT` and `GET` on the presigned URLs unchanged; they expire after `storage.presignTTLSeconds` (900 seconds by default), and their query string is the signature.

With `storage.type: local`, which is for development, the API serves those URLs itself at `PUT` and `GET /api/v1/storage/{key}/content`, signed by a `token` query parameter and with no bearer token. Production deployments use S3 and don't register these routes; don't expose them.

### Served by TNSW only

| Method | Path                                                  | Scope                       | Callers                | Body limit |
| ------ | ----------------------------------------------------- | --------------------------- | ---------------------- | ---------- |
| GET    | `/api/v1/consignments`                                | `consignment:read`          | Trader Portal          | –          |
| POST   | `/api/v1/consignments`                                | `consignment:write`         | Trader Portal          | none read  |
| GET    | `/api/v1/consignments/{id}`                           | `consignment:read`          | Trader Portal          | –          |
| POST   | `/api/v1/consignments/{id}/flows`                     | `consignment:write`         | Trader Portal          | 1 KiB      |
| GET    | `/api/v1/consignments/{id}/agency`                    | `consignment:read`          | Agency APIs            | –          |
| POST   | `/api/v1/internal/storage`                            | `storage:internalwrite`     | Agency APIs            | 1 MiB      |
| GET    | `/api/v1/chas`                                        | `cha:read`                  | Trader Portal          | –          |
| GET    | `/api/v1/companies`                                   | `company:read`              | Trader Portal          | –          |
| GET    | `/api/v1/admin/consignments/{id}`                     | `consignment:adminread`     | Trader Portal (admins) | –          |
| GET    | `/api/v1/admin/workflows/{id}/engine-status`          | `consignment:adminread`     | Trader Portal (admins) | –          |
| GET    | `/api/v1/admin/task/{id}/engine-status`               | `consignment:adminread`     | Trader Portal (admins) | –          |
| POST   | `/api/v1/admin/workflows/{id}/steps/{stepId}/resolve` | `consignment:adminwrite`    | Trader Portal (admins) | **none**   |
| POST   | `/api/v1/admin/task/{id}/steps/{stepId}/resolve`      | `consignment:adminwrite`    | Trader Portal (admins) | **none**   |
| POST   | `/api/v1/payments/{gatewayId}/validate`               | `payment-webhooks:validate` | GovPay+                | 1 MiB      |
| POST   | `/api/v1/payments/{gatewayId}/webhook`                | `payment-webhooks:process`  | GovPay+                | 1 MiB      |
| POST   | `/webhooks/slce`                                      | `slce-webhooks:write`       | SLC Edge               | 1 MiB      |
| POST   | `/webhooks/slpa`                                      | none: HMAC signature        | SLPA CMS               | 1 MiB      |

`/api/v1/chas` has no caller in the Trader Portal today, though its roles hold the scope.

### Served by agencies only

| Method | Path                 | Scope              | Callers                  | Body limit |
| ------ | -------------------- | ------------------ | ------------------------ | ---------- |
| POST   | `/api/v1/inject`     | `workflow:inject`  | TNSW (`NSW_TO_<AGENCY>`) | **none**   |
| GET    | `/api/v1/cases`      | `consignment:read` | Agency portal            | –          |
| GET    | `/api/v1/cases/{id}` | `consignment:read` | Agency portal            | –          |

## Authentication

Every route except `/health`, `/webhooks/slpa` and the local storage content routes takes `Authorization: Bearer <JWT>`, issued by the deployment's ThunderID. The backend validates the token itself, so the gateway must pass the header through unchanged. A gateway that also validates the token can reject bad ones early, but it must apply the same rules and must not replace the token:

- **Signature:** RS256, RS384 or RS512, by a `kid` found in the JWKS at `authn.jwksURL`. The JWKS is cached for 5 minutes and fetched again on an unknown `kid`.
- **Claims:** `iss` equal to `authn.issuer`; `aud` containing `authn.audience`, which differs per backend (TNSW's API resource, or each agency's own, such as `https://api.npqs.nsw-agency.local` locally); `exp` required; expiry and not-before checked with 30 seconds' leeway; `client_id` present.
- **Grant:** `grant_type` is `authorization_code` for a user or `client_credentials` for a machine client. A user token also needs `sub`, `email`, `ouId` and `ouHandle`.
- **Scopes:** read from the space-delimited `scope` claim; roles from `roles`.

The backend answers a missing token with 401 `{"error": "unauthorized", "message": "authentication required"}`, a rejected one with 401 `"invalid authentication token"`, and a token without the route's scope with 403 `{"error": "forbidden", "message": "insufficient scope"}`. These bodies have no `correlationId`, unlike every other error, and no `WWW-Authenticate` header.

Some routes need more than the scope. The trader routes (`/api/v1/users/me`, and `/api/v1/consignments` and the routes under it except `agency`) answer a machine token with 401. `/api/v1/cases` answers 403 to a machine token, or to a user who holds none of the agency's roles. A task route checks the step's own rule after the scope, and answers 403 or 404.

### Callers and their clients

| Caller              | IdP client                | Grant              | Scopes it holds                                         |
| ------------------- | ------------------------- | ------------------ | ------------------------------------------------------- |
| Trader Portal users | `TRADER_PORTAL_APP`       | authorization code | By role: Trader, CHA, NSW Admin                         |
| Agency portal users | `OGA_PORTAL_APP_<AGENCY>` | authorization code | By the agency's officer role                            |
| SLC Edge            | `SLCE_TO_NSW`             | client credentials | `slce-webhooks:write`                                   |
| GovPay+             | `GOVPAY_TO_NSW`           | client credentials | `payment-webhooks:validate`, `payment-webhooks:process` |
| An agency, to TNSW  | `<AGENCY>_TO_NSW`         | client credentials | `consignment:read`, `storage:*`, `task:write`           |
| TNSW, to an agency  | `NSW_TO_<AGENCY>`         | client credentials | `workflow:inject`, `task:write`                         |
| SLPA CMS            | none                      | –                  | – (HMAC signature instead)                              |

The client ids are those of [`idp/bootstrap/`](../idp/bootstrap/); a deployment's IdP may name them differently.

## Callbacks from SLC Edge, SLPA and GovPay+

The three external systems call TNSW, not the agencies. None of them is checked by source address in the code, so if the gateway can restrict each route to its sender's addresses, it should. On all three:

- the body is parsed as JSON whatever its `Content-Type`, and unknown fields are ignored;
- a body over 1 MiB is refused before anything else (SLC Edge and SLPA get 400, not 413);
- every response carries `X-Trace-ID`, and none carries `Retry-After`.

Don't retry these calls at the gateway: each sender has its own retry policy, and a gateway retry would duplicate deliveries.

### SLC Edge: `POST /webhooks/slce`

- **Auth:** a bearer token for `SLCE_TO_NSW`, with scope `slce-webhooks:write`. When SLC Edge asks the IdP for it, it must name TNSW's API resource (`resource=` with TNSW's audience), or the token won't carry TNSW's audience. Only the scope is checked, not the client id.
- **Events:** one route for every event, chosen by `eventType`: `CUSDEC_INTEGRATED`, `PAYMENT_CONFIRMED`, `WARRANTING_COMPLETED`, `EXPORT_RELEASED`, `CDN_INTEGRATED` and `CDN_ACKNOWLEDGED`.
- **Responses:** 200 or 202, with no body, means handled; 202 means a repeat of a CusDec result already applied. 503 and 500 mean retry later, for example an event about a declaration not recorded yet. 400 and 404 are final.

### SLPA CMS: `POST /webhooks/slpa`

- **Auth:** no bearer token. The CMS signs the raw body: `X-NSW-Signature: sha256=<hex HMAC-SHA256 of the body>`, with the secret in `integrations.slpaWebhookSecret` (`SLPA_WEBHOOK_SECRET`). The gateway must not change the body in any way, including its whitespace or encoding, or the signature fails. There is no timestamp in the signature and no replay protection, so only the signature and the gateway stand between the route and the internet.
- **Events:** `service_order.*` approval decisions and `invoice.paid`.
- **Responses:** 200 `{"received": true}` means handled. 404 and 409 mean the order is not known or not waiting yet: retry later. 400 and 401 are final.

### GovPay+: `POST /api/v1/payments/govpay/validate` and `.../webhook`

- **Auth:** a bearer token whose `client_id` must also be the one in `payment_methods.json` (`webhook_client_id`, `GOVPAY_TO_NSW` locally); any other client gets 401.
- **Encryption:** every call carries a `TransactionKey` header, an RSA-encrypted AES key, and the values in the body are encrypted with it. The gateway can't inspect or validate the payload; pass the header and the body through unchanged.
- **Responses:** `validate` answers 200, 404 for an unknown reference, 409 for one that can't be paid. `webhook` answers 200, including for a payment already settled, 404 and 422 as final, and 500 to be retried. `gatewayId` is `govpay`; any other value gets 500.

## Cross-cutting behaviour

- **CORS.** The backend answers CORS itself, from `cors.allowedOrigins`, `allowedMethods`, `allowedHeaders`, `allowCredentials` and `maxAge` (no defaults). It answers a preflight `OPTIONS` from an allowed origin with 204 before authentication. Let either the gateway or the backend handle CORS, not both: two sets of `Access-Control-*` headers break browsers.
- **Trace IDs.** The backend reads the first of `X-Trace-ID`, `X-Correlation-ID` or `X-Request-ID` that is 1–64 characters of `[A-Za-z0-9-_:./=]`, or generates one, and returns it as `X-Trace-ID` on every response. It is the `correlationId` in error bodies and the `traceId` in the logs. Forward the gateway's request id in `X-Request-ID` to tie the two logs together. The shipped CORS settings neither allow nor expose these headers, so browsers can't send or read them.
- **Timeouts.** `server.readHeaderTimeout`, `readTimeout`, `writeTimeout` and `idleTimeout` are 5, 15, 30 and 60 seconds in the shipped configs. Set the gateway's upstream timeout a little above `writeTimeout`.
- **Headers.** Requests up to Go's default of 1 MiB of headers are accepted.
- **Body limits.** There is no global limit: see each route's. Cap the gateway at 32 MiB for the task routes, and set a limit on `/api/v1/inject` and the admin `resolve` routes, which have none.
- **Health.** `GET /health` (and `HEAD`) is public. It answers 200 `{"status": "ok", ...}`, or 503 when the database or the authentication layer is unhealthy; it doesn't check the IdP's JWKS or Temporal. Use it for readiness, not liveness. Its `service` field reads `tnsw-api` on agency backends too.
- **Security log.** The backend logs every request except `/health` with `category=security`, including request and response bodies up to 4 KiB. It masks tokens, secrets and presigned URLs, but logs personal data as sent.

## Calls TNSW makes

TNSW calls out to these systems, at the base URL of each one's entry in `services.json`:

| System   | Service `id`         | Auth                                                                                           | Calls                                                                       |
| -------- | -------------------- | ---------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| SLC Edge | `customs-asycuda`    | OAuth2 client credentials against the external IdP (`NSW_TO_ASYCUDA_CLIENT_ID`)                | CusDec verify and submit, CDN submit                                        |
| SLPA CMS | `slpa`               | OAuth2 client credentials against the token endpoint of the SLPA CMS (`NSW_TO_SLPA_CLIENT_ID`) | eCDN upload, service orders, invoices, gate passes, container consolidation |
| IPPC hub | `ippc-hub`           | Mutual TLS                                                                                     | ePhyto certificates                                                         |
| Agencies | one entry per agency | OAuth2 client credentials (`NSW_TO_<AGENCY>`)                                                  | `POST /api/v1/inject`, `POST /api/v1/callbacks/{token}`                     |

The paths TNSW calls are set by the workflow artifacts in one-trade-artifacts, not by this repository.
