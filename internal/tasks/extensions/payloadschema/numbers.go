package payloadschema

import (
	"encoding/json"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
)

// maxNumber is the largest number, either side of zero, a user may enter in
// any form, whatever its schema says. It is the largest value of a NUMERIC(15, 2)
// column, which is how the database stores money, so a number a form accepts
// can still be stored as an amount. A form that needs a tighter limit sets its
// own "maximum".
const maxNumber = 9_999_999_999_999.99

// msgNumberTooLarge is the violation for a number past maxNumber.
var msgNumberTooLarge = "must be between -" + strconv.FormatFloat(maxNumber, 'f', -1, 64) +
	" and " + strconv.FormatFloat(maxNumber, 'f', -1, 64)

// numberViolations returns a violation for every number in data past
// maxNumber. Read-only values are left out: the server stored them, and the
// read-only check holds them to what it stored.
func (f *readOnlyForm) numberViolations(data map[string]any) []Violation {
	var out []Violation
	checkNumbers(data, f.root, "", &out)
	return out
}

// checkNumbers adds a violation to out for every number in v past maxNumber,
// v being the value at JSON Pointer at and n its read-only node (nil when
// nothing in v is read-only).
func checkNumbers(v any, n *roNode, at string, out *[]Violation) {
	if n != nil && n.readOnly() {
		return
	}
	switch x := v.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(x)) {
			child := x[key]
			var cn *roNode
			if n != nil {
				cn = n.property(key)
			}
			checkNumbers(child, cn, at+"/"+escapeToken(key), out)
		}
	case []any:
		var row *roNode
		if n != nil {
			row = n.row()
		}
		for i, child := range x {
			checkNumbers(child, row, at+"/"+strconv.Itoa(i), out)
		}
	default:
		checkNumericValue(v, at, out)
	}
}

func checkNumericValue(v any, at string, out *[]Violation) {
	switch x := v.(type) {
	case float64:
		checkNumberLimit(x, at, out)
	case float32:
		checkNumberLimit(float64(x), at, out)
	case json.Number:
		if f, err := x.Float64(); err != nil || math.Abs(f) > maxNumber {
			*out = append(*out, Violation{Path: at, Message: msgNumberTooLarge})
		}
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			checkNumberLimit(float64(rv.Int()), at, out)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			checkNumberLimit(float64(rv.Uint()), at, out)
		}
	}
}

func checkNumberLimit(x float64, at string, out *[]Violation) {
	if math.Abs(x) > maxNumber {
		*out = append(*out, Violation{Path: at, Message: msgNumberTooLarge})
	}
}
