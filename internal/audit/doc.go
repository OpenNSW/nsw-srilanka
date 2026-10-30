// Package audit records security-relevant application events as structured
// logs with Category. Bootstrap points LogSink at the OTLP handler only, so
// records are omitted from the process (pod) log.
//
// Call sites depend on Auditor. Bootstrap constructs a Client, registers
// LogSink, and passes the Client as Auditor. Core payment and storage emit
// shared/audit.Event values; CoreAdapter maps those into this package's Event
// vocabulary before they reach the Client.
//
// RegisterSink is intended for composition-root wiring only. Audit itself is
// nil-receiver safe so optional call sites can leave the dependency unset.
package audit
