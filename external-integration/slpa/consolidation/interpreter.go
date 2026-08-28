package consolidation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// CusdecInput is the task input carrying the CUSDEC serial the lookup is keyed
// on. It is recorded at the ECDN step, so nothing new is collected from the
// trader for this node.
const CusdecInput = "cusdec_serial"

// RowsKey is the output key the trader's form lines are recorded under: one per
// pre-advised container, for them to confirm, change or decline.
const RowsKey = "rows"

// SOContainersKey is the output key the service-order containers are recorded
// under. The save step reads them back to turn the container number the trader
// chose into the sqid the CMS reads.
const SOContainersKey = "so_containers"

// FormKey is the task input the submitted form arrives in.
const FormKey = "payload"

// ConsolidatedKey is the output key listing the containers this step
// consolidated. The gate-pass fan-out is built from it together with the ones
// SLPA had already paired.
const ConsolidatedKey = "consolidated"

// Outcomes of the lookup, which is what the workflow's gateway reads. Three
// rather than a boolean, because "nothing to do" and "nothing can be done" lead
// to opposite places: one is finished, the other needs a person.
const (
	// OutcomeReady means there are containers for the trader to consolidate.
	OutcomeReady = "ready"
	// OutcomeDone means SLPA has already consolidated everything here.
	OutcomeDone = "done"
	// OutcomeBlocked means nothing can be paired yet.
	OutcomeBlocked = "blocked"
)

// FetchInterpreter drives the lookup: GET the containers available for
// consolidation under a CUSDEC serial, and match the two sides the CMS returns.
//
// The matching is done here rather than left to the save step so that what will
// be sent is recorded — and shown to the trader — before anything is written on
// SLPA's side.
type FetchInterpreter struct{}

// NewFetchInterpreter returns the consolidation lookup interpreter.
func NewFetchInterpreter() *FetchInterpreter { return &FetchInterpreter{} }

// BuildRequest sends nothing: this is a GET, and the plugin does not ask for a
// body on a method that cannot carry one. It exists to satisfy the interpreter
// contract.
func (i *FetchInterpreter) BuildRequest(map[string]any) remote.Body { return nil }

// BuildQuery names the CUSDEC serial the lookup is keyed on.
//
// A missing serial is sent as an absent parameter rather than an empty one: the
// CMS answers for itself, which is a truer message than one invented here.
func (i *FetchInterpreter) BuildQuery(inputs map[string]any) url.Values {
	serial := strings.TrimSpace(fmt.Sprint(orEmpty(inputs[CusdecInput])))
	if serial == "" {
		slog.Error("slpa consolidation: no CUSDEC serial on the task inputs; the lookup cannot be keyed")
		return nil
	}
	return url.Values{"cusdecno": []string{serial}}
}

// BuildHeaders presents the client key the CMS identifies the company by.
func (i *FetchInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa consolidation")
}

