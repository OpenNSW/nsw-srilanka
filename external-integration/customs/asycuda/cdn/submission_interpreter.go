package cdn

import (
	"errors"
	"strings"
)

// SLC Edge submission statuses that count as accepted. RECEIVED is what the
// spec (§7.1.1) and the live endpoint return; the other two are carried over
// from the declaration vocabulary so a service still sending them is not read
// as a rejection.
const (
	statusQueued   = "QUEUED"
	statusAccepted = "ACCEPTED"
	statusReceived = "RECEIVED"
)

// capturedFields are the SLC Edge response fields surfaced to the workflow (and
// the UI). edgeId is the correlation ID the §7.2 integration-result callback is
// matched back to this dispatch note by (§2.1), so losing it here strands the
// note.
var capturedFields = []string{
	"edgeId", "status", "receivedAt",
	"message", "error", "detail", "title", "code", "fieldRef", "errors",
}

// CDNInterpreter adapts the generic API-call plugin to the SLC Edge Cargo
// Dispatch Note submission API (§7.1): it maps the trader form onto the Annex B
// payload and interprets the acknowledgement into an acceptance flag plus the
// fields to record (with a trader-facing error message on rejection).
//
// Unlike the CusDec submission this is a plain JSON body — a CDN carries no
// attachments (§7.1) — so the interpreter implements only BuildRequest and the
// plugin sends it through the ordinary JSON transport.
type CDNInterpreter struct{}

// NewCDNInterpreter returns the SLC Edge CDN submission interpreter.
func NewCDNInterpreter() *CDNInterpreter {
	return &CDNInterpreter{}
}

// BuildRequest maps the trader form onto the Annex B payload.
//
// The Interpreter contract has no error return, so a form that cannot be mapped
// is sent as the buildError itself. The plugin marshals it to a JSON object the
// endpoint rejects, and Interpret recognizes the same failure on the way back
// out and reports the trader-facing message — the same path a rejection takes,
// rather than a silently truncated payload.
func (CDNInterpreter) BuildRequest(inputs map[string]any) any {
	payload, err := buildFromInputs(inputs)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return payload
}

// Interpret reports whether the submission was accepted and captures the SLC
// response fields (and a trader-facing error message on rejection).
func (CDNInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	accepted := callErr == nil && !hasErrors(resp) && statusIsAccepted(resp)

	out := map[string]any{}
	for _, k := range capturedFields {
		if v, ok := resp[k]; ok {
			out[k] = v
		}
	}

	if !accepted {
		out["error"] = describeFailure(callErr, resp)
	}
	return accepted, out
}

func buildFromInputs(inputs map[string]any) (Submission, error) {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		return Submission{}, &buildError{"The dispatch note form could not be read."}
	}
	return BuildPayload(form)
}

// buildError marks a failure to assemble the submission (an unparseable
// declaration reference, an unreadable form) as opposed to a transport failure,
// so describeFailure can surface the message to the trader verbatim.
type buildError struct{ msg string }

func (e *buildError) Error() string { return e.msg }

// hasErrors reports whether the response carries error detail. §4.4 defines
// errors as a segment-keyed object that is empty on success, but the submission
// acknowledgement predates that shape and some responses still send an array,
// so both are read as a rejection when non-empty.
func hasErrors(resp map[string]any) bool {
	switch errs := resp["errors"].(type) {
	case []any:
		return len(errs) > 0
	case map[string]any:
		return len(errs) > 0
	default:
		return false
	}
}

// statusIsAccepted reports whether the response's "status" is an explicitly
// accepted value. Success statuses are defined by the spec, so anything else —
// including a missing or non-string status (e.g. a problem+json error body) —
// is treated as not accepted.
func statusIsAccepted(resp map[string]any) bool {
	s, ok := resp["status"].(string)
	if !ok {
		return false
	}
	return s == statusQueued || s == statusAccepted || s == statusReceived
}

// describeFailure builds a trader-facing, markdown message for a rejected
// submission. It prefers the SLC Edge error detail in the response body — an
// {"error": "<reason>"} string, a §4.4 segment-keyed errors object, an errors
// array of {code,message,fieldRef}, or a problem+json "detail"/"title" — and
// falls back to distinguishing a transport failure from an unexplained
// rejection.
func describeFailure(callErr error, resp map[string]any) string {
	const intro = "Your cargo dispatch note was not accepted by Sri Lanka Customs:"
	const outro = "\n\nPlease correct the highlighted fields and resubmit."

	// A local assembly failure never reached Customs, so it is reported on its
	// own terms rather than as their rejection.
	var be *buildError
	if errors.As(callErr, &be) {
		return "Your cargo dispatch note could not be submitted:\n\n- " + be.msg
	}

	bullets := validationBullets(resp)
	if len(bullets) == 0 {
		for _, key := range []string{"error", "detail", "title"} {
			if s := stringField(resp, key); s != "" {
				bullets = []string{"- " + s}
				break
			}
		}
	}
	if len(bullets) > 0 {
		return intro + "\n\n" + strings.Join(bullets, "\n") + outro
	}

	// A rejection carrying no detail is still a rejection: only treat this as
	// unreachable when the response body was empty too, so a 4xx with an
	// unrecognized shape is not reported to the trader as a network problem.
	if callErr != nil && len(resp) == 0 {
		return "We could not reach Sri Lanka Customs to submit your cargo dispatch note. Please try again in a few minutes."
	}
	return intro + outro
}
