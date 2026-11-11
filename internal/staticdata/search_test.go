package staticdata

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

// values returns the value of key in each item, in order.
func values(t *testing.T, items []json.RawMessage, key string) []string {
	t.Helper()
	out := make([]string, 0, len(items))
	for _, item := range items {
		var fields map[string]any
		if err := json.Unmarshal(item, &fields); err != nil {
			t.Fatalf("item is not an object: %s", item)
		}
		out = append(out, fmt.Sprint(fields[key]))
	}
	return out
}

// searchValues searches the rows in data (a JSON array) and returns key of each match, in
// rank order.
func searchValues(t *testing.T, data, query string, searchKeys []string, key string) []string {
	t.Helper()
	got, err := Search(json.RawMessage(`{"data":`+data+`}`), query, "", searchKeys, 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return values(t, got.Items, key)
}

func expectValues(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSearch_PrefixBeforeContains(t *testing.T) {
	data := `[
		{"const":"1","title":"Port of Colombo"},
		{"const":"2","title":"Colombo Port"},
		{"const":"3","title":"Galle"}
	]`
	expectValues(t, searchValues(t, data, "col", nil, "const"), []string{"2", "1"})
}

func TestSearch_ExactTitleBeforePrefix(t *testing.T) {
	data := `[
		{"const":"1","title":"Colombo Port"},
		{"const":"2","title":"Colombo"}
	]`
	expectValues(t, searchValues(t, data, "COLOMBO", nil, "const"), []string{"2", "1"})
}

func TestSearch_ConstPrefixBeforeTitleContains(t *testing.T) {
	data := `[
		{"const":"X","title":"The Colombo Harbor"},
		{"const":"COL","title":"Harbor"}
	]`
	expectValues(t, searchValues(t, data, "col", nil, "const"), []string{"COL", "X"})
}

func TestSearch_WordBoundaryBeforeSubstring(t *testing.T) {
	data := `[
		{"const":"USSJL","title":"SOUTHAMPTON"},
		{"const":"USHTO","title":"EAST HAMPTON"},
		{"const":"USPHF","title":"HAMPTON/HAMPTON ROADS"}
	]`
	expectValues(t, searchValues(t, data, "hampton", nil, "const"), []string{"USPHF", "USHTO", "USSJL"})
}

func TestSearch_WordBoundaryRoads(t *testing.T) {
	data := `[
		{"const":"USUJS","title":"HAMPTON"},
		{"const":"USPHF","title":"HAMPTON/HAMPTON ROADS"}
	]`
	expectValues(t, searchValues(t, data, "roads", nil, "const"), []string{"USPHF"})
}

func TestSearch_ShorterTitleThenConst(t *testing.T) {
	data := `[
		{"const":"USHPN","title":"HAMPTON"},
		{"const":"GBHMP","title":"HAMPTON"},
		{"const":"USHPF","title":"HAMPTON FALLS"}
	]`
	expectValues(t, searchValues(t, data, "hamp", nil, "const"), []string{"GBHMP", "USHPN", "USHPF"})
}

func TestSearch_EqualLengthOrdersByConstNotTitle(t *testing.T) {
	data := `[
		{"const":"Z","title":"PORT A"},
		{"const":"A","title":"PORT B"}
	]`
	expectValues(t, searchValues(t, data, "port", nil, "const"), []string{"A", "Z"})
}

func TestSearch_ExactConst(t *testing.T) {
	data := `[
		{"const":"USUJS","title":"HAMPTON"},
		{"const":"GBHMP","title":"HAMPTON"}
	]`
	expectValues(t, searchValues(t, data, "usujs", nil, "const"), []string{"USUJS"})
}

func TestSearch_EmptyQueryKeepsArtifactOrder(t *testing.T) {
	data := `[
		{"const":"b","title":"Beta"},
		{"const":"a","title":"Alpha"}
	]`
	expectValues(t, searchValues(t, data, "   ", nil, "const"), []string{"b", "a"})
}

func TestSearch_EmptySearchKeysUseTheDefault(t *testing.T) {
	data := `[
		{"const":"LK","title":"Sri Lanka"},
		{"const":"GB","title":"United Kingdom"}
	]`
	expectValues(t, searchValues(t, data, "lk", []string{}, "const"), []string{"LK"})
	expectValues(t, searchValues(t, data, "united", []string{}, "const"), []string{"GB"})
}

func TestSearch_LimitAndOffset(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"const":"1","title":"Port of Colombo"},
		{"const":"2","title":"Colombo"},
		{"const":"3","title":"Colombo Port"},
		"not-an-object",
		{"const":"4","title":"Galle"}
	]}`)

	got, err := Search(raw, "colombo", "", nil, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 3 || got.Offset != 1 || got.Limit != 1 {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	expectValues(t, values(t, got.Items, "const"), []string{"3"})
}

func TestSearch_ErrorsWhenNotAnArtifact(t *testing.T) {
	if _, err := Search(json.RawMessage(`not json`), "", "", nil, 0, 20); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSearch_KeepsRowsWithoutConstOrTitle(t *testing.T) {
	data := `[
		{"const":"LK"},
		{"title":"Sri Lanka"},
		{"const":"GB","title":""}
	]`
	expectValues(t, searchValues(t, data, "lk", nil, "const"), []string{"LK"})
	expectValues(t, searchValues(t, data, "sri", nil, "title"), []string{"Sri Lanka"})
	expectValues(t, searchValues(t, data, "gb", nil, "const"), []string{"GB"})
}

func TestSearch_ReturnsTheWholeRow(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"const":"22V4","title":"20x8 6, Mechanical","description":"20x8 6, Mechanical ventilation system","group":"Ventilated","size":20}
	]}`)

	got, err := Search(raw, "22v4", "", nil, 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one match, got %d", len(got.Items))
	}
	var row map[string]any
	if err := json.Unmarshal(got.Items[0], &row); err != nil {
		t.Fatalf("item is not an object: %s", got.Items[0])
	}
	want := map[string]any{
		"const":       "22V4",
		"title":       "20x8 6, Mechanical",
		"description": "20x8 6, Mechanical ventilation system",
		"group":       "Ventilated",
		"size":        float64(20),
	}
	if fmt.Sprint(row) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", row, want)
	}
}

