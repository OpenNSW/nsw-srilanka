#!/usr/bin/env bash
# Replays SLPA's CMS webhooks against a locally running server, in the exact
# shape their finalized contract sends them, so the SLPA steps can be driven
# without the CMS.
#
#   ./loop.sh                    # happy path: clerk -> accountant -> invoice -> paid
#   ./loop.sh clerk-approve      # send one event (see `./loop.sh list`)
#   ./loop.sh list               # every event this can send
#   ./loop.sh slug               # find the slug of a consignment waiting on SLPA
#
# The consignment is chosen by its slug — the correlator recorded when the
# service order was raised. `./loop.sh slug` reads it out of the database:
#
#   SLUG=$(./loop.sh slug) ./loop.sh
#
# Overridable: BASE_URL SECRET SLUG ORDER_ID ORDER_NO CUSDEC INVOICE_NO AMOUNT
set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
SECRET="${SECRET:-local-dev-slpa-webhook-secret}"
SLUG="${SLUG:-7da12e38-182a-4f28-93fd-7075a0c48967}"
ORDER_ID="${ORDER_ID:-262319}"
ORDER_NO="${ORDER_NO:-SO-FCL-EXPORT-2026-262319}"
CUSDEC="${CUSDEC:-E12345}"
INVOICE_NO="${INVOICE_NO:-INV-2026-04412}"
AMOUNT="${AMOUNT:-4820.5}"

now() { date -u +%FT%TZ; }

# send signs the body the way the CMS does — HMAC-SHA256 over the exact bytes,
# hex, behind an "sha256=" prefix — and reports what the server answered.
#
#   200  applied
#   401  the signature did not verify: SECRET differs from SLPA_WEBHOOK_SECRET
#   404  no consignment is waiting on this slug (wrong SLUG, or already settled)
#   409  the order is mid-transition; the CMS would retry, so wait and resend
send() {
    local body="$1" sig status
    # This openssl build prints a bare hex digest; strip anything up to the
    # last "= " rather than taking a field by position.
    sig=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.*= *//')

    printf '\n--> %s\n' "$(printf '%s' "$body" | sed -n 's/.*"event":"\([^"]*\)".*/\1/p')"
    status=$(curl -s -o /tmp/slpa_wh.json -w '%{http_code}' -X POST "$BASE_URL/webhooks/slpa" \
        -H 'Content-Type: application/json' \
        -H "X-Signature: sha256=$sig" \
        --data-raw "$body")
    printf '    HTTP %s  %s\n' "$status" "$(head -c 200 /tmp/slpa_wh.json)"
    [ "$status" = "200" ]
}

# --- service order approvals -------------------------------------------------
#
# The finalized payload, every field, for all five events. Only "event" decides
# what happens here; "status" is the CMS's own code, carried through onto the
# record for support to read and never branched on.

order_event() {
    send "{\"event\":\"service_order.$1\",\"slug\":\"$SLUG\",\"service_order_id\":$ORDER_ID,\"service_order_no\":\"$ORDER_NO\",\"cusdec_serial\":\"$CUSDEC\",\"status\":\"$2\",\"invoice_no\":\"$3\",\"total_amount\":$4,\"timestamp\":\"$(now)\"}"
}

clerk_approve()  { order_event approved_by_accounts_clerk  actclk_approve_act "" 0; }
clerk_reject()   { order_event rejected_by_accounts_clerk  actclk_reject_act  "" 0; }
act_approve()    { order_event approved_by_accountant      act_approve_client "$INVOICE_NO" "$AMOUNT"; }
act_reject()     { order_event rejected_by_accountant      act_reject_client  "" 0; }
act_to_clerk()   { order_event rejected_to_accounts_clerk  act_reject_actclk  "" 0; }

# --- invoice -----------------------------------------------------------------

invoice_generated() {
    send "{\"event\":\"invoice.generated\",\"slug\":\"$SLUG\",\"service_order_no\":\"$ORDER_NO\",\"invoice_no\":\"$INVOICE_NO\",\"details\":{\"invoice_details\":{\"invoice_no\":\"$INVOICE_NO\",\"invoice_serial\":\"BIBE1/2026/04412\",\"status\":\"unpaid\",\"total_payable_lkr\":$AMOUNT,\"invoice_url\":\"https://slpacargoapi.slpa.lk/invoices/$INVOICE_NO.pdf\",\"invoice_generated_at\":\"$(now)\"}},\"timestamp\":\"$(now)\"}"
}

invoice_paid() {
    send "{\"event\":\"invoice.paid\",\"slug\":\"$SLUG\",\"service_order_no\":\"$ORDER_NO\",\"invoice_no\":\"$INVOICE_NO\",\"details\":{\"invoice_details\":{\"invoice_no\":\"$INVOICE_NO\",\"status\":\"paid\",\"total_payable_lkr\":$AMOUNT,\"invoice_url\":\"https://slpacargoapi.slpa.lk/invoices/$INVOICE_NO.pdf\",\"payment_slip_url\":\"https://slpacargoapi.slpa.lk/receipts/$INVOICE_NO.pdf\",\"invoice_paid_at\":\"$(now)\",\"payment_receipt\":{\"payment_receipt\":\"RCPT-88213\",\"paid_amount\":$AMOUNT,\"paid_datetime\":\"$(now)\"}}},\"timestamp\":\"$(now)\"}"
}

# --- finding a consignment ---------------------------------------------------

# slug prints the slug of a task parked on the approval wait. Each event
# completes that wait and the workflow reopens it, so the same slug drives the
# whole approval sequence.
slug() {
    docker compose exec -T db psql -U postgres -d nsw_db -At -c \
        "select data->'so'->>'slug' from task_records_v2
          where active_task_template_id like 'slpa-%'
            and state = 'QUEUED_EXTERNALLY'
          order by updated_at desc limit 1;"
}

case "${1:-happy}" in
    clerk-approve)     clerk_approve ;;
    clerk-reject)      clerk_reject ;;
    accountant-approve) act_approve ;;
    accountant-reject) act_reject ;;
    back-to-clerk)     act_to_clerk ;;
    invoice)           invoice_generated ;;
    paid)              invoice_paid ;;
    slug)              slug ;;
    happy)
        # Each step waits: the workflow has to reopen the wait before the next
        # event can find it parked, and a 409 means we got there first.
        clerk_approve && sleep 3 && act_approve && sleep 3 &&
            invoice_generated && sleep 3 && invoice_paid
        ;;
    list)
        echo "clerk-approve       service_order.approved_by_accounts_clerk   (actclk_approve_act)"
        echo "clerk-reject        service_order.rejected_by_accounts_clerk   (actclk_reject_act)"
        echo "accountant-approve  service_order.approved_by_accountant       (act_approve_client)"
        echo "accountant-reject   service_order.rejected_by_accountant       (act_reject_client)"
        echo "back-to-clerk       service_order.rejected_to_accounts_clerk   (act_reject_actclk)"
        echo "invoice             invoice.generated"
        echo "paid                invoice.paid"
        echo "happy               clerk-approve -> accountant-approve -> invoice -> paid"
        echo "slug                print the slug of a consignment waiting on SLPA"
        ;;
    *) echo "usage: $0 {$(basename "$0") list | happy | <event>}"; exit 2 ;;
esac
