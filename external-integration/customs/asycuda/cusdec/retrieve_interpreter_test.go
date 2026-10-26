package cusdec

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The request names the declaration by its registered reference, with the
// declarant and exporter codes the trader declared it under (§6.7).
func TestRetrieveInterpreter_NamesTheDeclarationAndItsParties(t *testing.T) {
	body := NewRetrieveInterpreter().BuildRequest(map[string]any{
		RetrieveRefKey: "CBEX1/2026/E/1047",
		RetrieveFormKey: map[string]any{"traders": map[string]any{
			"declarant": map[string]any{"code": "D0012345"},
			"exporter":  map[string]any{"code": "C003"},
		}},
	})

	jsonBody, ok := body.(remote.JSONBody)
	require.True(t, ok, "retrieve is a JSON request")
	raw, err := json.Marshal(jsonBody.V)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"cusDecRef": {"office": "CBEX1", "year": "2026", "serial": "E", "number": 1047},
		"declarantCode": "D0012345",
		"companyCode": "C003"
	}`, string(raw))
}

// retrieved is a §6.7 answer whose declaration holds the given generalSegment.
func retrieved(t *testing.T, generalSegment string) map[string]any {
	t.Helper()
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "CUSDEC_RETRIEVED",
		"processedAt": "2026-10-21T09:00:00Z",
		"payload": {
			"cusdec": {"generalSegment": `+generalSegment+`},
			"cdnRefs": []
		}
	}`), &resp))
	return resp
}

// The declaration's current container count is captured, so the dispatch notes
// follow an amendment Customs made after submission.
func TestRetrieveInterpreter_CapturesTheCurrentContainerCount(t *testing.T) {
	accepted, out := NewRetrieveInterpreter().Interpret(nil, retrieved(t, `{"containerFlag": true, "numberOfContainers": 4}`))

	assert.True(t, accepted)
	assert.Equal(t, map[string]any{RetrieveCountKey: 4}, out)
}

// The amendment notification spells the declaration cusDec; that is read too.
func TestRetrieveInterpreter_ReadsTheCusDecSpelling(t *testing.T) {
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"payload": {"cusDec": {"generalSegment": {"numberOfContainers": 2}}}}`), &resp))

	accepted, out := NewRetrieveInterpreter().Interpret(nil, resp)

	assert.True(t, accepted)
	assert.Equal(t, 2, out[RetrieveCountKey])
}

// Any answer without a usable count leaves nothing captured, so the count the
// trader declared stands.
func TestRetrieveInterpreter_WithoutAUsableCountCapturesNone(t *testing.T) {
	for name, tc := range map[string]struct {
		callErr error
		resp    map[string]any
	}{
		"call failed":        {callErr: errors.New("404 Not Found"), resp: map[string]any{"error": "not found"}},
		"no declaration":     {resp: map[string]any{"payload": map[string]any{}}},
		"no container count": {resp: retrieved(t, `{"containerFlag": false}`)},
		"zero containers":    {resp: retrieved(t, `{"numberOfContainers": 0}`)},
		"fractional count":   {resp: retrieved(t, `{"numberOfContainers": 2.5}`)},
		"not a number":       {resp: retrieved(t, `{"numberOfContainers": "many"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			accepted, out := NewRetrieveInterpreter().Interpret(tc.callErr, tc.resp)

			assert.False(t, accepted)
			assert.NotContains(t, out, RetrieveCountKey)
			assert.NotEmpty(t, out["error"])
		})
	}
}
