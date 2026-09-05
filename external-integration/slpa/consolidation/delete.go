package consolidation

import (
	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
)

// CapSqidKey is the task input carrying the cap container's sqid. The artifact
// declares the endpoint as /export/container-consolidation/{cap_id} and the
// plugin fills that placeholder from the inputs, so this is the name the
// workflow maps the sqid to.
const CapSqidKey = "cap_id"

// DeleteInterpreter drives DELETE /export/container-consolidation/{cap_id}:
// it undoes a consolidation the trader wants to redo against a different real
// container.
//
// The CMS deletes the consolidation and everything hanging off it, gate passes
// included. The flow only offers this before a pass has been generated, so what
// it removes here is the pairing alone — but the endpoint's reach is why it is
// not offered afterwards, when a haulier may already be holding a pass.
type DeleteInterpreter struct{}

// NewDeleteInterpreter returns the consolidation delete interpreter.
func NewDeleteInterpreter() *DeleteInterpreter { return &DeleteInterpreter{} }

// BuildRequest sends nothing: the record to delete is named in the path, and
// the plugin does not ask for a body on a method that carries none.
func (i *DeleteInterpreter) BuildRequest(map[string]any) remote.Body { return nil }

// BuildHeaders presents the client key the CMS identifies the company by.
func (i *DeleteInterpreter) BuildHeaders(inputs map[string]any) map[string]string {
	return cms.ClientKeyHeaders(inputs, "slpa consolidation delete")
}

// Interpret reports whether the consolidation was removed.
//
// A delete that the CMS refuses leaves the trader where they were, with the
// consolidation still standing: the flow returns them to the gate-pass view
// rather than to a consolidation step that would then pair an already-paired
// container.
func (i *DeleteInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	body := cms.Flatten(resp)
	out := map[string]any{}

	if v, ok := body["message"]; ok {
		out["message"] = v
	}

	deleted := callErr == nil && !cms.HasErrors(body) && envelopeOK(resp)
	if !deleted {
		out["error"] = describeFailure(callErr, body, "SLPA did not remove the container consolidation:")
		return false, out
	}
	return true, out
}
