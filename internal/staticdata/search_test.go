package staticdata

import (
	"encoding/json"
	"testing"
)

func TestRank_PrefixBeforeContains(t *testing.T) {
	options := []Option{
		{Const: "1", Title: "Port of Colombo"},
		{Const: "2", Title: "Colombo Port"},
		{Const: "3", Title: "Galle"},
	}

	got := rank(options, "col")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d (%v)", len(got), got)
	}
	if got[0].Title != "Colombo Port" || got[1].Title != "Port of Colombo" {
		t.Fatalf("expected prefix before contains, got %q then %q", got[0].Title, got[1].Title)
	}
}

func TestRank_ExactTitleBeforePrefix(t *testing.T) {
	options := []Option{
		{Const: "1", Title: "Colombo Port"},
		{Const: "2", Title: "Colombo"},
	}

	got := rank(options, "COLOMBO")
	if len(got) != 2 || got[0].Const != "2" || got[1].Const != "1" {
		t.Fatalf("expected exact title first, got %+v", got)
	}
}

func TestRank_ConstPrefixBeforeTitleContains(t *testing.T) {
	options := []Option{
		{Const: "X", Title: "The Colombo Harbor"},
		{Const: "COL", Title: "Harbor"},
	}

	got := rank(options, "col")
	if len(got) != 2 || got[0].Const != "COL" || got[1].Const != "X" {
		t.Fatalf("expected const prefix before title contains, got %+v", got)
	}
}

func TestRank_WordBoundaryBeforeSubstring(t *testing.T) {
	options := []Option{
		{Const: "USSJL", Title: "SOUTHAMPTON"},
		{Const: "USHTO", Title: "EAST HAMPTON"},
		{Const: "USPHF", Title: "HAMPTON/HAMPTON ROADS"},
	}

	got := rank(options, "hampton")
	if len(got) != 3 || got[0].Const != "USPHF" || got[1].Const != "USHTO" || got[2].Const != "USSJL" {
		t.Fatalf("expected prefix, then a later word, then a substring, got %+v", got)
	}
}

func TestRank_WordBoundaryRoads(t *testing.T) {
	options := []Option{
		{Const: "USUJS", Title: "HAMPTON"},
		{Const: "USPHF", Title: "HAMPTON/HAMPTON ROADS"},
	}

	got := rank(options, "roads")
	if len(got) != 1 || got[0].Const != "USPHF" {
		t.Fatalf("expected the word-boundary match, got %+v", got)
	}
}

func TestRank_ShorterTitleThenConst(t *testing.T) {
	options := []Option{
		{Const: "USHPN", Title: "HAMPTON"},
		{Const: "GBHMP", Title: "HAMPTON"},
		{Const: "USHPF", Title: "HAMPTON FALLS"},
	}

	got := rank(options, "hamp")
	if len(got) != 3 || got[0].Const != "GBHMP" || got[1].Const != "USHPN" || got[2].Const != "USHPF" {
		t.Fatalf("expected shorter titles, then const, got %+v", got)
	}
}

func TestRank_EqualLengthOrdersByConstNotTitle(t *testing.T) {
	options := []Option{
		{Const: "Z", Title: "PORT A"},
		{Const: "A", Title: "PORT B"},
	}

	got := rank(options, "port")
	if len(got) != 2 || got[0].Const != "A" || got[1].Const != "Z" {
		t.Fatalf("expected const order for equal-length titles, got %+v", got)
	}
}

func TestRank_ExactConst(t *testing.T) {
	options := []Option{
		{Const: "USUJS", Title: "HAMPTON"},
		{Const: "GBHMP", Title: "HAMPTON"},
	}

	got := rank(options, "usujs")
	if len(got) != 1 || got[0].Const != "USUJS" {
		t.Fatalf("expected the exact const, got %+v", got)
	}
}

func TestRank_EmptyQueryKeepsArtifactOrder(t *testing.T) {
	options := []Option{
		{Const: "b", Title: "Beta"},
		{Const: "a", Title: "Alpha"},
	}

	got := rank(options, "   ")
	if len(got) != 2 || got[0].Const != "b" || got[1].Const != "a" {
		t.Fatalf("expected artifact order, got %+v", got)
	}
}

func TestSearch_LimitAndOffset(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"const":"1","title":"Port of Colombo"},
		{"const":"2","title":"Colombo"},
		{"const":"3","title":"Colombo Port"},
		"not-an-object",
		{"const":"4","title":"Galle"}
	]}`)

	got, err := Search(raw, "colombo", "", 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 3 || got.Offset != 1 || got.Limit != 1 {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].Const != "3" {
		t.Fatalf("expected the second-ranked match, got %+v", got.Items)
	}
}

func TestSearch_SkipsEntriesWithoutConstAndTitle(t *testing.T) {
	raw := json.RawMessage(`{"data":["0101",{"const":"LK"},{"title":"Sri Lanka"},{"const":"GB","title":"United Kingdom"}]}`)

	got, err := Search(raw, "", "", 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].Const != "GB" {
		t.Fatalf("expected only the complete option, got %+v", got)
	}
}

func TestSearch_ParentScopesPageAndTotal(t *testing.T) {
	raw := json.RawMessage(`{"data":[
		{"const":"a","title":"Tea Plant","parents":["Tea"]},
		{"const":"b","title":"Tea Bush","parents":["Tea","Camellia"]},
		{"const":"c","title":"Coconut","parents":["Palm"]},
		{"const":"d","title":"Unscoped"}
	]}`)

	got, err := Search(raw, "", "Tea", 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 2 || got.Offset != 1 || got.Limit != 1 || len(got.Items) != 1 || got.Items[0].Const != "b" {
		t.Fatalf("expected the second Tea row and a scoped total, got %+v", got)
	}

	all, err := Search(raw, "", "", 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if all.Total != 4 {
		t.Fatalf("expected an empty parent to keep every row, got total %d", all.Total)
	}
}
