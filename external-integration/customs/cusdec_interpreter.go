// Package customs holds Sri Lanka Customs (SLC Edge) specific integration
// logic layered on top of the generic API-call plugin.
package customs

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// SLC Edge submission statuses that count as accepted.
const (
	statusQueued   = "QUEUED"
	statusAccepted = "ACCEPTED"
)

// capturedFields are the SLC Edge response fields surfaced to the workflow
// (and the UI) — the lifecycle ids plus either-shape error detail.
var capturedFields = []string{
	"edgeId", "status", "acceptedAt", "errors", "nswId",
	"detail", "title", "code", "fieldRef",
}

// CusdecInterpreter adapts the generic API-call plugin to the SLC Edge Customs
// Declaration submission API: it builds the request (injecting a fresh nswId)
// and interprets the response into an acceptance flag plus the SLC fields to
// record (with a trader-facing error message on rejection).
type CusdecInterpreter struct{}

// NewCusdecInterpreter returns the SLC Edge CusDec interpreter.
func NewCusdecInterpreter() *CusdecInterpreter { return &CusdecInterpreter{} }

// BuildRequest sends the mapped "payload" and injects a freshly generated,
// time-ordered, unique nswId (the SLC Edge idempotency key).
func (CusdecInterpreter) BuildRequest(inputs map[string]any) any {
	body, ok := inputs["payload"]
	if !ok {
		return inputs
	}
	if obj, ok := body.(map[string]any); ok {
		obj["nswId"] = newID()
	}
	return body
}

// Interpret reports whether the submission was accepted and captures the SLC
// response fields (and a trader-facing error message on rejection).
func (CusdecInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	accepted := callErr == nil && !hasErrors(resp) && statusIsAccepted(resp)

	out := map[string]any{}
	for _, k := range capturedFields {
		if v, ok := resp[k]; ok {
			out[k] = v
		}
	}
	if !accepted {
		out["error"] = describeFailure(resp)
	}
	return accepted, out
}

// hasErrors reports whether the response carries a non-empty "errors" array.
func hasErrors(resp map[string]any) bool {
	errs, ok := resp["errors"].([]any)
	return ok && len(errs) > 0
}

// statusIsAccepted reports whether a string "status" indicates success; a
// non-string or absent status defers to the HTTP error / errors[].
func statusIsAccepted(resp map[string]any) bool {
	s, ok := resp["status"].(string)
	if !ok {
		return true
	}
	return s == statusQueued || s == statusAccepted
}

// describeFailure builds a trader-facing, markdown message from the SLC Edge
// error shapes: an "errors" array of {code,message,fieldRef}, or a problem+json
// "detail"/"title".
func describeFailure(resp map[string]any) string {
	const intro = "Your customs declaration was not accepted by Sri Lanka Customs:"
	const outro = "\n\nPlease correct the highlighted fields and resubmit."

	bullets := validationBullets(resp)
	if len(bullets) == 0 {
		if s := stringField(resp, "detail"); s != "" {
			bullets = []string{"- " + s}
		} else if s := stringField(resp, "title"); s != "" {
			bullets = []string{"- " + s}
		}
	}
	if len(bullets) == 0 {
		return intro + outro
	}
	return intro + "\n\n" + strings.Join(bullets, "\n") + outro
}

// validationBullets renders each entry of the "errors" array as a markdown
// bullet: the message (or code) with the offending field in italics.
func validationBullets(resp map[string]any) []string {
	errs, ok := resp["errors"].([]any)
	if !ok {
		return nil
	}
	bullets := make([]string, 0, len(errs))
	for _, e := range errs {
		m, ok := e.(map[string]any)
		if !ok {
			bullets = append(bullets, "- "+fmt.Sprintf("%v", e))
			continue
		}
		msg, _ := m["message"].(string)
		if msg == "" {
			msg, _ = m["code"].(string)
		}
		if field, _ := m["fieldRef"].(string); field != "" && msg != "" {
			msg = fmt.Sprintf("%s _(%s)_", msg, field)
		}
		if msg != "" {
			bullets = append(bullets, "- "+msg)
		}
	}
	return bullets
}

// stringField returns resp[key] as a string, or "" if absent or not a string.
func stringField(resp map[string]any, key string) string {
	s, _ := resp[key].(string)
	return s
}

// newID returns a fresh time-ordered unique id (UUIDv7), falling back to a
// random UUID.
func newID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	if id, err := uuid.NewRandom(); err == nil {
		return id.String()
	}
	return ""
}
