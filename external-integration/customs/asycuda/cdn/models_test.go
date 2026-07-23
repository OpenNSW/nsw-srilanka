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
		"edgId": "edg-123",
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
	assert.NoError(t, reqLive.validate())

	// Test spec prose format (eventType, processedAt)
	specJSON := []byte(`{
		"edgId": "edg-123",
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
	assert.NoError(t, reqSpec.validate())
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
	assert.NoError(t, req.validate())
}
