// Package document holds what the SLC Edge integrations share about the
// documents they exchange with ASYCUDA World: the registered reference (§4.1),
// reading the decoded JSON the forms and answers arrive as, and the declaration
// retrieve (§6.7). The cusdec, cdn and boatnote packages use it, so a document
// is read the same way wherever it is used.
package document

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Reference is the registered reference of a declaration or a dispatch note
// (§4.1).
type Reference struct {
	Year   string `json:"year"`
	Office string `json:"office"`
	Serial string `json:"serial"`
	Number int    `json:"number"`
}

// IsValid reports whether all fields of the reference are populated and valid.
func (r Reference) IsValid() bool {
	return r.Year != "" && r.Office != "" && r.Serial != "" && r.Number > 0
}

// String is the office/year/serial/number display form the integration results
// write into the workflow.
func (r Reference) String() string {
	return fmt.Sprintf("%s/%s/%s/%d", r.Office, r.Year, r.Serial, r.Number)
}

// ParseReference splits the office/year/serial/number display form of a
// registered reference back into its four elements (§4.1). That display form
// is what the integration results write into the workflow, so it is what every
// later step works from. Each error completes the sentence "The …." the cdn
// package shows the trader.
func ParseReference(s string) (Reference, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 4 {
		return Reference{}, fmt.Errorf(
			"customs declaration reference %q is not in the expected office/year/serial/number form", s)
	}
	number, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil {
		return Reference{}, fmt.Errorf(
			"customs declaration reference %q does not end in a declaration number", s)
	}
	ref := Reference{
		Office: strings.TrimSpace(parts[0]),
		Year:   strings.TrimSpace(parts[1]),
		Serial: strings.TrimSpace(parts[2]),
		Number: number,
	}
	if !ref.IsValid() {
		return Reference{}, fmt.Errorf("customs declaration reference %q is incomplete", s)
	}
	return ref, nil
}

// --- decoded JSON -----------------------------------------------------------
//
// Forms and answers arrive as decoded JSON, so every value is any and every
// number is float64. These read through that without the caller repeating type
// assertions, returning the zero value for anything absent or the wrong shape.

// Nested returns the object under key.
func Nested(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

// Str returns the string under key, trimmed.
func Str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

// Text returns the value under key as text: a string, trimmed, or a number,
// formatted (see FormatNumber). Answers carry some codes as numbers.
func Text(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return FormatNumber(v)
	default:
		return ""
	}
}

// Float reads a decoded JSON number, or a numeric string, as a float64.
func Float(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// FormatNumber renders a number without a trailing ".0": 500 stays "500".
func FormatNumber(f float64) string {
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Objects returns the objects in a decoded JSON array, skipping anything else.
func Objects(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// WholeNumber reads a count from the decoded JSON forms it can take: a number,
// or a numeric string.
func WholeNumber(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("%v has a fraction", n)
		}
		return int(n), nil
	case int:
		return n, nil
	case json.Number:
		return strconv.Atoi(n.String())
	case string:
		return strconv.Atoi(strings.TrimSpace(n))
	default:
		return 0, fmt.Errorf("got %T", v)
	}
}

// --- declaration retrieve (§6.7) ---------------------------------------------

// DeclarationRetrievePath is SLC Edge's declaration retrieve endpoint.
const DeclarationRetrievePath = "/api/declaration/v1/retrieve"

// DeclarationRetrieve is the §6.7 request body. The specification's example
// names the reference cusDecRef, as the amendment notification does (§6.5.4).
type DeclarationRetrieve struct {
	CusdecRef     Reference `json:"cusDecRef"`
	DeclarantCode string    `json:"declarantCode"`
	CompanyCode   string    `json:"companyCode"`
}

// NewDeclarationRetrieve names the declaration by its registered reference, in
// its display form, with the declarant and the exporter's company code the
// trader declared it under (TraderCodes): SLC Edge returns a declaration only to
// the codes it was registered with. An unreadable reference is sent empty, so
// the endpoint's refusal is what the step records.
func NewDeclarationRetrieve(cusdecRef string, form map[string]any) DeclarationRetrieve {
	ref, _ := ParseReference(cusdecRef)
	declarant, company := TraderCodes(form)
	return DeclarationRetrieve{CusdecRef: ref, DeclarantCode: declarant, CompanyCode: company}
}

// TraderCodes are the declarant's code and the exporter's company code from the
// trader's declaration form, which SLC Edge's retrieve endpoints match a
// document against (§6.7, §7.8).
func TraderCodes(form map[string]any) (declarant, company string) {
	traders := Nested(form, "traders")
	return Str(Nested(traders, "declarant"), "code"), Str(Nested(traders, "exporter"), "code")
}

// RetrievedDeclaration finds the declaration in a §6.7 answer. The
// specification shows it under payload.cusdec; payload.cusDec, the amendment's
// spelling, and an unwrapped cusdec are read too, since the example is not
// consistent about either. It returns nil when there is none.
func RetrievedDeclaration(resp map[string]any) map[string]any {
	return firstObject(resp, "cusdec", "cusDec")
}

// RetrievedCDNRefs are the dispatch notes a §6.7 answer lists against the
// declaration (cdnRefs), each with its reference and status, as they came.
func RetrievedCDNRefs(resp map[string]any) []map[string]any {
	raw, ok := Nested(resp, "payload")["cdnRefs"]
	if !ok {
		raw = resp["cdnRefs"]
	}
	return Objects(raw)
}

// FirstObject finds the object under one of keys in an SLC Edge answer, which
// the specification shows under payload but not consistently (§6.7, §7.8).
func FirstObject(resp map[string]any, keys ...string) map[string]any {
	return firstObject(resp, keys...)
}

func firstObject(resp map[string]any, keys ...string) map[string]any {
	for _, holder := range []map[string]any{Nested(resp, "payload"), resp} {
		for _, key := range keys {
			if obj, ok := holder[key].(map[string]any); ok && len(obj) > 0 {
				return obj
			}
		}
	}
	return nil
}
