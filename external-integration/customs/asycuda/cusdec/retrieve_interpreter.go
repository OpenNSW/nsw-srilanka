package cusdec

import (
	"fmt"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
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

// RetrieveDeclarationKey and RetrieveCDNRefsKey are where the retrieved
// declaration and the dispatch notes listed against it (cdnRefs) are captured,
// whenever the answer carries a declaration, so a later step can read the
// declaration as Customs holds it without retrieving it again.
const (
	RetrieveDeclarationKey = "declaration"
	RetrieveCDNRefsKey     = "cdn_refs"
)

// BuildRequest names the declaration by its registered reference, with the
// declarant and the exporter's company code the trader declared it under (see
// document.NewDeclarationRetrieve).
func (RetrieveInterpreter) BuildRequest(inputs map[string]any) remote.Body {
	form, _ := inputs[RetrieveFormKey].(map[string]any)
	ref, _ := inputs[RetrieveRefKey].(string)
	return remote.JSONBody{V: document.NewDeclarationRetrieve(ref, form)}
}

// Interpret captures the retrieved declaration, the dispatch notes listed
// against it, and its current container count. The answer is accepted only with
// a usable count, which is what the CDN split needs; a declaration without one
// is still captured, with the reason under "error".
func (RetrieveInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	if callErr != nil {
		return false, map[string]any{"error": "the declaration could not be retrieved: " + callErr.Error()}
	}
	decl := document.RetrievedDeclaration(resp)
	if decl == nil {
		return false, map[string]any{"error": "the retrieve answer carries no declaration"}
	}
	out := map[string]any{
		RetrieveDeclarationKey: decl,
		RetrieveCDNRefsKey:     document.RetrievedCDNRefs(resp),
	}
	count, err := containerCount(decl)
	if err != nil {
		out["error"] = err.Error()
		return false, out
	}
	out[RetrieveCountKey] = count
	return true, out
}

// containerCount reads generalSegment.numberOfContainers from a retrieved
// declaration.
func containerCount(decl map[string]any) (int, error) {
	raw, ok := document.Nested(decl, "generalSegment")["numberOfContainers"]
	if !ok {
		return 0, fmt.Errorf("the retrieved declaration carries no generalSegment.numberOfContainers")
	}
	count, err := document.WholeNumber(raw)
	if err != nil {
		return 0, fmt.Errorf("the retrieved declaration's numberOfContainers is not a whole number: %w", err)
	}
	if count < 1 {
		return 0, fmt.Errorf("the retrieved declaration reports %d containers", count)
	}
	return count, nil
}
