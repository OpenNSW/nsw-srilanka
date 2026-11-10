package staticdata

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

func TestTitles_Of(t *testing.T) {
	reg := newTestRegistry(t, "ports", "1", "ports.json", []byte(`{"data":[
		{"const":"LKCMB","title":"COLOMBO"},
		{"const":"NLRTM","title":"ROTTERDAM"},
		{"const":"LKCMB","title":"COLOMBO (DUPLICATE)"},
		"not an object"
	]}`))

	titles, err := NewTitles(reg).Of(context.Background(), "ports", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"LKCMB": "COLOMBO", "NLRTM": "ROTTERDAM"}
	if len(titles) != len(want) {
		t.Fatalf("got %v, want %v", titles, want)
	}
	for code, title := range want {
		if titles[code] != title {
			t.Errorf("%s: got %q, want %q", code, titles[code], title)
		}
	}
}

// An artifact edited in a manifest.json working copy is seen on the next call:
// Titles keeps nothing between calls, and the registry fetches a working copy's
// files every time.
func TestTitles_OfSeesAnEditedArtifact(t *testing.T) {
	loader := testutil.MemLoader{
		"ports.json":    []byte(`{"data":[{"const":"LKCMB","title":"COLOMBO"}]}`),
		"manifest.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","version":"1","path":"ports.json"}]}`),
	}
	set, err := artifact.LoadSet(context.Background(), loader)
	if err != nil {
		t.Fatal(err)
	}
	titles := NewTitles(set)
	if _, err := titles.Of(context.Background(), "ports", "1"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	loader["ports.json"] = []byte(`{"data":[{"const":"LKCMB","title":"PORT OF COLOMBO"}]}`)

	got, err := titles.Of(context.Background(), "ports", "1")
	if err != nil || got["LKCMB"] != "PORT OF COLOMBO" {
		t.Fatalf("second call: got %v, %v", got, err)
	}
}

func TestTitles_OfUnknown(t *testing.T) {
	reg := newTestRegistry(t, "ports", "1", "ports.json", []byte(`{"data":[]}`))

	_, err := NewTitles(reg).Of(context.Background(), "ports", "2")
	if !errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("got %v, want artifact.ErrNotFound", err)
	}
}

func TestTitles_OfRowWithoutTitle(t *testing.T) {
	reg := newTestRegistry(t, "ports", "1", "ports.json", []byte(`{"data":[{"const":"LKCMB"}]}`))

	_, err := NewTitles(reg).Of(context.Background(), "ports", "1")
	if !errors.Is(err, artifact.ErrInvalid) {
		t.Fatalf("got %v, want artifact.ErrInvalid", err)
	}
}
