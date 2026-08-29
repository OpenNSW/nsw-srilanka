package ecdn

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
)

// ErrNotAssembled reports a declaration this service could not render from the
// trader's form. It is a local failure, not SLPA's answer to anything: nothing
// is sent, and the trader is told what is missing here rather than shown
// whatever the CMS says about a document it never received.
var ErrNotAssembled = errors.New("slpa ecdn: the declaration could not be assembled")

// unsendable is the body of a request that must not be made. Encode returns the
// assembly failure, and remote.Client encodes the body before it opens the
// connection — so returning one of these from BuildRequest stops the call while
// keeping the outcome on the single path the plugin already has for reporting
// one: Interpret's callErr.
type unsendable struct{ err error }

func (b unsendable) Encode() ([]byte, string, error) { return nil, "", b.err }

// UploadRequest is the CMS upload body: the declaration travels as a string in
// one field, and the CMS validates it server-side.
//
// Identity is not in the body. SLPA authenticates the caller with a bearer token
// from its own token endpoint, and identifies which client company the
// submission is for with the slpacmsuser-key header — see ClientKeyHeader.
type UploadRequest struct {
	XMLPayload string `json:"xml_payload"`
}

// Interpreter renders the trader's form as the ECDN document and reads the CMS's
// answer back as an acceptance flag plus a trader-facing message.
type Interpreter struct{}

// NewInterpreter returns the ECDN interpreter.
func NewInterpreter() *Interpreter { return &Interpreter{} }

// BuildRequest renders the form as the ECDN document and wraps it for upload.
//
// A document that cannot be assembled is not sent. The contract has no error
// return, so the failure travels as a body that refuses to encode: the call is
// abandoned before anything leaves this service, and Interpret reports the local
// reason to the trader.
//
// Uploading an empty declaration instead would put a meaningless document in
// front of a live cargo system and answer the trader with whatever SLPA said
// about it — a rejection describing their submission as empty when the real
// fault is on our side, and one their form gives them no way to act on.
//
// The form already requires every field this needs and at least one container,
// so this is a defensive path rather than an expected one.
func (i *Interpreter) BuildRequest(inputs map[string]any) remote.Body {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		slog.Error("slpa ecdn: task inputs carry no payload form; the declaration will not be sent")
		return unsendable{err: fmt.Errorf("%w: the submitted form did not reach this step", ErrNotAssembled)}
	}

	doc, err := BuildXML(form)
	if err != nil {
		slog.Error("slpa ecdn: declaration could not be assembled; the declaration will not be sent", "error", err)
		return unsendable{err: fmt.Errorf("%w: %v", ErrNotAssembled, err)}
	}
	return remote.JSONBody{V: UploadRequest{XMLPayload: doc}}
}

// BuildHeaders presents the client key the CMS identifies the submission by.
// The key, the header and the shape expected of it are the same on every SLPA
// endpoint, so they live in the cms package.
func (i *Interpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa ecdn")
}

// Interpret reports whether the CMS accepted the declaration and captures the
// fields worth recording against the task.
func (i *Interpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	accepted := callErr == nil && !cms.HasErrors(body) && cms.Accepted(body)

	out := cms.Capture(body,
		"status", "message", "reference", "ecdn_id", "cusdec_serial", "validated_at",
		"error", "errors", "detail",
	)
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

	// A declaration that was never sent: the reason is ours, and saying SLPA
	// refused something they never saw would send the trader looking in the
	// wrong place.
	if errors.Is(callErr, ErrNotAssembled) {
		return "Your cargo declaration could not be prepared for SLPA, so nothing was submitted:\n\n" +
			strings.TrimPrefix(callErr.Error(), ErrNotAssembled.Error()+": ") + outro
	}

	return cms.Failure(callErr, body, intro, outro)
}
