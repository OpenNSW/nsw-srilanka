package cdn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCDNIntegrationResultRequest_DualFieldUnmarshaling(t *testing.T) {
	// Test live API format (event, processAt)
	liveJSON := []byte(`{
		"edgeId": "edge-123",
		"integrated": true,
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cdnRef": {"year": "2026", "office": "COL", "serial": "C", "number": 4567}
		}
	}`)
	var reqLive CDNIntegrationResultRequest
	err := json.Unmarshal(liveJSON, &reqLive)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqLive.Event)
	assert.False(t, reqLive.ProcessAt.IsZero())
	assert.NoError(t, reqLive.Validate())

	// Test spec prose format (eventType, processedAt)
	specJSON := []byte(`{
		"edgeId": "edge-123",
		"integrated": true,
		"eventType": "INTEGRATION_RESULT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cdnRef": {"year": "2026", "office": "COL", "serial": "C", "number": 4567}
		}
	}`)
	var reqSpec CDNIntegrationResultRequest
	err = json.Unmarshal(specJSON, &reqSpec)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqSpec.Event)
	assert.False(t, reqSpec.ProcessAt.IsZero())
	assert.NoError(t, reqSpec.Validate())
}

// §7.2: edgeId, integrated, and errors are top-level; payload carries only cdnRef.
func TestCDNIntegrationResultRequest_CanonicalShape(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CDN_INTEGRATED",
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": true,
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 333}
			},
			"errors": {}
		}`)
		var req CDNIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "CDN_INTEGRATED", req.Event)
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.True(t, req.Integrated)
		assert.Equal(t, 333, req.Payload.CDNRef.Number)
		assert.JSONEq(t, `{}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("failure", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CDN_INTEGRATED",
			"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
			"integrated": false,
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {},
			"errors": {"CDN.Package": ["Missing package count"]}
		}`)
		var req CDNIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.False(t, req.Integrated)
		assert.JSONEq(t, `{"CDN.Package": ["Missing package count"]}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})
}

func TestCDNAcknowledgmentRequest_DualFieldUnmarshaling(t *testing.T) {
	specJSON := []byte(`{
		"eventType": "ACKNOWLEDGMENT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cdnRef": {"year": "2026", "office": "COL", "serial": "C", "number": 4567}
		}
	}`)
	var req CDNAcknowledgmentRequest
	err := json.Unmarshal(specJSON, &req)
	require.NoError(t, err)
	assert.Equal(t, "ACKNOWLEDGMENT", req.Event)
	assert.Equal(t, 2026, req.ProcessAt.Year())
	assert.NoError(t, req.Validate())
}
