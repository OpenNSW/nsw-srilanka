# GovPay+ gateway

The `govpay` payment method. It plugs GovPay+ into `core/payment` as an instruction flow: the payer gets a reference number and pays it through their bank's GovPay option, and GovPay+ calls back on two endpoints:

- **Presentment**: `POST /api/v1/payments/govpay/validate`. GovPay+ sends a reference number, and NSW answers with the fields to show the payer.
- **Update**: `POST /api/v1/payments/govpay/webhook`. GovPay+ reports the completed payment, and NSW settles the transaction.

Both calls are encrypted to this GO's RSA key and authenticated as GovPay+'s OAuth2 client.

## Configuration

The method's entry under `payment.methods` in `config.yaml`:

```yaml
payment:
  methods:
    - id: govpay
      is_active: true
      render_info:
        display_name: GovPay
        description: Offline payment instructions
        display_order: 1
      config:
        webhook_client_id: GOVPAY_TO_NSW
        private_key: "file:/certs/govpay/go_private.pem"
      payees:
        cda:
          defaults: { subinst_id: "004" }
          services:
            app_fee: { service_id: "001" }
            kcfn_fee: { service_id: "002" }
```

| Key                        | Meaning                                                                                                                                                                                      |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config.webhook_client_id` | The OAuth2 client GovPay+ calls back as. A callback from any other client is refused.                                                                                                        |
| `config.private_key`       | This GO's RSA private key, as a `file:` or `env:` reference; an inline PEM is refused. Without it every callback answers 500; a key that is set but unreadable stops the backend at startup. |
| `payees`                   | The GovPay+ ids for each fee. See below.                                                                                                                                                     |

`configs/config.yaml` and `configs/config.docker.yaml` are git-ignored and seeded from their `*.example.yaml` templates only when they do not exist. After pulling a change to the `payment` section of a template, copy the change into your local file.

## Payee mapping

A PAYMENT step names its payee in NSW's own terms: an `org_id` (who is paid) and a `service_id` (for what), such as `cda` / `app_fee`. These are the same in every environment. GovPay+ registers each fee under its own **sub-institution id** and **service id**, which are issued by GovPay+ and can differ between environments. `payees` maps one to the other:

- `payees.<org_id>.services.<service_id>` lists a fee. A fee not listed cannot be paid through GovPay.
- A fee's mapping is its service entry merged over its org's `defaults`, keys in the service entry winning. Put ids shared by all of an org's fees, typically `subinst_id`, in `defaults`.
- Every resolved mapping must have both `subinst_id` and `service_id`. A mapping missing either stops the backend at startup.
- Quote the ids. Unquoted, YAML reads `001` as the number 1, which the gateway refuses.

In the example above, `cda/app_fee` resolves to `{"subinst_id": "004", "service_id": "001"}`.

The ids in the `config*.example.yaml` templates are placeholders for local runs. Each deployed environment needs the ids GovPay+ issued for that environment.

### How the ids are checked

GovPay+ posts the sub-institution and service id on both callbacks. NSW compares them with the mapping of the payee the reference was issued for (ignoring case and surrounding space):

| Situation                   | Presentment                                               | Update                          |
| --------------------------- | --------------------------------------------------------- | ------------------------------- |
| Ids match                   | Fields returned                                           | Payment settled                 |
| Ids do not match            | 404 `invalid_reference`, the same as an unknown reference | 500; the payment is not settled |
| Payee's mapping lacks an id | 500 `configuration_error`                                 | 500; the payment is not settled |

The ids are looked up from the current config on every call, not stored on the transaction, so changing a fee's ids takes effect for payments already pending.

## Checklist: adding a fee

1. **Get the ids from GovPay+.** For each environment, the sub-institution id and service id GovPay+ registered the fee under.
2. **Name the payee in the PAYMENT step.** Set `org_id` and `service_id` in its `plugin_properties` (see [Payments](../../../docs/WORKFLOW_GUIDE.md#payments-payment) in the workflow guide). Use the same pair in every environment; don't put GovPay+ ids in the step.
3. **Map the payee in every environment's config.** Add it under the govpay method's `payees` in the `payment` section:
   - `configs/config.example.yaml` and `configs/config.docker.example.yaml`, with placeholder ids, so local runs work.
   - Each deployed environment's config (Helm: `backend.config.payment`), with that environment's real ids.

   A new org needs its own entry with `defaults` and `services`; a new fee for an existing org only needs a line under its `services`.

4. **Add the reference format, if the fee has its own.** A step whose `reference` names an `issuer` / `id_type` needs that format under `refid` in `configs/config.example.yaml`, `configs/config.docker.example.yaml` and each deployed config. Check that every `param` the format reads exists in the step's inputs and matches the format's `pattern`.
5. **Restart the backend.** Startup validates every mapping, so a missing id fails here rather than at payment time.
6. **Try it.** Run the workflow to `PENDING_PAYMENT`, then simulate GovPay+'s callback with the mapped ids (see [Simulating a payment webhook](../../../README.md#5-simulating-a-payment-webhook-dev-only) in the root README).

## Troubleshooting

| Error                                                                                                                   | Cause                                                                                                                                            | Fix                                                                                                                                                                                                    |
| ----------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `payment (<org>/<service>): no payment method can pay this payee; map it under payment.methods[].payees in config.yaml` | The fee is not under the govpay method's `payees` in the config this backend loaded. Locally, often a copy that predates the example.            | Add the mapping (checklist step 3) and restart.                                                                                                                                                        |
| Startup fails with `govpay identity not configured: a govpay payee mapping must declare subinst_id and service_id`      | A resolved mapping lacks one of the ids.                                                                                                         | Add the missing id to the service entry or the org's `defaults`.                                                                                                                                       |
| `refid: invalid or missing param: value for param "<name>" does not match pattern "<pattern>"`                          | A reference param read from the step's inputs has the wrong shape, e.g. company data seeded from an old `companies.json`.                        | Fix the source data (`cp configs/companies.example.json configs/companies.json` and re-run `seed-companies` locally), then start a new application. Running workflows keep the data they started with. |
| Presentment answers 404 `invalid_reference` for a reference that exists                                                 | GovPay+ posted ids that do not match the payee's mapping; the backend logs `govpay presentment rejected: identity mismatch` with the ids it got. | Correct the environment's mapping, or the fee's registration with GovPay+.                                                                                                                             |
| Every callback answers 500                                                                                              | No `private_key` configured, or `webhook_client_id` missing.                                                                                     | Set both in `config`.                                                                                                                                                                                  |
