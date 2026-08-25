// Package cms reads the answers SLPA's Cargo Management System gives, in the one
// envelope every one of its endpoints uses. Each integration with the CMS phrases
// its own user-facing prose; how an outcome is recognised in the body is the same
// for all of them and lives here.
package cms

import (
	"fmt"
	"sort"
	"strings"
)

// Flatten lifts the CMS's response envelope into a flat map.
//
// The CMS nests the outcome under "data" and uses the top-level "status" for the
// envelope rather than the request: a served request is status 1 when it was
// accepted and 0 when it was not, whatever happened to the submission itself.
//
//	{"data": {"status": "ACCEPTED", "cusdec_serial": "…"}, "status": 1, "openapi": "3.0.3"}
//
// The nested fields win over the envelope's, so "status" means the outcome
// everywhere below. Envelope-level fields are kept as the fallback, because a
// refused request carries its reason there with no "data" to nest under.
//
// It always returns a map the caller owns and can read from: a nil response —
// what a call that never reached the service leaves behind — reads as an empty
// body, and the envelope is copied rather than handed back, so nothing here can
// change the response the caller decoded. The "data" key itself is dropped, being
// the envelope's container rather than part of the outcome.
func Flatten(resp map[string]any) map[string]any {
	nested, _ := resp["data"].(map[string]any)

	body := make(map[string]any, len(resp)+len(nested))
	for k, v := range resp {
		if k == "data" {
			continue
		}
		body[k] = v
	}
	for k, v := range nested {
		body[k] = v
	}
	return body
}

// Accepted reports whether a flattened body says the CMS accepted the request.
//
// These endpoints answer 200 with a body describing the outcome, so a transport
// success is not by itself an acceptance, and neither is the envelope's status:
// the accepted declarations carry "ACCEPTED" in the nested outcome. Anything
// unrecognized counts as a refusal, because treating an unstored submission as
// stored is the worse failure.
//
// An endpoint whose acceptance looks different says so for itself — the service
// order answers with the order's own workflow state and an order number, which
// its own interpreter reads.
func Accepted(body map[string]any) bool {
	return strings.EqualFold(String(body, "status"), "ACCEPTED")
}

// HasErrors reports whether a flattened body carries a refusal.
//
// The CMS refuses with one object under "error", holding a code, a message and —
// for a validation failure — the per-field details.
func HasErrors(body map[string]any) bool {
	obj, ok := body["error"].(map[string]any)
	return ok && len(obj) > 0
}

// Reasons renders the CMS's reasons for a refusal as markdown bullets, or nil
// when the body carries none. Preferring their wording over anything invented
// here is the point: their messages name the field and the rule.
//
//	{"error": {"code": "VALIDATION_FAILED",
//	           "message": "The given request data was invalid.",
//	           "details": {"containers.0.cbm": ["The containers.0.cbm must be at least 0.01."]}}}
func Reasons(body map[string]any) []string {
	obj, ok := body["error"].(map[string]any)
	if !ok {
		return nil
	}

	// A validation failure puts the per-field reasons in "details" and only a
	// summary in "message" ("The given request data was invalid."), so the details
	// are what is worth showing — they name the field and the rule. Every other
	// refusal (a duplicate declaration, a missing client key) says it in
	// "message" alone.
	if bullets := detailReasons(obj); len(bullets) > 0 {
		return bullets
	}

	message := String(obj, "message")
	if message == "" {
		return nil
	}
	// The code travels with the message because it is what SLPA support asks for
	// when a submission is refused for a reason the trader cannot act on alone.
	if code := String(obj, "code"); code != "" {
		return []string{fmt.Sprintf("- %s _(%s)_", message, code)}
	}
	return []string{"- " + message}
}

// detailReasons renders the per-field messages of a validation failure. A field
// can carry several — a service that is both unknown and wrong for the commodity
// reports both — and they are sorted by field so the same refusal reads the same
// way twice.
func detailReasons(obj map[string]any) []string {
	details, ok := obj["details"].(map[string]any)
	if !ok {
		return nil
	}

	fields := make([]string, 0, len(details))
	for field := range details {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	bullets := make([]string, 0, len(fields))
	for _, field := range fields {
		for _, message := range messages(details[field]) {
			bullets = append(bullets, fmt.Sprintf("- %s _(%s)_", message, field))
		}
	}
	return bullets
}

// messages reads one field's reasons, which the CMS sends as a list of strings.
func messages(value any) []string {
	list, ok := value.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// String reads a trimmed string field, or "" when it is absent or not a string.
func String(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}