func TestSearch_CustomSearchKeys(t *testing.T) {
	data := `[
		{"foo":"A1","boo":"Alpha"},
		{"foo":"B2","boo":"Beta"},
		{"foo":"AL9","boo":"Gamma"}
	]`
	// "al" is a prefix of boo "Alpha" and of foo "AL9". Both rank in the prefix tier, and their
	// first keys (boo) have equal length, so the next key (foo) decides: "a1" before "al9".
	expectValues(t, searchValues(t, data, "al", []string{"boo", "foo"}, "foo"), []string{"A1", "AL9"})
	expectValues(t, searchValues(t, data, "beta", []string{"boo", "foo"}, "foo"), []string{"B2"})
}

func TestSearch_MatchesOnlyTheSearchKeys(t *testing.T) {
	data := `[
		{"foo":"A1","boo":"Alpha"}
	]`
	expectValues(t, searchValues(t, data, "alpha", []string{"foo"}, "foo"), []string{})
	expectValues(t, searchValues(t, data, "alpha", nil, "foo"), []string{})
	expectValues(t, searchValues(t, data, "alpha", []string{"boo"}, "foo"), []string{"A1"})
}

func TestSearch_FirstSearchKeyLengthBreaksTies(t *testing.T) {
	data := `[
		{"foo":"1","boo":"Colombo Harbour"},
		{"foo":"2","boo":"Colombo"}
	]`
	// Both are in the prefix tier; the shorter first-key value wins.
	expectValues(t, searchValues(t, data, "col", []string{"boo", "foo"}, "foo"), []string{"2", "1"})
}

func TestSearch_NumbersAndBooleansAreSearchable(t *testing.T) {
	data := `[
		{"code":42,"name":"Answer"},
		{"code":true,"name":"Yes"},
		{"code":["42"],"name":"List"},
		{"code":{"value":"42"},"name":"Object"},
		{"code":null,"name":"Null"}
	]`
	expectValues(t, searchValues(t, data, "42", []string{"code"}, "name"), []string{"Answer"})
	expectValues(t, searchValues(t, data, "true", []string{"code"}, "name"), []string{"Yes"})
}

func TestSearch_ParentScopesPageAndTotal(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"const":"a","title":"Tea Plant","parents":["Tea"]},
		{"const":"b","title":"Tea Bush","parents":["Tea","Camellia"]},
		{"const":"c","title":"Coconut","parents":["Palm"]},
		{"const":"d","title":"Unscoped"}
	]}`)

	got, err := Search(raw, "", "Tea", nil, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 2 || got.Offset != 1 || got.Limit != 1 {
		t.Fatalf("expected a scoped total, got %+v", got)
	}
	expectValues(t, values(t, got.Items, "const"), []string{"b"})

	all, err := Search(raw, "", "", nil, 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if all.Total != 4 {
		t.Fatalf("expected an empty parent to keep every row, got total %d", all.Total)
	}
}

func TestSearch_MalformedParentsMatchNoParent(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"const":"a","title":"Tea Plant","parents":"Tea"},
		{"const":"b","title":"Tea Bush","parents":["Tea"]}
	]}`)

	got, err := Search(raw, "", "Tea", nil, 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectValues(t, values(t, got.Items, "const"), []string{"b"})
}
