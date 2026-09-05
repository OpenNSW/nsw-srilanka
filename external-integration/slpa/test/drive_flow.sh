#!/bin/bash
# Drives an SLPA consignment through the CMS callbacks, locally.
#
#   ./drive_flow.sh approve      # clerk -> accountant -> approved
#   ./drive_flow.sh reject-clerk # rejected by the accounts clerk
#   ./drive_flow.sh invoice      # invoice.generated -> invoice.paid
#   ./drive_flow.sh all          # approve, then invoice
#
# Override per consignment:
#   SLUG=... ORDER_NO=... ./drive_flow.sh all
set -u

BASE_URL="${BASE_URL:-http://localhost:8080}"
SECRET="${SECRET:-local-dev-slpa-webhook-secret}"
SLUG="${SLUG:-88ec481f-726c-4eb3-971c-1b280a3e7aec}"
ORDER_NO="${ORDER_NO:-SO-FCL-EXPORT-2026-262322}"
INVOICE_NO="${INVOICE_NO:-INV-2026-04412}"
ORDER_ID="${ORDER_ID:-262319}"
CUSDEC="${CUSDEC:-E12345}"

send() {
    local body="$1"
    local sig
    # This openssl build prints a bare hex digest, so strip anything up to the
    # last "= " rather than taking a field by position.
    sig=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.*= *//')

    echo "--> $(printf '%s' "$body" | sed -n 's/.*"event":"\([^"]*\)".*/\1/p')"
    curl -s -o /tmp/slpa_wh.json -w '    HTTP %{http_code}\n' -X POST "$BASE_URL/webhooks/slpa" \
        -H 'Content-Type: application/json' \
        -H "X-Signature: sha256=$sig" \
        --data-raw "$body"
    cat /tmp/slpa_wh.json 2>/dev/null | head -c 300; echo
}

approve() {
    send "{\"event\":\"service_order.approved_by_accounts_clerk\",\"slug\":\"$SLUG\",\"service_order_id\":$ORDER_ID,\"service_order_no\":\"$ORDER_NO\",\"cusdec_serial\":\"$CUSDEC\",\"status\":\"actclk_approve_act\",\"timestamp\":\"$(date -u +%FT%TZ)\"}"
    sleep 2
    send "{\"event\":\"service_order.approved_by_accountant\",\"slug\":\"$SLUG\",\"service_order_no\":\"$ORDER_NO\",\"status\":\"act_approve_client\",\"invoice_no\":\"$INVOICE_NO\",\"total_amount\":4820.5,\"timestamp\":\"$(date -u +%FT%TZ)\"}"
}

reject_clerk() {
    send "{\"event\":\"service_order.rejected_by_accounts_clerk\",\"slug\":\"$SLUG\",\"service_order_no\":\"$ORDER_NO\",\"status\":\"actclk_reject_act\",\"timestamp\":\"$(date -u +%FT%TZ)\"}"
}

invoice() {
    send "{\"event\":\"invoice.generated\",\"slug\":\"$SLUG\",\"service_order_no\":\"$ORDER_NO\",\"invoice_no\":\"$INVOICE_NO\",\"details\":{\"invoice_details\":{\"invoice_no\":\"$INVOICE_NO\",\"status\":\"unpaid\",\"total_payable_lkr\":4820.5,\"invoice_url\":\"https://slpacargoapi.slpa.lk/invoices/$INVOICE_NO.pdf\",\"invoice_generated_at\":\"$(date -u +%FT%TZ)\"}}}"
    sleep 2
    send "{\"event\":\"invoice.paid\",\"slug\":\"$SLUG\",\"service_order_no\":\"$ORDER_NO\",\"invoice_no\":\"$INVOICE_NO\",\"details\":{\"invoice_details\":{\"invoice_no\":\"$INVOICE_NO\",\"status\":\"paid\",\"total_payable_lkr\":4820.5,\"invoice_url\":\"https://slpacargoapi.slpa.lk/invoices/$INVOICE_NO.pdf\",\"invoice_paid_at\":\"$(date -u +%FT%TZ)\",\"payment_receipt\":{\"payment_receipt\":\"https://slpacargoapi.slpa.lk/receipts/$INVOICE_NO.pdf\",\"paid_amount\":4820.5,\"paid_datetime\":\"$(date -u +%FT%TZ)\"}}}}"
}

case "${1:-all}" in
    approve)      approve ;;
    reject-clerk) reject_clerk ;;
    invoice)      invoice ;;
    all)          approve; sleep 2; invoice ;;
    *) echo "usage: $0 {approve|reject-clerk|invoice|all}"; exit 2 ;;
esac
