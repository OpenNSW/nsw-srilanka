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
// envelope rather than the request: a served request is status 1 whether or not
// it was accepted.
//
//	{"data": {"status": "ACCEPTED", "id": "…"}, "status": 1, "openapi": "3.0.3"}
//
// The nested fields win over the envelope's, so "status" means the outcome
// everywhere below. Envelope-level fields are kept as the fallback, because a
// request the CMS refuses outright reports it there with no "data" to nest under.
func Flatten(resp map[string]any) map[string]any {
	nested, ok := resp["data"].(map[string]any)
	if !ok {
		return resp
	}

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
// success is not by itself an acceptance. An explicit success is required:
// anything unrecognized counts as a rejection, because treating an unstored
// submission as stored is the worse failure.
func Accepted(body map[string]any) bool {
	switch strings.ToUpper(String(body, "status")) {
	case "SUCCESS", "ACCEPTED", "OK", "VALID", "PROCESSED", "CREATED":
		return true
	}
	// Some responses carry no status string and signal success with a flag.
	if ok, isBool := body["success"].(bool); isBool {
		return ok
	}
	return false
}

// HasErrors reports whether a flattened body carries reasons for a refusal, in
// any of the three shapes the CMS uses: a list, a field-keyed map, or one object
// under "error".
func HasErrors(body map[string]any) bool {
	switch errs := body["errors"].(type) {
	case []any:
		return len(errs) > 0
	case map[string]any:
		return len(errs) > 0
	}
	if obj, ok := body["error"].(map[string]any); ok {
		return len(obj) > 0
	}
	return String(body, "error") != ""
}

// Reasons renders the CMS's own reasons for a refusal as markdown bullets, or
// nil when the body carries none. Preferring their wording over anything
// invented here is the point: their messages name the field.
func Reasons(body map[string]any) []string {
	if bullets := listedReasons(body); len(bullets) > 0 {
		return bullets
	}
	if bullets := objectReasons(body); len(bullets) > 0 {
		return bullets
	}
	// A refusal with no structure at all still usually says something.
	for _, key := range []string{"error", "message", "detail"} {
		if s := String(body, key); s != "" {
			return []string{"- " + s}
		}
	}
	return nil
}

// listedReasons renders the per-field shapes: a list of errors, or a map keyed by
// the field each one is about.
func listedReasons(body map[string]any) []string {
	switch errs := body["errors"].(type) {
	case []any:
		bullets := make([]string, 0, len(errs))
		for _, e := range errs {
			switch v := e.(type) {
			case string:
				bullets = append(bullets, "- "+v)
			case map[string]any:
				if msg := describe(v); msg != "" {
					bullets = append(bullets, "- "+msg)
				}
			}
		}
		return bullets
	case map[string]any:
		// Sorted so the same refusal reads the same way twice; Go's map order
		// would otherwise reshuffle it on every render.
		fields := make([]string, 0, len(errs))
		for field := range errs {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		bullets := make([]string, 0, len(fields))
		for _, field := range fields {
			if msg, ok := errs[field].(string); ok && msg != "" {
				bullets = append(bullets, fmt.Sprintf("- %s _(%s)_", msg, field))
			}
		}
		return bullets
	}
	return nil
}

// objectReason renders the object the CMS puts under "error" when it refuses a
// request outright — {"code": "…", "message": "…"} — as opposed to the per-field
// shapes above. Without this the reason is dropped: the object is not a string,
// so a lookup for a string-valued "error" finds nothing.
//
// The code travels with the message because it is what SLPA support asks for
// when a submission is refused for a reason the user cannot act on alone.
func objectReasons(body map[string]any) []string {
	obj, ok := body["error"].(map[string]any)
	if !ok {
		return nil
	}

	// A validation failure carries the per-field reasons under "details" and only
	// a summary in "message" ("The given request data was invalid."), so the
	// details are what is worth showing — they name the field and the rule.
	if bullets := detailReasons(obj); len(bullets) > 0 {
		return bullets
	}
	if reason := describe(obj); reason != "" {
		return []string{"- " + reason}
	}
	return nil
}

// detailReasons renders the per-field messages of a validation failure. A field
// can carry several, and the CMS sends them as a list; sorted by field so the
// same refusal reads the same way twice.
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
		for _, msg := range messages(details[field]) {
			bullets = append(bullets, fmt.Sprintf("- %s _(%s)_", msg, field))
		}
	}
	return bullets
}

// messages reads one field's reasons, which arrive as a list or as a lone string.
func messages(value any) []string {
	switch v := value.(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

// describe renders one error object: its message, qualified by the field it is
// about or the code that identifies it.
func describe(obj map[string]any) string {
	msg := String(obj, "message")
	if msg == "" {
		msg = String(obj, "description")
	}
	if msg == "" {
		return ""
	}
	for _, qualifier := range []string{"field", "code"} {
		if q := String(obj, qualifier); q != "" {
			return fmt.Sprintf("%s _(%s)_", msg, q)
		}
	}
	return msg
}

// String reads a trimmed string field, or "" when it is absent or not a string.
func String(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}
