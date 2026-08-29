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

func TestInteger(t *testing.T) {
	m := map[string]any{"packages": float64(25), "as string": "25", "fractional": 25.9}
	assert.Equal(t, 25, Integer(m, "packages"))
	assert.Equal(t, 25, Integer(m, "as string"))
	assert.Equal(t, 25, Integer(m, "fractional"))
	assert.Zero(t, Integer(m, "missing"))
}
