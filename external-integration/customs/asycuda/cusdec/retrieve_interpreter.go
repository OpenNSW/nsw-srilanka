package cusdec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/OpenNSW/core/remote"
)

// RetrieveInterpreter adapts the generic API-call plugin to the SLC Edge
// declaration retrieve endpoint (POST /api/declaration/v1/retrieve, §6.7).
//
// Retrieve returns the declaration as it currently stands in ASYCUDA World, so
// it reflects any amendment Customs made after the trader submitted it. The
// consignment reads the container count from it before opening one dispatch
// note branch per container.
//
// An answer that does not carry a usable count is not accepted, and nothing is
// captured for it: the step still completes, and the branches fall back to the
// count the trader declared. Retrieve is read-only (§2.2), so a missed answer
// costs a check, not a declaration.
type RetrieveInterpreter struct{}

// NewRetrieveInterpreter returns the retrieve interpreter.
func NewRetrieveInterpreter() RetrieveInterpreter { return RetrieveInterpreter{} }

// RetrieveRefKey, RetrieveFormKey are the task inputs the retrieve request is
// built from: the registered reference, in its office/year/serial/number display
// form, and the trader's declaration form, which names the declarant and the
// exporter the request must match.
const (
	RetrieveRefKey  = "cusdec_ref"
	RetrieveFormKey = "userform"
)

// RetrieveCountKey is where the declaration's current container count is
// captured when the retrieve answers with one.
const RetrieveCountKey = "container_count"

// retrieveRequest is the §6.7 request body. The specification's example names
// the reference cusDecRef, as the amendment notification does (§6.5.4).
type retrieveRequest struct {
	CusdecRef     DocumentReference `json:"cusDecRef"`
	DeclarantCode string            `json:"declarantCode"`
	CompanyCode   string            `json:"companyCode"`
}

// BuildRequest names the declaration by its registered reference, with the
// declarant and the exporter's company code the trader declared it under: SLC
// Edge returns a declaration only to the codes it was registered with (§6.7).
// An unreadable reference is still sent, empty, so the endpoint's refusal is
// what the step records.
func (RetrieveInterpreter) BuildRequest(inputs map[string]any) remote.Body {
	ref, _ := parseCusdecRef(stringInput(inputs, RetrieveRefKey))
	form, _ := inputs[RetrieveFormKey].(map[string]any)
	traders := nested(form, "traders")
	return remote.JSONBody{V: retrieveRequest{
		CusdecRef:     ref,
		DeclarantCode: str(nested(traders, "declarant"), "code"),
		CompanyCode:   str(nested(traders, "exporter"), "code"),
	}}
}

// Interpret captures the current container count from the retrieved
// declaration. Any answer without one is not accepted, with the reason recorded
// under "error" for the logs; the count the trader declared stands.
func (RetrieveInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	if callErr != nil {
		return false, map[string]any{"error": "the declaration could not be retrieved: " + callErr.Error()}
	}
	count, err := retrievedContainerCount(resp)
	if err != nil {
		return false, map[string]any{"error": err.Error()}
	}
	return true, map[string]any{RetrieveCountKey: count}
}

// retrievedContainerCount reads generalSegment.numberOfContainers from the
// declaration in a §6.7 answer. The specification shows the declaration under
// payload.cusdec; payload.cusDec, the amendment's spelling, and an unwrapped
// cusdec are read too, since the example is not consistent about either.
func retrievedContainerCount(resp map[string]any) (int, error) {
	decl := retrievedDeclaration(resp)
	if decl == nil {
		return 0, fmt.Errorf("the retrieve answer carries no declaration")
	}
	raw, ok := nested(decl, "generalSegment")["numberOfContainers"]
	if !ok {
		return 0, fmt.Errorf("the retrieved declaration carries no generalSegment.numberOfContainers")
	}
	count, err := wholeNumber(raw)
	if err != nil {
		return 0, fmt.Errorf("the retrieved declaration's numberOfContainers is not a whole number: %w", err)
	}
	if count < 1 {
		return 0, fmt.Errorf("the retrieved declaration reports %d containers", count)
	}
	return count, nil
}

func retrievedDeclaration(resp map[string]any) map[string]any {
	for _, holder := range []map[string]any{nested(resp, "payload"), resp} {
		for _, key := range []string{"cusdec", "cusDec"} {
			if decl, ok := holder[key].(map[string]any); ok && len(decl) > 0 {
				return decl
			}
		}
	}
	return nil
}

// wholeNumber accepts the decoded JSON forms a count can take: a number, or a
// numeric string.
func wholeNumber(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("%v has a fraction", n)
		}
		return int(n), nil
	case json.Number:
		return strconv.Atoi(n.String())
	case string:
		return strconv.Atoi(strings.TrimSpace(n))
	default:
		return 0, fmt.Errorf("got %T", v)
	}
}

// parseCusdecRef splits the office/year/serial/number display form the
// integration result writes into the workflow back into the reference (§4.1).
func parseCusdecRef(s string) (DocumentReference, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 4 {
		return DocumentReference{}, fmt.Errorf("reference %q is not office/year/serial/number", s)
	}
	number, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil {
		return DocumentReference{}, fmt.Errorf("reference %q does not end in a number", s)
	}
	ref := DocumentReference{
		Office: strings.TrimSpace(parts[0]),
		Year:   strings.TrimSpace(parts[1]),
		Serial: strings.TrimSpace(parts[2]),
		Number: number,
	}
	if !ref.IsValid() {
		return DocumentReference{}, fmt.Errorf("reference %q is incomplete", s)
	}
	return ref, nil
}

func stringInput(inputs map[string]any, key string) string {
	s, _ := inputs[key].(string)
	return s
}
