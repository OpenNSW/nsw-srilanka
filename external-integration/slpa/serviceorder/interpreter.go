package serviceorder

import (
	"log/slog"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// Interpreter turns the trader's service selection into a create-order call and
// reads the CMS's answer back as an acceptance flag plus a trader-facing message.
//
// Identity is not in the body: SLPA authenticates the caller with a bearer token
// and identifies the submitting company with the slpacmsuser-key header, which
// the workflow carries and BuildHeaders presents. See the ecdn package for the
// same arrangement on the declaration itself.
type Interpreter struct{}

// NewInterpreter returns the service-order interpreter.
func NewInterpreter() *Interpreter { return &Interpreter{} }

// BuildRequest assembles the order from the mapped form.
//
// The contract has no error return, so an order that cannot be assembled is sent
// with no lines on it: the CMS validates the request and answers with its own
// reason, which is what the trader is shown. The form requires a service against
// every line, so this is a defensive path — hence the log, which is the only place
// the local reason survives.
func (i *Interpreter) BuildRequest(inputs map[string]any) remote.Body {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		slog.Error("slpa service order: task inputs carry no payload form; sending an empty order")
		return remote.JSONBody{V: Request{}}
	}

	req, err := Build(form)
	if err != nil {
		slog.Error("slpa service order: order could not be assembled; sending an empty order", "error", err)
		return remote.JSONBody{V: Request{CusdecNo: fields.String(form, "cusdecNo")}}
	}
	return remote.JSONBody{V: req}
}

// BuildHeaders presents the client key the CMS identifies the submission by.
// The key, the header and the shape expected of it are the same on every SLPA
// endpoint, so they live in the cms package.
func (i *Interpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa service order")
}

// Interpret reports whether the CMS raised the order and captures what identifies
// it afterwards.
func (i *Interpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	accepted := callErr == nil && !cms.HasErrors(body) && raised(body)

	// The identifiers SLPA hands back are what a trader quotes at the terminal
	// and what any later sundry order keys off, so they are recorded even though
	// nothing downstream reads them yet.
	out := cms.Capture(body,
		"status", "message", "service_order_no", "slug", "invoice_no", "parent_invoice_no",
		"cusdec_serial", "total_lkr", "total_usd", "created_at", "reason",
		"error", "errors", "detail",
	)
	if !accepted {
		out["error"] = describeFailure(callErr, body)
	}
	return accepted, out
}

// raised reports whether the CMS created the order.
//
// Unlike the declaration upload, this endpoint answers with the order's own
// workflow state rather than a verdict — "client_new_actclk" for one it has just
// created — so a fixed list of success words cannot recognise it, and guessing at
// their state machine would break on the next state they add. What is unambiguous
// is the order number: the CMS issues one only for an order that exists, so that
// is what is read.
//
// cms.Accepted is still consulted, so an endpoint that answers with a plain
// verdict is honoured too, and an error in the body is checked by the caller
// either way — a refusal that happens to carry a number is not read as a
// success.
func raised(body map[string]any) bool {
	if cms.String(body, "service_order_no") != "" {
		return true
	}
	return cms.Accepted(body)
}

// describeFailure builds the trader-facing message for an order the CMS did not
// raise, preferring the CMS's own reasons over anything invented here.
func describeFailure(callErr error, body map[string]any) string {
	const intro = "SLPA did not raise your service order:"
	const outro = "\n\nPlease correct the details and submit again."

	return cms.Failure(callErr, body, intro, outro)
}
