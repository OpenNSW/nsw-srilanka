package ecdn

import (
	"log/slog"
	"strings"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
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
// calls it the ClientSqid). SLPA issues one per registered company, so it is
// per-consignment rather than per-deployment: it is presented on every call from
// the task's inputs — see BuildHeaders — not configured on the service.
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
func (i *Interpreter) BuildRequest(inputs map[string]any) remote.Body {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		slog.Error("slpa ecdn: task inputs carry no payload form; sending an empty declaration")
		return remote.JSONBody{V: UploadRequest{}}
	}

	doc, err := BuildXML(form)
	if err != nil {
		slog.Error("slpa ecdn: declaration could not be assembled; sending an empty declaration", "error", err)
		return remote.JSONBody{V: UploadRequest{}}
	}
	return remote.JSONBody{V: UploadRequest{XMLPayload: doc}}
}

// ClientKeyInput is the task input the SLPA-issued client key arrives in.
//
// The key belongs to the submitting company, so it travels with the consignment:
// the company profile is propagated into the workflow, the split that spawns an
// agency flow carries it into the branch, and the artifact maps it here. Nothing
// in this package reads a database — which company a submission is filed under is
// a workflow decision, expressed in the artifact rather than in code.
const ClientKeyInput = "client_key"

// BuildHeaders presents the client key the CMS identifies the submission by.
//
// No key means none is sent: filing a declaration against no company is not
// something to guess at, and SLPA answers for itself ("Client identifier header
// 'slpacmsuser-key' is required"), which is a truer message than one invented
// here.
func (i *Interpreter) BuildHeaders(inputs map[string]any) map[string]string {
	key, _ := inputs[ClientKeyInput].(string)
	if key = strings.TrimSpace(key); key == "" {
		slog.Error("slpa ecdn: no SLPA client key on the task inputs; the CMS will refuse the call")
		return nil
	}
	return map[string]string{ClientKeyHeader: key}
}

// Interpret reports whether the CMS accepted the declaration and captures the
// fields worth recording against the task.
func (i *Interpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	accepted := callErr == nil && !cms.HasErrors(body) && cms.Accepted(body)

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

// describeFailure builds the trader-facing message for a declaration the CMS did
// not accept, preferring the CMS's own reasons over anything invented here.
func describeFailure(callErr error, body map[string]any) string {
	const intro = "SLPA did not accept your cargo declaration:"
	const outro = "\n\nPlease correct the details and submit again."

	if reasons := cms.Reasons(body); len(reasons) > 0 {
		return intro + "\n\n" + strings.Join(reasons, "\n") + outro
	}
	if callErr != nil && len(body) == 0 {
		// Either the CMS could not be reached or it answered with something that
		// is not one of its own responses (an HTML error page, say). The
		// difference matters to whoever reads the logs, not to the trader, and
		// claiming a specific cause we have not established would be wrong.
		return "We could not get a usable answer from the SLPA Cargo Management System. Please try again in a few minutes."
	}
	return intro + outro
}
