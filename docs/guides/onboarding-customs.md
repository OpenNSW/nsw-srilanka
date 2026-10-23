# Connecting Sri Lanka Customs

Customs is connected differently from other agencies. TNSW doesn't hand it review tasks: the
trade flow's Customs steps submit declarations (CusDec) and cargo dispatch notes (CDN) to
SLC Edge, Customs' gateway to ASYCUDA World, and SLC Edge reports what happened on one
webhook. So connecting Customs is two sets of credentials, one per direction, and each is
issued by the side that receives the calls:

| Direction                         | What it carries                                                              | Who issues the client                                           |
| --------------------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------- |
| TNSW → SLC Edge                   | CusDec submission and verification, CDN submission                           | Customs, from its own identity server                           |
| SLC Edge → TNSW, `/webhooks/slce` | Integration results, payment, warranting, CDN acknowledgment, export release | TNSW's ThunderID, as a client granted `nsw:slce-webhooks:write` |

The trade flows themselves already exist in `one-trade-artifacts` (`tnsw/customs/`).
This guide only connects a deployment to Customs.

Names, hosts and URLs here are examples; use your own.

## Before you start

You need:

- A **ThunderID admin** with console access. The NSW API must have the `slce-webhooks`
  resource with a `write` action, which the platform resources include.
- **Cluster access** to TNSW's namespace, and write access to its configuration.
- From **Customs**: the SLC Edge API URL, its token URL, and a client ID and secret for
  TNSW.

## Step 1: Create Customs' client in ThunderID

In the ThunderID console:

1. In **Applications**, create a **Backend Service** for Customs, for example named
   `SLCE_TO_NSW`, in the OU that holds the platform's clients. Add the custom scope
   `nsw:slce-webhooks:write`.
2. In **Roles**, create a role for it, for example `SLCE Webhook`, with the permission
   `nsw:slce-webhooks:write` on the NSW API, and assign it to the application on the
   **Assignments** tab.

Note the client ID. Keep the client secret only until it has been handed to Customs
(Step 3); if it is lost, use **Regenerate Client Secret**.

## Step 2: Configure TNSW

TNSW reads all of these at startup, so make the changes together and roll the backend
once.

Add the secret Customs gave TNSW to TNSW's Secret (`nsw-secrets` in the examples):

```sh
read -rs -p 'Secret from Customs: ' S; echo
S="$S" jq -n '{stringData: {"m2m-asycuda-secret": $ENV.S}}' \
  | kubectl patch secret nsw-secrets -n tnsw --type merge --patch-file /dev/stdin
unset S
```

In TNSW's Helm values, expose it to the backend, and let Customs' client in:

```yaml
backend:
  env:
    M2M_NSW_TO_ASYCUDA_SECRET:
      valueFrom: { secretKeyRef: { name: nsw-secrets, key: m2m-asycuda-secret } }
  config:
    authn:
      clientIDs:
        - "<Customs' client ID>" # from Step 1, next to the existing entries
```

In TNSW's `services.json`, add SLC Edge with the values Customs gave you:

```json
{
  "id": "customs-asycuda",
  "url": "https://slc-edge.example.gov.lk",
  "timeout": "30s",
  "auth": {
    "type": "oauth2",
    "options": {
      "token_url": "https://slc-edge.example.gov.lk/auth/token",
      "client_id": "<client ID from Customs>",
      "client_secret": "env:M2M_NSW_TO_ASYCUDA_SECRET",
      "scopes": []
    }
  }
}
```

The `id` is not an example: the Customs steps in `one-trade-artifacts` call the service
`customs-asycuda`. They call `/api/declaration/v1`, `/api/declaration/v1/verify` and
`/api/cdn/v1` on it.

Then roll out:

```sh
helm upgrade lk-tnsw ./deployments/helm/lk-tnsw -n tnsw -f <your-values>.yaml
```

## Step 3: Hand over to Customs

Give Customs, through a secure channel for the secret:

