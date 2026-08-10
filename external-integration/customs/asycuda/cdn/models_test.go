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
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
			"cdnRef": {"year": "2026", "office": "COL", "serial": "C", "number": 4567},
			"errors": {}
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
		"eventType": "INTEGRATION_RESULT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
			"cdnRef": {"year": "2026", "office": "COL", "serial": "C", "number": 4567},
			"errors": {}
		}
	}`)
	var reqSpec CDNIntegrationResultRequest
	err = json.Unmarshal(specJSON, &reqSpec)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqSpec.Event)
	assert.False(t, reqSpec.ProcessAt.IsZero())
	assert.NoError(t, reqSpec.Validate())
}

// §7.2 v1.3: edgeId, integrated, cdnRef, and errors travel inside payload.
func TestCDNIntegrationResultRequest_CanonicalShape(t *testing.T) {
	t.Run("v1.3 spec success inside payload", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CDN_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": true,
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 333},
				"errors": {}
			}
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

	t.Run("v1.3 spec failure inside payload", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CDN_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": false,
				"errors": {"CDN.Package": ["Missing package count"]}
			}
		}`)
		var req CDNIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.False(t, req.Integrated)
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.JSONEq(t, `{"CDN.Package": ["Missing package count"]}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("omitted payload.integrated is rejected", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CDN_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 333}
			}
		}`)
		var req CDNIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.EqualError(t, req.Validate(), "integrated is required")
	})

	t.Run("top-level only payload is rejected", func(t *testing.T) {
		body := []byte(`{
			"edgeId": "edge-legacy",
			"integrated": true,
			"eventType": "CDN_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"errors": {},
			"payload": {
				"cdnRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 333}
			}
		}`)
		var req CDNIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Empty(t, req.EdgeID)
		assert.False(t, req.Integrated)
		assert.EqualError(t, req.Validate(), "edgeId is required")
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
