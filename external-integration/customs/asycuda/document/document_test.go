package document

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reference reads back from the display form the integration results write.
func TestParseReference(t *testing.T) {
	ref, err := ParseReference(" CBEX1 / 2026 / E / 59 ")
	require.NoError(t, err)
	assert.Equal(t, Reference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 59}, ref)
	assert.Equal(t, "CBEX1/2026/E/59", ref.String())

	for _, s := range []string{"", "CBEX1-59", "CBEX1/2026/E/x", "CBEX1/2026//59", "CBEX1/2026/E/0"} {
		_, err := ParseReference(s)
		assert.Error(t, err, s)
	}
}

// The declaration is read however the answer carries it, and the retrieve is
// sent with the codes the trader declared it under.
func TestDeclarationRetrieve(t *testing.T) {
	for name, answer := range map[string]string{
		"payload.cusdec": `{"payload": {"cusdec": {"id": 1}, "cdnRefs": [{"number": 1}, "x"]}}`,
		"payload.cusDec": `{"payload": {"cusDec": {"id": 1}, "cdnRefs": [{"number": 1}]}}`,
		"unwrapped":      `{"cusdec": {"id": 1}, "cdnRefs": [{"number": 1}]}`,
	} {
		var resp map[string]any
		require.NoError(t, json.Unmarshal([]byte(answer), &resp))
		assert.Equal(t, map[string]any{"id": float64(1)}, RetrievedDeclaration(resp), name)
		assert.Equal(t, []map[string]any{{"number": float64(1)}}, RetrievedCDNRefs(resp), name)
	}
	assert.Nil(t, RetrievedDeclaration(map[string]any{"payload": map[string]any{}}))

	form := map[string]any{"traders": map[string]any{
		"declarant": map[string]any{"code": " D0012345 "},
		"exporter":  map[string]any{"code": "900000000001"},
	}}
	assert.Equal(t, DeclarationRetrieve{
		CusdecRef:     Reference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 59},
		DeclarantCode: "D0012345",
		CompanyCode:   "900000000001",
	}, NewDeclarationRetrieve("CBEX1/2026/E/59", form))
}

// Text reads codes that answers carry as numbers.
func TestText(t *testing.T) {
	m := map[string]any{"s": " a ", "n": float64(2026), "f": 1.5, "b": true}
	assert.Equal(t, "a", Text(m, "s"))
	assert.Equal(t, "2026", Text(m, "n"))
	assert.Equal(t, "1.5", Text(m, "f"))
	assert.Equal(t, "", Text(m, "b"))
	assert.Equal(t, "", Str(m, "n"), "Str reads strings only")
}

// Only a cancelled note is left out. A status other than the known two is
// counted as live: counting a live note as missing would ask the trader for a
// dispatch note Customs already holds.
func TestActiveCDNRefs_LeavesOutOnlyCancelledNotes(t *testing.T) {
	note := func(number int, status string) map[string]any {
		return map[string]any{"office": "CBEX1", "year": "2026", "serial": "C", "number": number, "status": status}
	}

	refs := ActiveCDNRefs([]map[string]any{
		note(1, "Registered"),
		note(2, "Cancelled"),
		note(3, "CANCELLED"),
		note(4, "Exited"),
		note(5, ""),
	})

	numbers := make([]int, 0, len(refs))
	for _, r := range refs {
		numbers = append(numbers, r.Number)
	}
	assert.Equal(t, []int{1, 4, 5}, numbers)
}
