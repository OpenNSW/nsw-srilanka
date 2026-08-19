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
		"eventType": "INTEGRATION_RESULT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
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

// §7.2: payload carries edgeId, integrated and errors alongside cdnRef.
func TestCDNIntegrationResultRequest_CanonicalShape(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.Payload.EdgeID)
		assert.True(t, req.Payload.Integrated)
		assert.Equal(t, 333, req.Payload.CDNRef.Number)
		assert.JSONEq(t, `{}`, string(req.Payload.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("failure", func(t *testing.T) {
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
		assert.False(t, req.Payload.Integrated)
		assert.JSONEq(t, `{"CDN.Package": ["Missing package count"]}`, string(req.Payload.Errors))
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

// §7.2 carries edgeId, integrated and errors inside payload. The spec shape is
// what a real SLC Edge callback sends, so it must decode; the flat shape is kept
// working because earlier revisions of this integration sent it that way.
func TestCDNIntegrationResult_AcceptsSpecPayloadShape(t *testing.T) {
	body := []byte(`{
	  "eventType": "CDN_INTEGRATED",
	  "processedAt": "2026-06-26T04:04:52Z",
	  "payload": {
	    "edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
	    "integrated": true,
	    "cdnRef": { "year": "2026", "office": "CBEX1", "serial": "E", "number": 333 },
	    "errors": {}
	  }
	}`)

	var req CDNIntegrationResultRequest
	require.NoError(t, json.Unmarshal(body, &req))
	require.NoError(t, req.Validate())

	assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.Payload.EdgeID)
	assert.True(t, req.Payload.Integrated)
	assert.Equal(t, "CDN_INTEGRATED", req.Event)
	assert.Equal(t, 333, req.Payload.CDNRef.Number)
}

func TestCDNIntegrationResult_SpecShapeRejection(t *testing.T) {
	body := []byte(`{
	  "eventType": "CDN_INTEGRATED",
	  "processedAt": "2026-06-26T04:04:52Z",
	  "payload": {
	    "edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
	    "integrated": false,
	    "errors": { "0": [ { "code": 331, "description": "Missing office Code" } ] }
	  }
	}`)

	var req CDNIntegrationResultRequest
	require.NoError(t, json.Unmarshal(body, &req))
	// A rejection carries no cdnRef, and must still validate.
	require.NoError(t, req.Validate())
	assert.False(t, req.Payload.Integrated)
	require.NotEmpty(t, req.Payload.Errors)

	// The trader sees the descriptions, never the raw segment-keyed JSON.
	msg := describeErrors(req.Payload.Errors)
	assert.Contains(t, msg, "Missing office Code")
	assert.NotContains(t, msg, `"code"`)
}

// Only payload is read. A callback that puts these at the top level is not §7.2
// and must fail validation rather than being quietly accepted.
func TestCDNIntegrationResult_RejectsTopLevelFields(t *testing.T) {
	body := []byte(`{
	  "eventType": "CDN_INTEGRATED",
	  "processedAt": "2026-06-26T04:04:52Z",
	  "edgeId": "flat-edge",
	  "integrated": true,
	  "payload": { "cdnRef": { "year": "2026", "office": "CBEX1", "serial": "C", "number": 28237 } }
	}`)

	var req CDNIntegrationResultRequest
	require.NoError(t, json.Unmarshal(body, &req))
	assert.Empty(t, req.Payload.EdgeID)
	assert.False(t, req.Payload.Integrated)
	require.Error(t, req.Validate())
}
