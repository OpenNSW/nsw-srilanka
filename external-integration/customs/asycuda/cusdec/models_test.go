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
		"edgeId": "edge-123",
		"integrated": true,
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var reqLive CusdecIntegrationResultRequest
	err := json.Unmarshal(liveJSON, &reqLive)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqLive.Event)
	assert.Equal(t, "CBEX1", reqLive.Payload.CusdecRef.Office)
	assert.Equal(t, 43254, reqLive.Payload.CusdecRef.Number)
	assert.NoError(t, reqLive.validate())

	// Test spec prose format (eventType, processedAt, cusdecRef)
	specJSON := []byte(`{
		"edgeId": "edge-123",
		"integrated": true,
		"eventType": "INTEGRATION_RESULT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var reqSpec CusdecIntegrationResultRequest
	err = json.Unmarshal(specJSON, &reqSpec)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqSpec.Event)
	assert.Equal(t, "CBEX1", reqSpec.Payload.CusdecRef.Office)
	assert.Equal(t, 43254, reqSpec.Payload.CusdecRef.Number)
	assert.NoError(t, reqSpec.validate())
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
	assert.NoError(t, req.validate())
}
