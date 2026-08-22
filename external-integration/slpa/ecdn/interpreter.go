package ecdn

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// UploadRequest is the CMS upload body: the declaration travels as a string in
// one field, and the CMS validates it server-side.
//
// Identity is not in the body. SLPA authenticates the caller with a bearer token
// from its own token endpoint, and identifies which client company the
// submission is for with the slpacmsuser-key header — see ClientKeyHeader.
type UploadRequest struct {
	XMLPayload string `json:"xml_payload"`
}

// ClientKeyHeader carries the SLPA-issued client company identifier (the CMS
// calls it the ClientSqid). It is the same for every submission this deployment
// makes, so it is declared on the slpa service in services.json — as
// "slpacmsuser-key": "env:SLPA_CMS_CLIENT_KEY" — and remote.Manager applies it
// to every call. Nothing in this package sends it: a value that is a property of
// the deployment does not belong in a request builder.
//
// This constant documents the header the configuration must use; it is not read
// here.
const ClientKeyHeader = "slpacmsuser-key"

// Interpreter renders the trader's form as the ECDN document and reads the CMS's
// answer back as an acceptance flag plus a trader-facing message.
type Interpreter struct{}

// NewInterpreter returns the ECDN interpreter.
func NewInterpreter() *Interpreter { return &Interpreter{} }

// BuildRequest renders the form as the ECDN document and wraps it for upload.
//
// The contract has no error return, so a document that cannot be assembled is
// sent as an empty declaration: the CMS validates the document and answers with
// its own reason, which is what the trader is shown. The form the trader submits
// already requires every field this needs and at least one container, so this is
// a defensive path rather than an expected one — hence the log, which is the only
// place the local reason survives.
func (i *Interpreter) BuildRequest(inputs map[string]any) any {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		slog.Error("slpa ecdn: task inputs carry no payload form; sending an empty declaration")
		return UploadRequest{}
	}

	doc, err := BuildXML(form)
	if err != nil {
		slog.Error("slpa ecdn: declaration could not be assembled; sending an empty declaration", "error", err)
		return UploadRequest{}
	}
	return UploadRequest{XMLPayload: doc}
}

// Interpret reports whether the CMS accepted the declaration and captures the
// fields worth recording against the task.
func (i *Interpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := flattenEnvelope(resp)
	accepted := callErr == nil && !hasErrors(body) && statusIsAccepted(body)

	out := map[string]any{}
	for _, k := range []string{
		"status", "message", "reference", "ecdn_id", "cusdec_serial", "validated_at",
		"error", "errors", "detail",
	} {
		if v, ok := body[k]; ok {
			out[k] = v
		}
	}
	if !accepted {
		out["error"] = describeFailure(callErr, body)
	}
	return accepted, out
}

// flattenEnvelope lifts the CMS's response envelope into the flat map the rest of
// this file reads.
//
// The CMS answers with the outcome nested under "data" and a top-level "status"
// that describes the envelope rather than the declaration — a served request is
// status 1 whether or not the document was accepted:
//
//	{"data": {"status": "ACCEPTED", "cusdec_serial": "...", "validated_at": "..."},
//	 "status": 1, "openapi": "3.0.3"}
//
// The nested fields therefore win over the envelope's, so "status" means the
// declaration's verdict everywhere below. Envelope-level fields are kept as the
// fallback, because a request the CMS refuses outright reports it there with no
// "data" to nest it under.
func flattenEnvelope(resp map[string]any) map[string]any {
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

// statusIsAccepted reports whether the CMS answered with a success.
//
// The endpoint returns 200 with a body describing the outcome, so a transport
// success is not by itself an acceptance. An explicit success status is required:
// anything unrecognized is treated as a rejection, because recording a Service
// Order as raisable when the CMS has not stored the declaration is the worse
// failure.
func statusIsAccepted(resp map[string]any) bool {
	switch s := strings.ToUpper(strings.TrimSpace(stringField(resp, "status"))); s {
	case "SUCCESS", "ACCEPTED", "OK", "VALID", "PROCESSED":
		return true
	}
	// Some responses carry no status field and signal success with a flag.
	if ok, isBool := resp["success"].(bool); isBool {
		return ok
	}
	return false
}

func hasErrors(resp map[string]any) bool {
	switch errs := resp["errors"].(type) {
	case []any:
		return len(errs) > 0
	case map[string]any:
		return len(errs) > 0
	}
	if obj, ok := resp["error"].(map[string]any); ok {
		return len(obj) > 0
	}
	return stringField(resp, "error") != ""
}

// describeFailure builds the trader-facing message for a declaration the CMS did
// not accept, preferring the CMS's own reasons over anything invented here.
func describeFailure(callErr error, resp map[string]any) string {
	const intro = "SLPA did not accept your cargo declaration:"
	const outro = "\n\nPlease correct the details and submit again."

	if bullets := reasonBullets(resp); len(bullets) > 0 {
		return intro + "\n\n" + strings.Join(bullets, "\n") + outro
	}
	if bullet := errorObjectBullet(resp); bullet != "" {
		return intro + "\n\n" + bullet + outro
	}
	for _, key := range []string{"error", "message", "detail"} {
		if s := stringField(resp, key); s != "" {
			return intro + "\n\n- " + s + outro
		}
	}
	if callErr != nil && len(resp) == 0 {
		return "We could not reach the SLPA Cargo Management System. Please try again in a few minutes."
	}
	return intro + outro
}

// errorObjectBullet renders the object the CMS puts under "error" when it
// refuses the request outright — {"code": "…", "message": "…"} — as opposed to
// the field-keyed "errors" map it returns for a declaration it has read and
// validated. Without this the reason is dropped: the object is not a string, so
// the string-valued fields below find nothing and the trader is told only that
// the declaration was not accepted.
//
// The code travels with the message because it is what SLPA support asks for
// when a submission is refused for a reason the trader cannot act on alone.
func errorObjectBullet(resp map[string]any) string {
	obj, ok := resp["error"].(map[string]any)
	if !ok {
		return ""
	}

	msg := stringField(obj, "message")
	if msg == "" {
		msg = stringField(obj, "description")
	}
	if msg == "" {
		return ""
	}
	if code := stringField(obj, "code"); code != "" {
		return fmt.Sprintf("- %s _(%s)_", msg, code)
	}
	return "- " + msg
}

func reasonBullets(resp map[string]any) []string {
	switch errs := resp["errors"].(type) {
	case []any:
		bullets := make([]string, 0, len(errs))
		for _, e := range errs {
			switch v := e.(type) {
			case string:
				bullets = append(bullets, "- "+v)
			case map[string]any:
				msg := stringField(v, "message")
				if msg == "" {
					msg = stringField(v, "description")
				}
				if field := stringField(v, "field"); field != "" && msg != "" {
					msg = fmt.Sprintf("%s _(%s)_", msg, field)
				}
				if msg != "" {
					bullets = append(bullets, "- "+msg)
				}
			}
		}
		return bullets
	case map[string]any:
		// Sorted so the trader sees the same message twice for the same
		// rejection; Go's map order would otherwise reshuffle it each render.
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

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}
