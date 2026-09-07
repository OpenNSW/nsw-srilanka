package fields

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	m := map[string]any{
		"text":    "  MSCU8492019 ",
		"coded":   float64(40),
		"decimal": 40.5,
		"counted": 3,
		"other":   map[string]any{},
	}
	assert.Equal(t, "MSCU8492019", String(m, "text"))
	assert.Equal(t, "40", String(m, "coded"), "a coded choice must not render as 40.000000")
	assert.Equal(t, "40.5", String(m, "decimal"))
	assert.Equal(t, "3", String(m, "counted"))
	assert.Empty(t, String(m, "other"))
	assert.Empty(t, String(m, "missing"))
}

// The string case is the one that matters in practice: a JSONForms number
// widget round-trips as a string in some browsers, and a volume read as zero
// gets the trader's submission refused over a value that is on their screen.
func TestNumber(t *testing.T) {
	m := map[string]any{
		"float":        25.5,
		"int":          3,
		"as string":    " 25.5 ",
		"not a number": "twenty",
		"wrong shape":  []any{1},
	}
	assert.Equal(t, 25.5, Number(m, "float"))
	assert.Equal(t, 3.0, Number(m, "int"))
	assert.Equal(t, 25.5, Number(m, "as string"))
	assert.Zero(t, Number(m, "not a number"))
	assert.Zero(t, Number(m, "wrong shape"))
	assert.Zero(t, Number(m, "missing"))
}

// Both shapes occur, and which one a reader gets depends on whether the value
// reached it through storage or through a mapping made in the same process. A
// reader that accepted only []any silently found no rows in the other, which
// read as "SLPA no longer holds this" right after a successful save.
func TestRows_ReadsBothShapesARecordCarries(t *testing.T) {
	want := []map[string]any{{"sqid": "cap-A"}, {"sqid": "cap-B"}}

	assert.Equal(t, want, Rows([]any{
		map[string]any{"sqid": "cap-A"},
		map[string]any{"sqid": "cap-B"},
	}), "the []any a task record holds after a round trip through JSON")

	assert.Equal(t, want, Rows([]map[string]any{
		{"sqid": "cap-A"},
		{"sqid": "cap-B"},
	}), "the typed slice a step recorded in Go")
}

func TestRows_SkipsWhatIsNotARecordAndReportsWhatIsNotAList(t *testing.T) {
	assert.Equal(t, []map[string]any{{"sqid": "cap-A"}},
		Rows([]any{"MSCU8492019", nil, map[string]any{"sqid": "cap-A"}}),
		"a list is worth reading for the rows it does hold")

	assert.Empty(t, Rows([]any{}))
	assert.NotNil(t, Rows([]any{}), "an empty list is still a list")

	// nil so a caller can tell "not a list" from "no rows" and say so.
	assert.Nil(t, Rows("MSCU8492019"))
	assert.Nil(t, Rows(map[string]any{"sqid": "cap-A"}))
	assert.Nil(t, Rows(nil))
}

func TestInteger(t *testing.T) {
	m := map[string]any{"packages": float64(25), "as string": "25", "fractional": 25.9}
	assert.Equal(t, 25, Integer(m, "packages"))
	assert.Equal(t, 25, Integer(m, "as string"))
	assert.Equal(t, 25, Integer(m, "fractional"))
	assert.Zero(t, Integer(m, "missing"))
}
