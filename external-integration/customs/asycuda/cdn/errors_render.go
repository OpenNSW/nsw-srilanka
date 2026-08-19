package cdn

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Rendering for the §4.4 errors object — the segment-keyed shape ASYCUDA uses
// for validation failures, where "0" is the document as a whole and any other
// key is an item number.
//
// Both directions of the CDN exchange need this: the §7.1 submission
// acknowledgement can carry error detail inline, and the §7.2 integration result
// carries it in the errors object. It therefore lives with neither of them.

// describeErrors renders the §4.4 segment-keyed errors object from an
// integration result as a trader-facing message.
//
// The raw JSON must not be passed through verbatim: `{"0":[{"code":331,...}]}`
// in front of a trader who has no way to read it is worse than no reason at all.
// Anything that does not parse falls back to the raw text — an unreadable reason
// still beats a blank one.
func describeErrors(raw json.RawMessage) string {
	const intro = "Sri Lanka Customs could not integrate your cargo dispatch note:"
	const outro = "\n\nPlease correct the highlighted fields and resubmit."

	if len(raw) == 0 {
		return "Sri Lanka Customs rejected the cargo dispatch note without giving a reason."
	}

	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return strings.TrimSpace(string(raw))
	}

	bullets := validationBullets(map[string]any{"errors": parsed})
	if len(bullets) == 0 {
		return strings.TrimSpace(string(raw))
	}
	return intro + "\n\n" + strings.Join(bullets, "\n") + outro
}

// validationBullets renders the response's error detail as markdown bullets,
// accepting both the §4.4 segment-keyed object and the flat array form.
func validationBullets(resp map[string]any) []string {
	switch errs := resp["errors"].(type) {
	case []any:
		return entryBullets(errs, "")
	case map[string]any:
		// §4.4: "0" is the note as a whole, any other key is an item number.
		var bullets []string
		for _, key := range sortedSegmentKeys(errs) {
			entries, ok := errs[key].([]any)
			if !ok {
				continue
			}
			prefix := ""
			if key != "0" {
				prefix = "Item " + key + ": "
			}
			bullets = append(bullets, entryBullets(entries, prefix)...)
		}
		return bullets
	default:
		return nil
	}
}

func entryBullets(entries []any, prefix string) []string {
	bullets := make([]string, 0, len(entries))
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			bullets = append(bullets, "- "+prefix+fmt.Sprintf("%v", e))
			continue
		}
		// §4.4 names the text "description"; the submission acknowledgement
		// uses "message". Either is the trader-facing reason.
		msg := stringField(m, "description")
		if msg == "" {
			msg = stringField(m, "message")
		}
		if msg == "" {
			msg = stringField(m, "code")
		}
		if field := stringField(m, "fieldRef"); field != "" && msg != "" {
			msg = fmt.Sprintf("%s _(%s)_", msg, field)
		}
		if msg != "" {
			bullets = append(bullets, "- "+prefix+msg)
		}
	}
	return bullets
}

// stringField returns m[key] as a string, or "" if absent or not a string.
// Error codes arrive as JSON numbers, so those are rendered too.
func stringField(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return formatCode(v)
	default:
		return ""
	}
}

// sortedSegmentKeys orders the §4.4 errors object so the general segment ("0")
// leads and item segments follow numerically, rather than in Go's randomized
// map order.
func sortedSegmentKeys(errs map[string]any) []string {
	keys := make([]string, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, erri := strconv.Atoi(keys[i])
		nj, errj := strconv.Atoi(keys[j])
		if erri == nil && errj == nil {
			return ni < nj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// formatCode renders an error code, which arrives as a JSON number and so
// decodes to float64, without the trailing ".0" a plain format would add.
func formatCode(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
