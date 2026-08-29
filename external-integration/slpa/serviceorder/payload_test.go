package serviceorder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A JSONForms number widget round-trips as a string in some browsers. Before the
// shared accessor, the volume read as zero and the trader was refused over a
// value on their own screen.
func TestBuild_AcceptsACBMSubmittedAsAString(t *testing.T) {
	req, err := Build(map[string]any{
		"cusdecNo":  "BIBE1CBEX1-2026-E-10692026",
		"cargoType": "FCL",
		"containers": []any{map[string]any{
			"containerNo": "MSCU8492019", "containerType": "dry", "containerSize": float64(40),
			"serviceId": "1", "cbm": "25.5",
		}},
	})
	require.NoError(t, err)
	require.Len(t, req.Containers, 1)
	assert.Equal(t, 25.5, req.Containers[0].CBM)
}
