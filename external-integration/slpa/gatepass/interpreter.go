// Package gatepass issues the export container gate pass SLPA's Cargo
// Management System prints, one per consolidated container.
//
// The pass is what the haulier presents at the terminal gate, so it carries the
// truck and driver the container leaves on and the seal it leaves under. None of
// that is known to SLPA or recorded anywhere earlier in the flow, so the trader
// supplies it per container at this step.
package gatepass

import (
	"log/slog"
	"strings"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// Request is the gate-pass body the CMS reads.
type Request struct {
	ContainerNo string `json:"container_no"`
	TruckNo     string `json:"truck_no"`
	DriverName  string `json:"driver_name"`
	SealNo      string `json:"seal_no"`
}

// Interpreter turns the trader's haulage details into a gate-pass call and reads
// the issued pass back.
//
// The service order's slug is part of the endpoint's path rather than its body,
// so the artifact declares the path with a {slug} placeholder and the plugin
// fills it from the same input the rest of the flow carries.
type Interpreter struct{}

// NewInterpreter returns the gate-pass interpreter.
func NewInterpreter() *Interpreter { return &Interpreter{} }

// BuildRequest assembles the pass from the trader's form and the container this
// branch is about.
//
// The container number comes from the consolidation step rather than the form:
// the trader is issuing a pass for a container SLPA has already paired, and
// letting it be typed again would allow a pass for a container that was never
// consolidated — which the CMS refuses with a 422 the trader cannot act on.
func (i *Interpreter) BuildRequest(inputs map[string]any) remote.Body {
	form, _ := inputs["payload"].(map[string]any)
	if form == nil {
		slog.Error("slpa gate pass: task inputs carry no form; sending an empty request")
		form = map[string]any{}
	}

	req := Request{
		ContainerNo: fields.String(inputs, "container_no"),
		TruckNo:     fields.String(form, "truck_no"),
		DriverName:  fields.String(form, "driver_name"),
		SealNo:      fields.String(form, "seal_no"),
	}
	if req.ContainerNo == "" {
		// Fall back to the form only if the branch carries no container, so a
		// misconfigured mapping is still visible in the CMS's own answer rather
		// than as a silently blank field.
		req.ContainerNo = fields.String(form, "container_no")
	}
	return remote.JSONBody{V: req}
}

// BuildHeaders presents the client key the CMS identifies the company by. The
// key, the header and the shape expected of it are the same on every SLPA
// endpoint, so they live in the cms package.
func (i *Interpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa gate pass")
}

// Interpret reports whether the pass was issued and captures what the haulier
// presents at the gate.
func (i *Interpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	// The pass number and its barcode are the whole point of the call: they are
	// what the terminal scans, so they are recorded even before anything
	// downstream reads them. gate_pass_url is the printable pass itself, which
	// is what the haulier actually carries to the gate.
	out := cms.Capture(body,
		"gate_pass_id", "gate_pass_no", "container_no", "truck_no", "driver_name",
		"seal_no", "barcode", "status", "issued_at", "gate_pass_url", "message",
	)

	issued := callErr == nil && !cms.HasErrors(body) && cms.String(body, "gate_pass_no") != ""
	if !issued {
		out["error"] = describeFailure(callErr, body)
		return false, out
	}
	return true, out
}

// describeFailure builds the trader-facing message for a pass the CMS did not
// issue.
//
// The one refusal worth naming is the unpaid invoice: the CMS allows a pass only
// against a paid service order, and we can run ahead of its own paid flag even
// though our flow has seen the payment webhook. That is a wait, not a mistake in
// what the trader entered, and saying so keeps them from re-entering correct
// details.
func describeFailure(callErr error, body map[string]any) string {
	const intro = "SLPA did not issue the gate pass:"
	const outro = "\n\nPlease check the details and try again."

	reasons := cms.Reasons(body)
	if mentionsUnpaid(reasons) {
		return "SLPA has not registered the payment for this service order yet, so it will not issue a gate pass.\n\n" +
			strings.Join(reasons, "\n") +
			"\n\nThis usually clears within a few minutes of the payment being recorded. Try again shortly."
	}
	return cms.Failure(callErr, body, intro, outro)
}

// mentionsUnpaid reports whether the CMS refused for want of payment. Matched on
// its words rather than a status code, since the endpoint answers 422 for an
// invalid container too.
func mentionsUnpaid(reasons []string) bool {
	for _, reason := range reasons {
		lower := strings.ToLower(reason)
		if strings.Contains(lower, "unpaid") || strings.Contains(lower, "not paid") || strings.Contains(lower, "payment") {
			return true
		}
	}
	return false
}