- The webhook URL: `https://<TNSW API host>/webhooks/slce`.
- The token URL, with the NSW API identifier as its `resource`:
  `https://<IdP host>/oauth2/token?resource=https://api.nsw-srilanka.local`. Without
  `resource`, the token request fails with `invalid_target`.
- The client ID and secret from Step 1, and how to request the token:
  `grant_type=client_credentials` and `scope=nsw:slce-webhooks:write`.
- The webhook contract, [`api/webhooks.slce.openapi.yaml`](../../api/webhooks.slce.openapi.yaml).

## Step 4: Verify

1. **Customs' client can get a token.** With the client ID and secret from Step 1:

   ```sh
   read -rs -p 'Customs client secret: ' SECRET; echo
   curl -s -K <(printf 'user = "%s:%s"\n' "<Customs' client ID>" "$SECRET") \
     --data-urlencode grant_type=client_credentials \
     --data-urlencode scope=nsw:slce-webhooks:write \
     'https://idp.example.com/oauth2/token?resource=https://api.nsw-srilanka.local' \
     | jq -r .access_token \
     | jq -R 'split(".")[1] | gsub("-";"+") | gsub("_";"/")
              | . + ("=" * ((4 - length % 4) % 4)) | @base64d | fromjson'
   unset SECRET
   ```

   The credentials reach `curl` as a config on a file descriptor, so the secret never
   appears in its process arguments.

   Check `aud` is `https://api.nsw-srilanka.local` and `scope` is
   `nsw:slce-webhooks:write`.

2. **TNSW reaches SLC Edge.** A trader submits a CusDec. The step accepts it and moves to
   officer review, or returns it to the trader with SLC Edge's errors. Either way, SLC Edge
   answered.
3. **SLC Edge reaches TNSW.** After Customs processes the declaration, the consignment
   moves past the step waiting on Customs.

## Common changes

| Task                   | Where                                                                                       |
| ---------------------- | ------------------------------------------------------------------------------------------- |
| Rotate TNSW's secret   | Customs issues a new one; update `m2m-asycuda-secret` and roll the backend                  |
| Rotate Customs' secret | **Regenerate Client Secret** in the console and hand it to Customs. Nothing changes in TNSW |
| SLC Edge moves         | `url` and `token_url` of `customs-asycuda` in `services.json`, then roll the backend        |

## Troubleshooting

| Symptom                                                | Likely cause                                                                                                                 |
| ------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| CusDec submission fails with a token error in the logs | Wrong `token_url`, `client_id` or `M2M_NSW_TO_ASYCUDA_SECRET`                                                                |
| Customs' token request fails with `invalid_target`     | Its token URL lacks `?resource=https://api.nsw-srilanka.local`                                                               |
| Webhook `401`                                          | Customs' client ID not in `authn.clientIDs`, or its token's `aud` isn't the NSW API identifier                               |
| Webhook `403` `insufficient scope`                     | The client lacks its role (Step 1), or the token request didn't ask for `nsw:slce-webhooks:write`                            |
| Webhook `2xx`, but the consignment doesn't move        | The event names a declaration or note TNSW doesn't know. See [Diagnosing webhook deliveries](#diagnosing-webhook-deliveries) |

### Diagnosing webhook deliveries

With `logLevel: debug`, the backend logs each delivery's full body as
`slce: webhook received`. The bodies hold declaration references, amounts, bank
references and vessel details, so turn debug on only while diagnosing a problem, and set
`logLevel` back to `info` straight after.

## Local development

Locally, `SLCE_TO_NSW` comes from
[`idp/bootstrap/10-nsw-platform.yaml`](../../idp/bootstrap/10-nsw-platform.yaml), and
`customs-asycuda` is in `configs/services*.example.json` with placeholder values. Put the
real SLC Edge test credentials in your gitignored `configs/services.json` and `.env`
(`M2M_NSW_TO_ASYCUDA_SECRET`).
