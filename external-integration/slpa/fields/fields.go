// Package fields reads the values a trader's JSONForms submission carries.
//
// A submitted form reaches an integration as decoded JSON — every value is any,
// every number a float64 — and the widget a field was rendered with decides
// which Go type actually arrives. These accessors read through that, so a
// payload builder states which field it wants rather than repeating the type
// assertions, and every SLPA integration reads a form the same way.
package fields

import (
	"strconv"
	"strings"
)

// String reads a text field.
//
// A number is accepted and rendered: coded values a form offers as a choice —
// a container size, say — arrive as numbers where the CMS wants the digits as
// text.
func String(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

// Number reads a numeric field.
//
// A numeric string is parsed rather than refused: JSONForms number widgets
// round-trip as strings in some browsers, and a volume the trader did enter
// must not read as zero — the CMS would refuse the submission for a value that
// is on their screen.
//
// Anything unreadable is zero, which every caller checks against its own
// minimum, so an absent value and an unparseable one are refused the same way.
func Number(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// Integer reads a whole-number field, truncating as the CMS's own fields do.
func Integer(m map[string]any, key string) int { return int(Number(m, key)) }
