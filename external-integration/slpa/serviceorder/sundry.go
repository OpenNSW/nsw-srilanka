package serviceorder

import (
	"fmt"
	"log/slog"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// SundryRequest is the create-order body for a sundry (add-on) order.
//
// It goes to the same endpoint as a standard order but names the invoice of the
// order it adds to instead of a declaration: the CMS finds the declaration, and
// derives the cargo type, from that invoice. Each line is a sundry service — a
// crane, a forklift, a weighing — against one of the declaration's containers.
type SundryRequest struct {
	ParentInvoiceNo string       `json:"parent_invoice_no"`
	SundryInvoice   bool         `json:"sundry_invoice"`
	AdditionalNote  string       `json:"additional_note,omitempty"`
	Containers      []SundryLine `json:"containers"`
}

// SundryLine is one sundry service ordered against a container. The container's
// type, size and volume are already on the parent order, so only the container
// number is sent to say which one the service is for.
type SundryLine struct {
	ContainerNo string `json:"container_no"`
	ServiceID   string `json:"service_id"`
	Quantity    int    `json:"quantity"`
	LCLStatus   bool   `json:"lcl_status"`
}

// BuildSundry assembles a sundry order from the sundry form: the parent invoice
// number the trader entered, and the services chosen per container.
func BuildSundry(form map[string]any) (SundryRequest, error) {
	if len(form) == 0 {
		return SundryRequest{}, &buildError{"The sundry order form could not be read."}
	}

	req := SundryRequest{
		ParentInvoiceNo: fields.String(form, "parentInvoiceNo"),
		SundryInvoice:   true,
		AdditionalNote:  fields.String(form, "additionalNote"),
	}
	if req.ParentInvoiceNo == "" {
		return SundryRequest{}, &buildError{
			"Enter the invoice number of the service order this sundry order adds to."}
	}

	lines, err := buildSundryLines(form)
	if err != nil {
		return SundryRequest{}, err
	}
	req.Containers = lines
	return req, nil
}

// buildSundryLines maps the form's service lines to the order's.
func buildSundryLines(form map[string]any) ([]SundryLine, error) {
	rows, _ := form["lines"].([]any)
	if len(rows) == 0 {
		return nil, &buildError{"Add at least one sundry service before raising a sundry order."}
	}

	lines := make([]SundryLine, 0, len(rows))
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, &buildError{fmt.Sprintf("Line %d could not be read.", i+1)}
		}

		containerNo := fields.String(row, "containerNo")
		if containerNo == "" {
			return nil, &buildError{fmt.Sprintf("Enter the container number for line %d.", i+1)}
		}

		serviceID := fields.String(row, "serviceId")
		if serviceID == "" {
			return nil, &buildError{fmt.Sprintf("Choose a sundry service for container %s.", containerNo)}
		}

		quantity := fields.Integer(row, "quantity")
		if quantity < 1 {
			return nil, &buildError{fmt.Sprintf("Enter a quantity of at least 1 for container %s.", containerNo)}
		}

		lines = append(lines, SundryLine{
			ContainerNo: containerNo,
			ServiceID:   serviceID,
			Quantity:    quantity,
			LCLStatus:   false,
		})
	}
	return lines, nil
}

// SundryInterpreter raises a sundry order. It differs from Interpreter only in
// the body it builds and the message a refusal opens with: the CMS answers both
// kinds of order the same way, with an order number for one it has raised.
type SundryInterpreter struct {
	Interpreter
}

// NewSundryInterpreter returns the sundry-order interpreter.
func NewSundryInterpreter() *SundryInterpreter { return &SundryInterpreter{} }

// BuildRequest assembles the sundry order from the mapped form.
//
// As with a standard order, the contract cannot fail a call, so an order that
// cannot be assembled is sent with no lines: the CMS refuses it with its own
// reason, which is what the trader sees. The form requires every field read here,
// so this is a defensive path.
func (i *SundryInterpreter) BuildRequest(inputs map[string]any) remote.Body {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		slog.Error("slpa sundry order: task inputs carry no payload form; sending an empty order")
		return remote.JSONBody{V: SundryRequest{SundryInvoice: true}}
	}

	req, err := BuildSundry(form)
	if err != nil {
		slog.Error("slpa sundry order: order could not be assembled; sending an empty order", "error", err)
		return remote.JSONBody{V: SundryRequest{
			ParentInvoiceNo: fields.String(form, "parentInvoiceNo"),
			SundryInvoice:   true,
		}}
	}
	return remote.JSONBody{V: req}
}

// BuildHeaders presents the trader's CMS client key, as for a standard order.
func (i *SundryInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa sundry order")
}

// Interpret reports whether the CMS raised the sundry order.
func (i *SundryInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	return interpret(callErr, resp, "SLPA did not raise your sundry order:")
}
