// Package audit records security-relevant application events as structured
// logs with Category. Bootstrap points LogSink at the OTLP handler only, so
// records are omitted from the process (pod) log.
//
// Call sites depend on Auditor. Bootstrap constructs a Client, registers
// LogSink, and passes the Client as Auditor. Core payment and storage emit
// shared/audit.Event values; CoreAdapter maps those into this package's Event
// vocabulary before they reach the Client.
//
// Authenticated SLC Edge deliveries emit CUSTOMS_EVENT records targeting a
// CUSTOMS_DECLARATION or CARGO_DISPATCH_NOTE by its local record ID. Metadata
// keeps eventType, httpStatus, caseId and available edgeId/documentRef values.
// Success means callback processing was accepted, including duplicate deliveries
// and handled Customs rejections (integrated=false). Unresolved documents leave
// the target empty; malformed/unknown callbacks use CUSTOMS_CALLBACK. Payloads
// and Customs error descriptions are not copied into these audit records.
// Callback event names are allowlisted (UNKNOWN otherwise); callback-controlled
// edgeId and documentRef values over 128 bytes are omitted without changing lookups.
// A callback that arrives early is answered 202 and retried in the background:
// its delivery record carries heldForRetry=true, and a second CUSTOMS_EVENT record
// without httpStatus follows when the retries end, with retryAttempts and
// retryOutcome (applied, refused, or gave_up). A restart drops pending retries
// and their outcome records with them.
//
// SLPA deliveries that pass the signature check emit SLPA_EVENT records with
// actor SERVICE/slpa, since the route carries no IdP token. A record targets the
// SLPA_SERVICE_ORDER by the slug its consignment recorded; metadata keeps
// eventType, httpStatus, caseId and available slug, serviceOrderNo and invoiceNo
// values. Unresolved orders leave the target empty; malformed or unknown events
// use SLPA_CALLBACK. Event names and identifier bounds follow the SLC Edge rules,
// and the order's contents (amounts, receipts, shipper, consignee) are not
// copied. Deliveries that fail the signature check are left to the security log.
//
// RegisterSink is intended for composition-root wiring only. Audit itself is
// nil-receiver safe so optional call sites can leave the dependency unset.
package audit
