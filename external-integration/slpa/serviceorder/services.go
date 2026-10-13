package serviceorder

import (
	"log/slog"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// ServicesKey is the output key the sundry services SLPA offers are recorded
// under, and ServicesErrorKey the key a failed lookup's message is recorded
// under. The sundry form's projector reads the first to list the services.
const (
	ServicesKey      = "services"
	ServicesErrorKey = "error"
)

// ServicesInterpreter drives the lookup of the sundry services a trader can
// order: GET /export/services/{fclLcl}/sundry, which the artifact addresses.
//
// The list is SLPA's to change — a service is added, renamed or withdrawn on
// their side — so it is read from the CMS each time a sundry order is raised
// rather than kept here, where it would go stale without anyone noticing until
// an order naming a withdrawn service was refused.
type ServicesInterpreter struct{}

// NewServicesInterpreter returns the sundry-services lookup interpreter.
func NewServicesInterpreter() *ServicesInterpreter { return &ServicesInterpreter{} }

// BuildRequest sends nothing: this is a GET.
func (i *ServicesInterpreter) BuildRequest(map[string]any) remote.Body { return nil }

// BuildHeaders presents the client key: the CMS refuses the lookup without it,
// as it does every other call.
func (i *ServicesInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa sundry services")
}

// Interpret records the sundry services the CMS offers.
//
// The error key is always set — to nil when the lookup succeeded — so that a
// successful retry clears the message an earlier failure left for the trader.
//
// A lookup that returns no services is not a success: with nothing to choose
// from, no sundry order can be raised, and the trader is told so rather than
// shown an empty list.
func (i *ServicesInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	out := map[string]any{ServicesErrorKey: nil}

	if callErr != nil || cms.HasErrors(body) {
		out[ServicesErrorKey] = cms.Failure(callErr, body,
			"SLPA could not list the sundry services:",
			"\n\nPress Look Again to ask SLPA for the list once more.")
		return false, out
	}

	services := sundryServices(resp["data"])
	if len(services) == 0 {
		slog.Warn("slpa sundry services: the CMS answered with no sundry services")
		out[ServicesErrorKey] = "SLPA is not offering any sundry services right now, so a sundry order cannot be raised yet." +
			"\n\nPress Look Again to ask SLPA for the list once more."
		return false, out
	}

	out[ServicesKey] = services
	return true, out
}

// sundryServices reads the services out of the CMS's list, in the order it sent
// them: the ID an order line names, and the name a trader picks it by. An entry the CMS marks as not a sundry service, or one missing its ID or
// name, is left out: it would be an option no order could use, or one the trader
// could not read.
func sundryServices(data any) []map[string]any {
	rows := fields.Rows(data)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if sundry, ok := row["is_sundry"].(bool); ok && !sundry {
			continue
		}
		id := fields.String(row, "service_id")
		name := fields.String(row, "service_name")
		if id == "" || name == "" {
			continue
		}
		out = append(out, map[string]any{"service_id": id, "service_name": name})
	}
	return out
}
