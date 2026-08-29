// Package serviceorder raises an Export Service Order against a declaration the
// SLPA Cargo Management System has already validated.
//
// It is the step after the ECDN: the CMS holds the declaration and its
// containers, so an order names the declaration and, per container, the service
// being ordered. Everything else about the cargo is already on their side —
// cargo_type is derived from the CUSDEC record rather than sent — which is why
// the trader is asked for the service and nothing more.
package serviceorder

import (
	"fmt"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// Request is the create-order body.
//
// A standard order names the declaration with CusdecNo; the sundry (add-on)
// order, which keys off a parent invoice instead, is not raised here. Neither is
// an order for loose cargo, which the CMS takes as lcl_containers — only FCL
// declarations are supported for now.
type Request struct {
	CusdecNo       string      `json:"cusdec_no"`
	SundryInvoice  bool        `json:"sundry_invoice"`
	AdditionalNote string      `json:"additional_note,omitempty"`
	Containers     []Container `json:"containers,omitempty"`
}

// Container is one FCL container and the service ordered for it. The container's
// own details come from the declaration, so they are echoed rather than asked
// for; ServiceID and the volume are the trader's only inputs. The commodity is
// not sent at all — the CMS reads it from the CUSDEC record, and an order that
// names one is held to the CMS's pairing rules ("Dangerous Cargo (DC) commodity
// can only use service DANGEROUS CARGO").
type Container struct {
	ContainerNo   string  `json:"container_no"`
	ContainerType string  `json:"container_type"`
	ContainerSize string  `json:"container_size"`
	ServiceID     string  `json:"service_id"`
	Quantity      int     `json:"quantity"`
	CBM           float64 `json:"cbm"`
	LCLStatus     bool    `json:"lcl_status"`
}

// minCBM is the smallest volume the CMS accepts on a line. Sending less is
// refused with a validation error, so an unfilled volume is caught here where the
// message can name the container.
const minCBM = 0.01

// FCL and LCL are the cargo types a declaration can have. The CMS derives the
// authoritative value from the CUSDEC record; this is read only to refuse what
// cannot be ordered yet.
const (
	FCL = "FCL"
	LCL = "LCL"
)

// Build assembles the order from the service-selection form.
//
// The form carries the declaration's container rows with a service chosen against
// each, so this is mostly a rename into the CMS's field names.
func Build(form map[string]any) (Request, error) {
	if len(form) == 0 {
		return Request{}, &buildError{"The service order form could not be read."}
	}

	req := Request{
		CusdecNo:       fields.String(form, "cusdecNo"),
		AdditionalNote: fields.String(form, "additionalNote"),
	}
	if req.CusdecNo == "" {
		return Request{}, &buildError{
			"This declaration has no SLPA serial yet, so no service order can be raised against it."}
	}

	// Loose cargo is ordered against lcl_containers rather than the declaration's
	// containers, and nothing here builds those, so it is refused rather than
	// ordered as though it were containerised.
	if strings.EqualFold(fields.String(form, "cargoType"), LCL) {
		return Request{}, &buildError{
			"This is an LCL declaration. Raising a service order for loose cargo is not supported yet — contact SLPA to raise it directly."}
	}

	containers, err := buildContainers(form)
	if err != nil {
		return Request{}, err
	}
	req.Containers = containers
	return req, nil
}

// buildContainers maps the form's container rows to the order's.
func buildContainers(form map[string]any) ([]Container, error) {
	rows, _ := form["containers"].([]any)
	if len(rows) == 0 {
		return nil, &buildError{"Add at least one container before raising a service order."}
	}

	containers := make([]Container, 0, len(rows))
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, &buildError{fmt.Sprintf("Container %d could not be read.", i+1)}
		}

		label := containerLabel(row, i)

		serviceID := fields.String(row, "serviceId")
		if serviceID == "" {
			return nil, &buildError{fmt.Sprintf("Choose a service for container %s before submitting.", label)}
		}

		cbm := fields.Number(row, "cbm")
		if cbm < minCBM {
			return nil, &buildError{fmt.Sprintf(
				"Enter the volume in CBM for container %s: SLPA requires at least %.2f.", label, minCBM)}
		}

		containers = append(containers, Container{
			ContainerNo:   fields.String(row, "containerNo"),
			ContainerType: fields.String(row, "containerType"),
			ContainerSize: fields.String(row, "containerSize"),
			ServiceID:     serviceID,
			// One container per row, as the declaration lists them.
			Quantity:  1,
			CBM:       cbm,
			LCLStatus: false,
		})
	}
	return containers, nil
}

// containerLabel names a container in a message: its number when it has one, and
// its position otherwise, so a row the trader has not filled in is still
// identifiable.
func containerLabel(row map[string]any, index int) string {
	if no := fields.String(row, "containerNo"); no != "" {
		return no
	}
	return fmt.Sprintf("#%d", index+1)
}

// buildError marks a failure to assemble the order, as opposed to a transport
// failure or a CMS rejection. Its message is written for a person, though the
// plugin contract Interpreter.BuildRequest implements cannot fail a call, so the
// reason is logged rather than shown.
type buildError struct{ msg string }

func (e *buildError) Error() string { return e.msg }

// --- form accessors -------------------------------------------------------
