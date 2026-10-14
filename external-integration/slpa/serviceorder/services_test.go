package serviceorder

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
)

// servicesAnswer is the CMS's answer to the sundry-services lookup, as it sends
// it: the list under data, beside the envelope's own fields.
func servicesAnswer(rows ...any) map[string]any {
	return map[string]any{
		"openapi":       "3.0.3",
		"status":        float64(1),
		"last_modified": "2026-10-13 09:00:00",
		"data":          rows,
	}
}

func TestServicesInterpreter_RecordsTheServicesInTheOrderSent(t *testing.T) {
	ok, out := NewServicesInterpreter().Interpret(nil, servicesAnswer(
		map[string]any{"service_id": "l1c73wk5", "service_name": "CRANE CHARGES OVER 10 M UP TO 15 M",
			"is_sundry": true, "is_main": false, "fcl_lcl_type": "FCL_General"},
		map[string]any{"service_id": "cn90sg2a", "service_name": "GARMENT",
			"is_sundry": true, "is_main": true, "fcl_lcl_type": "FCL_LCL_BB_Bulk"},
	))

	require.True(t, ok)
	assert.Equal(t, []map[string]any{
		{"service_id": "l1c73wk5", "service_name": "CRANE CHARGES OVER 10 M UP TO 15 M"},
		{"service_id": "cn90sg2a", "service_name": "GARMENT"},
	}, out[ServicesKey])
}

// The error key is set even on success, to nil, so that a retry that succeeds
// clears the message an earlier failure left on the trader's panel.
func TestServicesInterpreter_ClearsAnEarlierErrorOnSuccess(t *testing.T) {
	ok, out := NewServicesInterpreter().Interpret(nil, servicesAnswer(
		map[string]any{"service_id": "cn90sg2a", "service_name": "GARMENT", "is_sundry": true},
	))

	require.True(t, ok)
	v, present := out[ServicesErrorKey]
	assert.True(t, present, "the error key is written so a stale message is overwritten")
	assert.Nil(t, v)
}

// An entry that is not a sundry service, or that is missing its ID or name,
// would be an option no order could use or one the trader could not read.
func TestServicesInterpreter_LeavesOutWhatCannotBeOrdered(t *testing.T) {
	ok, out := NewServicesInterpreter().Interpret(nil, servicesAnswer(
		map[string]any{"service_id": "main0001", "service_name": "MAIN ONLY", "is_sundry": false},
		map[string]any{"service_id": "", "service_name": "NO ID", "is_sundry": true},
		map[string]any{"service_id": "noname01", "is_sundry": true},
		"not an object",
		map[string]any{"service_id": "gq43znje", "service_name": "CRANE CHARGES OVER 15 M UP TO 20 M"},
	))

	require.True(t, ok)
	assert.Equal(t, []map[string]any{
		{"service_id": "gq43znje", "service_name": "CRANE CHARGES OVER 15 M UP TO 20 M"},
	}, out[ServicesKey])
}

// With nothing to choose from no sundry order can be raised, so an empty list is
// reported to the trader rather than shown as an empty dropdown.
func TestServicesInterpreter_AnEmptyListIsNotASuccess(t *testing.T) {
	ok, out := NewServicesInterpreter().Interpret(nil, servicesAnswer())

	assert.False(t, ok)
	assert.Contains(t, out[ServicesErrorKey], "not offering any sundry services")
	assert.NotContains(t, out, ServicesKey)
}

// A refusal carries the CMS's own reason, which names what is wrong.
func TestServicesInterpreter_ReportsTheCMSsReason(t *testing.T) {
	ok, out := NewServicesInterpreter().Interpret(errors.New("400 Bad Request"), map[string]any{
		"openapi": "3.0.3",
		"status":  float64(0),
		"error": map[string]any{
			"code":    "MISSING_CLIENT_HEADER",
			"message": "Client identifier header 'slpacmsuser-key' is required.",
		},
	})

	assert.False(t, ok)
	msg, _ := out[ServicesErrorKey].(string)
	assert.Contains(t, msg, "SLPA could not list the sundry services:")
	assert.Contains(t, msg, "slpacmsuser-key")
}

func TestServicesInterpreter_ReportsAnUnreachableCMS(t *testing.T) {
	ok, out := NewServicesInterpreter().Interpret(errors.New("dial tcp: connection refused"), nil)

	assert.False(t, ok)
	assert.Equal(t, cms.Unreachable, out[ServicesErrorKey])
}

func TestServicesInterpreter_PresentsTheClientKey(t *testing.T) {
	headers := NewServicesInterpreter().BuildHeaders(map[string]any{cms.ClientKeyInput: "agztNvLSUA"})

	assert.Equal(t, map[string]string{cms.ClientKeyHeader: "agztNvLSUA"}, headers)
}

func TestServicesInterpreter_SendsNoBody(t *testing.T) {
	assert.Nil(t, NewServicesInterpreter().BuildRequest(map[string]any{cms.ClientKeyInput: "agztNvLSUA"}))
}