// Interpret reads the two sides of the lookup and records what to consolidate.
//
// A lookup that reaches the CMS and returns nothing to pair is not an
// acceptance: there is no association to save, and the workflow needs to tell
// those apart to avoid sending an empty save call.
func (i *FetchInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	out := map[string]any{}

	// Every path sets an outcome, so the workflow's gateway always has a value
	// to read: a lookup that failed leaves the trader where they can retry it.
	if callErr != nil || cms.HasErrors(body) {
		out["outcome"] = OutcomeBlocked
		out[RowsKey] = []map[string]any{}
		out["error"] = describeFailure(callErr, body,
			"SLPA could not tell us which containers are available for consolidation:")
		return false, out
	}

	fetched, err := decode(body)
	if err != nil {
		slog.Error("slpa consolidation: the CMS answered with something this step cannot read", "error", err)
		out["outcome"] = OutcomeBlocked
		out[RowsKey] = []map[string]any{}
		out["error"] = "SLPA answered the consolidation lookup with something we could not read. Please try again in a few minutes."
		return false, out
	}

	rows := Rows(fetched)
	done := AlreadyConsolidated(fetched)
	soNumbers := SOContainerNumbers(fetched)

	// The form is always recorded, empty included: the workflow's gateway reads
	// its length, and the trader's panel reads the rest.
	out[RowsKey] = rowsOut(rows)
	out[SOContainersKey] = soContainersOut(fetched.SOContainers)
	out["so_container_numbers"] = soNumbers
	out["available_so_containers"] = strings.Join(soNumbers, ", ")
	out["already_consolidated"] = done
	out["cap_container_count"] = len(fetched.CapContainers)
	out["so_container_count"] = len(fetched.SOContainers)
	out["summary"] = summarise(rows, done, soNumbers)

	switch {
	case len(rows) > 0:
		out["outcome"] = OutcomeReady
		return true, out

	case len(done) > 0:
		// Everything SLPA holds is already paired: nothing for the trader to do.
		out["outcome"] = OutcomeDone
		out[ConsolidatedKey] = done
		return true, out

	default:
		out["outcome"] = OutcomeBlocked
		out["error"] = "SLPA is not reporting any containers to consolidate for this declaration yet.\n\n" +
			summarise(rows, done, soNumbers) +
			"\n\nContainers appear here once the terminal has pre-advised them against the declaration. Use **Check Again** once they have."
		return false, out
	}
}

// SaveInterpreter drives the save call: the pairs the lookup matched, as the CMS
// reads them.
type SaveInterpreter struct{}

// NewSaveInterpreter returns the consolidation save interpreter.
func NewSaveInterpreter() *SaveInterpreter { return &SaveInterpreter{} }

// BuildRequest sends what the trader ticked, as the CMS reads it.
//
// The rows come from their submitted form and the sqids from the lookup that
// filled it, so nothing is paired here that they did not choose.
//
// The contract has no error return, so a selection that cannot be read is sent
// as an empty list: the CMS validates the request and answers with its own
// reason. The workflow only reaches this step when a row was ticked, so this is
// a defensive path — hence the log.
func (i *SaveInterpreter) BuildRequest(inputs map[string]any) remote.Body {
	selection := Resolve(submittedRows(inputs), knownSOContainers(inputs[SOContainersKey]))
	if len(selection.Pairs) == 0 {
		slog.Error("slpa consolidation: nothing resolvable in the submitted selection; sending an empty save call",
			"unresolved", selection.Unresolved)
	}
	return remote.JSONBody{V: SaveRequest{Containers: selection.Pairs}}
}

// BuildHeaders presents the client key the CMS identifies the company by.
func (i *SaveInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa consolidation")
}

// Interpret reports whether the CMS saved the consolidation.
//
// This endpoint answers with the envelope's own status and a message rather than
// a verdict word, so status 1 with no error object is what a save looks like.
func (i *SaveInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	out := map[string]any{}

	if v, ok := body["message"]; ok {
		out["message"] = v
	}
	saved := callErr == nil && !cms.HasErrors(body) && envelopeOK(resp)

	if !saved {
		out["error"] = describeFailure(callErr, body, "SLPA did not save the container consolidation:")
		return false, out
	}
	return true, out
}

// envelopeOK reads the envelope's status, which is how this endpoint reports the
// outcome: 1 for a request it served, 0 for one it refused. Read from the raw
// response rather than the flattened body, since there is no nested "data" here
// to shadow it.
func envelopeOK(resp map[string]any) bool {
	switch v := resp["status"].(type) {
	case float64:
		return v == 1
	case int:
		return v == 1
	case string:
		return strings.TrimSpace(v) == "1"
	default:
		// No status at all: a 200 from this endpoint always carries one, so
		// treating its absence as a save would risk reporting an unsaved
		// consolidation as done.
		return false
	}
}

// decode re-reads the flattened body into the response this step models. Going
// back through JSON keeps one definition of the field names — the struct tags —
// rather than a second, hand-written reading of the same map.
func decode(body map[string]any) (FetchResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return FetchResponse{}, err
	}
	var fetched FetchResponse
	if err := json.Unmarshal(raw, &fetched); err != nil {
		return FetchResponse{}, err
	}
	return fetched, nil
}

