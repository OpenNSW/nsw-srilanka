// Package cms reads the answers SLPA's Cargo Management System gives, in the one
// envelope every one of its endpoints uses. Each integration with the CMS phrases
// its own user-facing prose; how an outcome is recognised in the body is the same
// for all of them and lives here.
package cms

import (
	"fmt"
	"log/slog"
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

// --- what every SLPA integration presents and records ------------------------
//
// The CMS answers each of its endpoints in the same envelope and identifies the
// caller the same way on all of them, so these belong to the integration as a
// whole rather than to whichever endpoint was written first.

// ClientKeyInput is the task input the SLPA-issued client key arrives in, and
// ClientKeyHeader the header the CMS identifies the submitting company by (the
// CMS calls it the ClientSqid).
//
// SLPA issues one key per registered company, so it is per-consignment rather
// than per-deployment: the company profile is propagated into the workflow, the
// split that spawns an agency flow carries it into the branch, and the artifact
// maps it onto the task. Nothing here reads a database — which company a
// submission is filed under is a workflow decision, expressed in the artifact
// rather than in code.
const (
	ClientKeyInput  = "client_key"
	ClientKeyHeader = "slpacmsuser-key"
)

// ClientKeyHeaders presents the client key on a call, if the task carries one.
//
// The input is expected to be a string: the artifact maps it from the company
// profile's own field (company.data.slpacmsuser_key), which SLPA issues as an
// opaque string. Anything else — a number, an object, a profile mapped in whole
// by mistake — counts as no key at all rather than being rendered into one,
// because a header built from the wrong shape would file the submission against
// a company nobody chose.
//
// No key means none is sent: submitting against no company is not something to
// guess at, and SLPA answers for itself ("Client identifier header
// 'slpacmsuser-key' is required"), which is a truer message than one invented
// here. integration names the caller in the log, since that is the only place
// the local reason survives.
func ClientKeyHeaders(inputs map[string]any, integration string) map[string]string {
	raw, present := inputs[ClientKeyInput]
	key, isString := raw.(string)

	switch {
	case present && !isString && raw != nil:
		slog.Error(integration+": the SLPA client key on the task inputs is not a string; the CMS will refuse the call",
			"got", fmt.Sprintf("%T", raw))
		return nil
	case strings.TrimSpace(key) == "":
		slog.Error(integration + ": no SLPA client key on the task inputs; the CMS will refuse the call")
		return nil
	}
	return map[string]string{ClientKeyHeader: strings.TrimSpace(key)}
}

// Capture copies the fields worth recording against the task out of a flattened
// body, skipping those the CMS did not send.
//
// Each endpoint names its own keys: what a trader quotes at the terminal differs
// from what a later step reads, and a field absent from one answer must not
// appear as an empty one on the task.
func Capture(body map[string]any, keys ...string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := body[k]; ok {
			out[k] = v
		}
	}
	return out
}

// Unreachable is what a trader is told when the CMS could not be reached, or
// answered with something that is not one of its own responses (an HTML error
// page, say). Which of the two it was matters to whoever reads the logs, not to
// the trader, and naming a cause we have not established would be wrong.
const Unreachable = "We could not get a usable answer from the SLPA Cargo Management System. Please try again in a few minutes."

// Failure builds the trader-facing message for a submission the CMS did not
// accept, preferring the CMS's own reasons over anything invented here.
//
// intro says what was not accepted and outro what to do about it; both belong to
// the calling integration, since only it knows what the trader submitted.
func Failure(callErr error, body map[string]any, intro, outro string) string {
	if reasons := Reasons(body); len(reasons) > 0 {
		return intro + "\n\n" + strings.Join(reasons, "\n") + outro
	}
	if callErr != nil && len(body) == 0 {
		return Unreachable
	}
	return intro + outro
}
