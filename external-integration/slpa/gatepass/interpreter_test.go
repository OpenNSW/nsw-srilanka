package gatepass

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
)

func sent(t *testing.T, inputs map[string]any) Request {
	t.Helper()
	raw, _, err := NewInterpreter().BuildRequest(inputs).Encode()
	require.NoError(t, err)
	var req Request
	require.NoError(t, json.Unmarshal(raw, &req))
	return req
}

func decoded(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestBuildRequest_CarriesTheHaulageDetails(t *testing.T) {
	req := sent(t, map[string]any{
		"container_no": "CON-FCL-001",
		"payload": map[string]any{
			"truck_no":    " LM-4821 ",
			"driver_name": "K. Perera",
			"seal_no":     "SL-93820",
		},
	})

	assert.Equal(t, Request{
		ContainerNo: "CON-FCL-001",
		TruckNo:     "LM-4821",
		DriverName:  "K. Perera",
		SealNo:      "SL-93820",
	}, req)
}

// The container comes from what SLPA consolidated, not from the form: a pass for
// a container that was never consolidated is refused by the CMS with a 422 the
// trader cannot act on.
func TestBuildRequest_TakesTheContainerFromTheBranchNotTheForm(t *testing.T) {
	req := sent(t, map[string]any{
		"container_no": "CON-FCL-001",
		"payload":      map[string]any{"container_no": "CON-TYPED-BY-HAND", "truck_no": "LM-4821"},
	})
	assert.Equal(t, "CON-FCL-001", req.ContainerNo)
}

func TestInterpret_CapturesWhatTheGateScans(t *testing.T) {
	const raw = `{"openapi": "3.0.3", "status": 1, "data": {
	  "gate_pass_id": 7, "gate_pass_no": "GP-2026-000123", "container_no": "CON-FCL-001",
	  "truck_no": "LM-4821", "driver_name": "K. Perera", "seal_no": "SL-93820",
	  "barcode": "GP2026000123CONFCL001", "status": "ISSUED",
	  "issued_at": "2026-08-28T10:25:03.803Z"}}`

	issued, out := NewInterpreter().Interpret(nil, decoded(t, raw))

	require.True(t, issued)
	assert.Equal(t, "GP-2026-000123", out["gate_pass_no"])
	assert.Equal(t, "GP2026000123CONFCL001", out["barcode"])
	assert.Equal(t, "2026-08-28T10:25:03.803Z", out["issued_at"])
	assert.NotContains(t, out, "error")
}

// No pass number means no pass, whatever else the body says.
func TestInterpret_WithoutAPassNumberIsNotIssued(t *testing.T) {
	issued, out := NewInterpreter().Interpret(nil, decoded(t, `{"status": 1, "data": {"status": "ISSUED"}}`))
	assert.False(t, issued)
	assert.Contains(t, out["error"], "did not issue the gate pass")
}

// SLPA issues a pass only against a paid service order, and we can run ahead of
// its own paid flag. That is a wait, not a mistake in what the trader entered —
// telling them to correct correct details would send them in circles.
func TestInterpret_UnpaidInvoiceReadsAsAWaitNotAMistake(t *testing.T) {
	const raw = `{"status": 0, "error": {"code": "UNPROCESSABLE",
	  "message": "unpaid invoice", "details": {"invoice": ["The service order invoice is unpaid"]}}}`

	issued, out := NewInterpreter().Interpret(nil, decoded(t, raw))

	require.False(t, issued)
	msg, _ := out["error"].(string)
	assert.Contains(t, msg, "has not registered the payment")
	assert.Contains(t, msg, "Try again shortly")
	assert.NotContains(t, msg, "check the details")
}

// An invalid container answers 422 too, so the two refusals must not be conflated.
func TestInterpret_InvalidContainerIsReportedAsGiven(t *testing.T) {
	const raw = `{"status": 0, "error": {"code": "UNPROCESSABLE",
	  "message": "invalid container", "details": {"container_no": ["CON-X is not consolidated"]}}}`

	issued, out := NewInterpreter().Interpret(nil, decoded(t, raw))

	require.False(t, issued)
	msg, _ := out["error"].(string)
	assert.Contains(t, msg, "CON-X is not consolidated")
	assert.NotContains(t, msg, "has not registered the payment")
}

func TestInterpret_UnreachableCMSSaysSo(t *testing.T) {
	issued, out := NewInterpreter().Interpret(errors.New("dial tcp: timeout"), nil)
	require.False(t, issued)
	assert.Contains(t, out["error"], "could not get a usable answer")
}

func TestBuildHeaders_PresentsTheClientKey(t *testing.T) {
	i := NewInterpreter()
	assert.Equal(t, map[string]string{cms.ClientKeyHeader: "agztNvLSUA"},
		i.BuildHeaders(map[string]any{cms.ClientKeyInput: "agztNvLSUA"}))
	assert.Nil(t, i.BuildHeaders(map[string]any{}))
}
