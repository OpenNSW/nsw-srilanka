package renderer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/uiprojector"
)

// The form as authored: a free-text container field the projector turns into a
// list of what this declaration currently has available.
const consolidationForm = `{
  "schema": {
    "type": "object",
    "properties": {
      "so_container_no": {"type": "string", "readOnly": true},
      "cap_container_no": {"type": "string"}
    }
  },
  "uiSchema": {"type": "VerticalLayout", "elements": []}
}`

func projectConsolidation(t *testing.T, capContainers []any) map[string]any {
	t.Helper()

	record := map[string]any{
		"consolidation":     map[string]any{"cap_containers": capContainers},
		"consolidationform": map[string]any{"so_container_no": "DUMY0000001"},
	}

	projection, err := NewSLPAConsolidationProjector().
		Project(context.Background(), []byte(consolidationForm), record)
	require.NoError(t, err)

	form, ok := projection.Content.(uiprojector.FormContent)
	require.True(t, ok, "the projector renders a form")

	schema, ok := form.Schema.(map[string]any)
	require.True(t, ok)
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	field, ok := props["cap_container_no"].(map[string]any)
	require.True(t, ok)
	return field
}

// The option's value is the sqid and its title the container number: one answer
// keys the save and the delete, while the trader reads what is on the box.
func TestConsolidationProjector_OffersTheContainersFreeToPair(t *testing.T) {
	field := projectConsolidation(t, []any{
		map[string]any{"sqid": "cap-A", "container_no": "MSCU8492019", "so_container_sqid": nil},
		map[string]any{"sqid": "cap-B", "container_no": "TCLU1234567"},
	})

	assert.Equal(t, []any{
		map[string]any{"const": "cap-A", "title": "MSCU8492019"},
		map[string]any{"const": "cap-B", "title": "TCLU1234567"},
	}, field["oneOf"])
}

// so_container_sqid is the CMS's value and it has not promised which JSON type
// it comes back as. Read with a hard string assertion, a number here said "not
// paired yet" and put an already-consolidated container back in front of the
// trader — a save the CMS then refuses.
func TestConsolidationProjector_LeavesOutAlreadyPairedWhateverTypeTheSqidArrivesAs(t *testing.T) {
	field := projectConsolidation(t, []any{
		map[string]any{"sqid": "cap-A", "container_no": "MSCU8492019", "so_container_sqid": "so-1"},
		map[string]any{"sqid": "cap-B", "container_no": "TCLU1234567", "so_container_sqid": float64(4021)},
		map[string]any{"sqid": "cap-C", "container_no": "GESU5566778", "so_container_sqid": nil},
	})

	assert.Equal(t, []any{
		map[string]any{"const": "cap-C", "title": "GESU5566778"},
	}, field["oneOf"], "only the container SLPA has not paired yet")
}

// An entry missing either half would become an option nothing downstream could
// resolve: the save pairs by sqid, and the trader picks by number.
func TestConsolidationProjector_LeavesOutAnEntryMissingEitherHalf(t *testing.T) {
	field := projectConsolidation(t, []any{
		map[string]any{"sqid": "cap-A"},
		map[string]any{"container_no": "TCLU1234567"},
		map[string]any{"sqid": "cap-C", "container_no": "GESU5566778"},
	})

	assert.Equal(t, []any{
		map[string]any{"const": "cap-C", "title": "GESU5566778"},
	}, field["oneOf"])
}

// With nothing to offer the field is left as authored. An enum with no members
// is a field nothing can satisfy, which would leave the trader unable to submit
// and unable to see why; the step's own panel says what SLPA is holding.
func TestConsolidationProjector_LeavesTheFieldAsAuthoredWithNothingToOffer(t *testing.T) {
	assert.NotContains(t, projectConsolidation(t, []any{}), "oneOf")

	assert.NotContains(t, projectConsolidation(t, []any{
		map[string]any{"sqid": "cap-A", "container_no": "MSCU8492019", "so_container_sqid": "so-1"},
	}), "oneOf", "everything here is already consolidated")
}

// The trader's own answers are handed back so a refused save returns them to
// the choice they made rather than to an empty form.
func TestConsolidationProjector_CarriesTheTradersAnswersBack(t *testing.T) {
	projection, err := NewSLPAConsolidationProjector().Project(
		context.Background(),
		[]byte(consolidationForm),
		map[string]any{"consolidationform": map[string]any{"so_container_no": "DUMY0000001"}},
	)
	require.NoError(t, err)

	form, ok := projection.Content.(uiprojector.FormContent)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"so_container_no": "DUMY0000001"}, form.Data)
}