// rowsOut renders the form lines as plain maps, which is what a task record
// holds and what the trader's form is filled from.
func rowsOut(rows []Row) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"cap_container_no": r.CapContainerNo,
			"cap_sqid":         r.CapSqid,
			"so_container_no":  r.SOContainerNo,
			"consolidate":      r.Consolidate,
		})
	}
	return out
}

// soContainersOut records the service-order side, which the save step reads back
// to resolve the container number the trader chose into its sqid.
func soContainersOut(containers []SOContainer) []map[string]any {
	out := make([]map[string]any, 0, len(containers))
	for _, so := range containers {
		out = append(out, map[string]any{
			"sqid":         so.Sqid,
			"container_no": so.ContainerNo,
			"size":         so.ContainerSize,
		})
	}
	return out
}

// submittedRows recovers the trader's form lines from the task inputs.
//
// The form arrives under the reserved "payload" key, as every other form-driven
// step here receives it; the rows are read from it directly so nothing but what
// they submitted decides what is sent.
func submittedRows(inputs map[string]any) []Row {
	form, _ := inputs[FormKey].(map[string]any)
	if form == nil {
		return nil
	}
	return readRows(form[RowsKey])
}

// readRows tolerates both the []any a task record holds after a round trip
// through JSON and the typed slice recorded in Go.
func readRows(value any) []Row {
	items, ok := value.([]any)
	if !ok {
		typed, isTyped := value.([]map[string]any)
		if !isTyped {
			return nil
		}
		items = make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	}

	rows := make([]Row, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, Row{
			CapContainerNo: fields.String(m, "cap_container_no"),
			CapSqid:        fields.String(m, "cap_sqid"),
			SOContainerNo:  fields.String(m, "so_container_no"),
			Consolidate:    truthy(m["consolidate"]),
		})
	}
	return rows
}

// knownSOContainers recovers the service-order side the lookup recorded.
func knownSOContainers(value any) []SOContainer {
	items, ok := value.([]any)
	if !ok {
		typed, isTyped := value.([]map[string]any)
		if !isTyped {
			return nil
		}
		items = make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	}

	out := make([]SOContainer, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, SOContainer{
			Sqid:          fields.String(m, "sqid"),
			ContainerNo:   fields.String(m, "container_no"),
			ContainerSize: fields.String(m, "size"),
		})
	}
	return out
}

// truthy reads a checkbox, which reaches here as a bool from JSON and has been
// seen as a string from a form that stringifies its values.
func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

// summarise describes what SLPA is holding, in the trader's terms.
func summarise(rows []Row, alreadyDone []string, soNumbers []string) string {
	var lines []string
	if n := len(rows); n > 0 {
		var names []string
		for _, r := range rows {
			names = append(names, r.CapContainerNo)
		}
		lines = append(lines, fmt.Sprintf("- %d container(s) pre-advised by the terminal, waiting to be consolidated: %s", n, strings.Join(names, ", ")))
	}
	if n := len(alreadyDone); n > 0 {
		lines = append(lines, fmt.Sprintf("- %d already consolidated by SLPA: %s", n, strings.Join(alreadyDone, ", ")))
	}
	if n := len(soNumbers); n > 0 {
		// Named separately from the pre-advised containers on purpose: these are
		// the placeholders the order was priced against, not real containers,
		// and the trader is choosing between them rather than recognising them.
		lines = append(lines, fmt.Sprintf("- %d service order container(s) to consolidate against: %s", n, strings.Join(soNumbers, ", ")))
	}
	if len(lines) == 0 {
		return "- SLPA reported no containers for this declaration."
	}
	return strings.Join(lines, "\n")
}

// describeFailure builds the trader-facing message, preferring the CMS's own
// reasons over anything invented here.
func describeFailure(callErr error, body map[string]any, intro string) string {
	return cms.Failure(callErr, body, intro, "")
}

// orEmpty keeps fmt.Sprint from rendering a missing input as "<nil>".
func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}
