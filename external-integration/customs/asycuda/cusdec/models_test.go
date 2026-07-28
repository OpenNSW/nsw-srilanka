package cusdec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCusdecIntegrationResultRequest_DualFieldUnmarshaling(t *testing.T) {
	// Test live API format (event, processAt, cusDecRef)
	liveJSON := []byte(`{
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
			"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var reqLive CusdecIntegrationResultRequest
	err := json.Unmarshal(liveJSON, &reqLive)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqLive.Event)
	assert.Equal(t, "CBEX1", reqLive.Payload.CusdecRef.Office)
	assert.Equal(t, 43254, reqLive.Payload.CusdecRef.Number)
	assert.NoError(t, reqLive.Validate())

	// Test spec prose format (eventType, processedAt, cusdecRef)
	specJSON := []byte(`{
		"eventType": "INTEGRATION_RESULT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var reqSpec CusdecIntegrationResultRequest
	err = json.Unmarshal(specJSON, &reqSpec)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqSpec.Event)
	assert.Equal(t, "CBEX1", reqSpec.Payload.CusdecRef.Office)
	assert.Equal(t, 43254, reqSpec.Payload.CusdecRef.Number)
	assert.NoError(t, reqSpec.Validate())
}

// §6.2 places edgeId, integrated, taxes, and errors inside payload.
func TestCusdecIntegrationResultRequest_NestedPayloadFields(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": true,
				"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
				"taxes": [{"code": "tax1", "rate": 1, "amount": 222}],
				"errors": {}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "CUSDEC_INTEGRATED", req.Event)
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.True(t, req.Integrated)
		assert.Len(t, req.Payload.Taxes, 1)
		assert.JSONEq(t, `{}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("failure carries payload errors", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": false,
				"errors": {"Declaration.HSCode": ["Invalid HS code"]}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.False(t, req.Integrated)
		assert.JSONEq(t, `{"Declaration.HSCode": ["Invalid HS code"]}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	// Only payload.* is authoritative: fields at the top level are not part of
	// §6.2 and must be ignored rather than silently correlating the wrong record.
	t.Run("top-level fields are ignored", func(t *testing.T) {
		body := []byte(`{
			"edgeId": "not-a-real-edge",
			"integrated": true,
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"errors": {"Bogus": ["ignored"]},
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": false,
				"errors": {"Declaration.HSCode": ["Invalid HS code"]}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.False(t, req.Integrated)
		assert.JSONEq(t, `{"Declaration.HSCode": ["Invalid HS code"]}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("top-level only payload is rejected", func(t *testing.T) {
		body := []byte(`{
			"edgeId": "edge-legacy",
			"integrated": true,
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"errors": {},
			"payload": {
				"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Empty(t, req.EdgeID)
		assert.False(t, req.Integrated)
		assert.EqualError(t, req.Validate(), "edgeId is required")
	})
}

// Decoding into a reused value must not carry fields the second document omits.
func TestCusdecIntegrationResultRequest_ReusedReceiverIsReset(t *testing.T) {
	var req CusdecIntegrationResultRequest

	first := []byte(`{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-06-26T04:04:52Z",
		"payload": {
			"edgeId": "edge-first",
			"integrated": true,
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
			"taxes": [{"code": "tax1", "rate": 1, "amount": 222}],
			"errors": {}
		}
	}`)
	require.NoError(t, json.Unmarshal(first, &req))
	require.Equal(t, "edge-first", req.EdgeID)
	require.True(t, req.Integrated)
	require.Len(t, req.Payload.Taxes, 1)

	second := []byte(`{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-06-27T04:04:52Z",
		"payload": {
			"edgeId": "edge-second",
			"integrated": false,
			"errors": {"Declaration.HSCode": ["Invalid HS code"]}
		}
	}`)
	require.NoError(t, json.Unmarshal(second, &req))

	assert.Equal(t, "edge-second", req.EdgeID)
	assert.False(t, req.Integrated)
	assert.Empty(t, req.Payload.Taxes, "taxes from the first document must not survive")
	assert.False(t, req.Payload.CusdecRef.IsValid(), "cusdecRef from the first document must not survive")
	assert.JSONEq(t, `{"Declaration.HSCode": ["Invalid HS code"]}`, string(req.Errors))
}

func TestCusdecEventRequest_ReusedReceiverIsReset(t *testing.T) {
	var req CusdecEventRequest

	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "PAYMENT_CONFIRMED",
		"processedAt": "2026-04-26T11:15:22Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
			"amountPaid": 2035.00,
			"currency": "LKR"
		}
	}`), &req))
	require.Equal(t, 2035.00, req.Payload.AmountPaid)

	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "EXPORT_RELEASED",
		"processedAt": "2026-04-26T11:15:22Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
			"vesselName": "EVER GIVEN"
		}
	}`), &req))

	assert.Equal(t, "EXPORT_RELEASED", req.Event)
	assert.Equal(t, "EVER GIVEN", req.Payload.VesselName)
	assert.Zero(t, req.Payload.AmountPaid, "amountPaid from the payment document must not survive")
	assert.Empty(t, req.Payload.Currency, "currency from the payment document must not survive")
}

func TestCusdecEventRequest_DualFieldUnmarshaling(t *testing.T) {
	specJSON := []byte(`{
		"eventType": "PAYMENT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var req CusdecEventRequest
	err := json.Unmarshal(specJSON, &req)
	require.NoError(t, err)
	assert.Equal(t, "PAYMENT", req.Event)
	assert.Equal(t, "CBEX1", req.Payload.CusdecRef.Office)
	assert.NoError(t, req.Validate())
}
