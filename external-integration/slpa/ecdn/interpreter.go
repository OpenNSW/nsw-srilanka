package ecdn

import (
	"context"
	"errors"
	"fmt"
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
// calls it the ClientSqid). It is per-company rather than per-deployment, so it
// is resolved from the submitting company's profile on every upload rather than
// configured once.
const ClientKeyHeader = "slpacmsuser-key"

// ClientKeyInput is the task input the SLPA-issued client key arrives in.
//
// The key belongs to the submitting company, so it travels with the consignment:
// the company profile is propagated into the workflow, the split that spawns an
// agency flow carries it into the branch, and the artifact maps it here. Nothing
// in this package reads a database — which company a submission is filed under is
// a workflow decision, expressed in the artifact rather than in code.
const ClientKeyInput = "client_key"

// Interpreter renders the trader's form as the ECDN document, presents the client
// key the CMS identifies the submission by, and reads the CMS's answer back as an
// acceptance flag plus a trader-facing message.
//
// It implements the API-call plugin's CallInterpreter, because that key is a
// header taken from the task inputs rather than a value configurable per service.
// That is the only reason this needs more than the plain JSON path.
type Interpreter struct{}

// NewInterpreter returns the ECDN interpreter.
func NewInterpreter() *Interpreter { return &Interpreter{} }

// BuildCall assembles the upload: the declaration as the body, and the client key
// as the header SLPA identifies the submitting company by.
func (i *Interpreter) BuildCall(_ context.Context, _ string, inputs map[string]any) (any, map[string]string, error) {
	body, err := buildBody(inputs)
	if err != nil {
		return nil, nil, err
	}

	clientKey, _ := inputs[ClientKeyInput].(string)
	clientKey = strings.TrimSpace(clientKey)
	if clientKey == "" {
		return nil, nil, &buildError{
			"Your company is not registered with the SLPA Cargo Management System, so this declaration cannot be submitted. Please contact SLPA to be issued a CMS client key."}
	}

	return body, map[string]string{
		ClientKeyHeader: clientKey,
		"Accept":        "application/json",
	}, nil
}

// buildBody renders the form as the ECDN document and wraps it for upload.
func buildBody(inputs map[string]any) (UploadRequest, error) {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		return UploadRequest{}, &buildError{"The cargo declaration form could not be read."}
	}

	doc, err := BuildXML(form)
	if err != nil {
		return UploadRequest{}, err
	}
	return UploadRequest{XMLPayload: doc}, nil
}

// Interpret reports whether the CMS accepted the declaration and captures the
// fields worth recording against the task.
func (i *Interpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	accepted := callErr == nil && !hasErrors(resp) && statusIsAccepted(resp)

	out := map[string]any{}
	for _, k := range []string{"status", "message", "reference", "ecdn_id", "error", "errors", "detail"} {
		if v, ok := resp[k]; ok {
			out[k] = v
		}
	}
	if !accepted {
		out["error"] = describeFailure(callErr, resp)
	}
	return accepted, out
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
	return stringField(resp, "error") != ""
}

// describeFailure builds the trader-facing message for a declaration the CMS did
// not accept, preferring the CMS's own reasons over anything invented here.
func describeFailure(callErr error, resp map[string]any) string {
	const intro = "SLPA did not accept your cargo declaration:"
	const outro = "\n\nPlease correct the details and submit again."

	// A local assembly failure never reached SLPA, so it is reported on its own
	// terms rather than as their rejection.
	var be *buildError
	if errors.As(callErr, &be) {
		return "Your cargo declaration could not be submitted:\n\n- " + be.msg
	}

	if bullets := reasonBullets(resp); len(bullets) > 0 {
		return intro + "\n\n" + strings.Join(bullets, "\n") + outro
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
