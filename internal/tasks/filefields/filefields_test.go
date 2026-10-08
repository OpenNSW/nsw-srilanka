package filefields_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/filefields"
)

func parse(t *testing.T, files ...string) []filefields.Path {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"id": "x:render", "files": files})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filefields.Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", files, err)
	}
	return paths
}

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

// mark replaces a value with "<value>!", recording what it saw.
func mark(seen *[]string) func(string) (string, error) {
	return func(s string) (string, error) {
		*seen = append(*seen, s)
		return s + "!", nil
	}
}

func TestParse(t *testing.T) {
	files := []string{
		"traderinput.invoice_file_url",
		"fcau.analysis_certificates[*]",
		"userform.supportingDocuments[*].file",
		"userform.items[*].lines[*].certificate",
		"review.batches[*][*]",
		"Certificate-2.key",
	}
	paths := parse(t, files...)
	got := make([]string, len(paths))
	for i, p := range paths {
		got[i] = p.String()
	}
	if !reflect.DeepEqual(got, files) {
		t.Errorf("Parse = %q, want %q", got, files)
	}
}

func TestParseWithoutFiles(t *testing.T) {
	for _, raw := range []string{"", "null", `{}`, `{"id":"x:render","sections":{}}`, `{"files":[]}`} {
		paths, err := filefields.Parse(json.RawMessage(raw))
		if err != nil || len(paths) != 0 {
			t.Errorf("Parse(%q) = %v, %v; want none", raw, paths, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{
		`{"files":"traderinput.invoice"}`,
		`{"files":[1]}`,
		`[]`,
	} {
		if _, err := filefields.Parse(json.RawMessage(raw)); err == nil {
			t.Errorf("Parse(%s) succeeded", raw)
		}
	}
	for _, path := range []string{
		"",
		".a",
		"a.",
		"a..b",
		"a b",
		"a.b c",
		"[*]",
		"a.[*]",
		"a[0]",
		"a[]",
		"a[*]b",
		"a.**",
		"a.$b",
	} {
		raw, _ := json.Marshal(map[string]any{"files": []string{"ok.path", path}})
		_, err := filefields.Parse(raw)
		if err == nil {
			t.Errorf("Parse accepted file field %q", path)
		} else if !strings.Contains(err.Error(), "file field") {
			t.Errorf("Parse(%q): error %q does not name the file field", path, err)
		}
	}
}

func TestRebase(t *testing.T) {
	cases := []struct {
		path, prefix, want string
		ok                 bool
	}{
		{"traderinput.invoice", "traderinput", "invoice", true},
		{"traderinput.docs[*].file", "traderinput", "docs[*].file", true},
		{"traderinput.invoice", "", "traderinput.invoice", true},
		{"traderinput.invoice", "userform", "", false},
		{"traderinput", "traderinput", "", false},
		{"traderinput[*].file", "traderinput", "", false},
		{"traderinputs.invoice", "traderinput", "", false},
	}
	for _, tc := range cases {
		got, ok := parse(t, tc.path)[0].Rebase(tc.prefix)
		if ok != tc.ok || (ok && got.String() != tc.want) {
			t.Errorf("%q.Rebase(%q) = %q, %v; want %q, %v", tc.path, tc.prefix, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReplace(t *testing.T) {
	data := decode(t, `{
		"traderinput": {"invoice": "k1", "note": "not a file"},
		"fcau": {"certs": ["k2", "k3"]},
		"userform": {
			"docs": [{"file": "k4", "name": "a"}, {"file": "k5"}],
			"items": [{"lines": [{"cert": "k6"}, {"cert": "k7"}]}, {"lines": []}]
		},
		"review": {"batches": [["k8"], ["k9", "k10"]]}
	}`)
	var seen []string
	paths := parse(t,
		"traderinput.invoice",
		"fcau.certs[*]",
		"userform.docs[*].file",
		"userform.items[*].lines[*].cert",
		"review.batches[*][*]",
	)
	if err := filefields.Replace(data, paths, mark(&seen)); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	want := decode(t, `{
		"traderinput": {"invoice": "k1!", "note": "not a file"},
		"fcau": {"certs": ["k2!", "k3!"]},
		"userform": {
			"docs": [{"file": "k4!", "name": "a"}, {"file": "k5!"}],
			"items": [{"lines": [{"cert": "k6!"}, {"cert": "k7!"}]}, {"lines": []}]
		},
		"review": {"batches": [["k8!"], ["k9!", "k10!"]]}
	}`)
	if !reflect.DeepEqual(data, want) {
		t.Errorf("Replace left\n%v\nwant\n%v", data, want)
	}
	if len(seen) != 10 {
		t.Errorf("fn saw %q, want each of the ten files once", seen)
	}
}

// Anything that is not a non-empty string where a path leads is left alone.
func TestReplaceSkips(t *testing.T) {
	const doc = `{
		"empty": {"file": ""},
		"null": {"file": null},
		"number": {"file": 3},
		"object": {"file": {"key": "k"}},
		"notArray": {"files": "k"},
		"notObject": "k",
		"mixed": {"files": ["", null, 1, ["k"], {"k": "k"}]}
	}`
	data := decode(t, doc)
	var seen []string
	paths := parse(t,
		"empty.file", "null.file", "number.file", "object.file",
		"notArray.files[*]", "notObject.file", "mixed.files[*]",
		"missing.file", "empty.missing",
	)
	if err := filefields.Replace(data, paths, mark(&seen)); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("fn saw %q, want nothing", seen)
	}
	if !reflect.DeepEqual(data, decode(t, doc)) {
		t.Errorf("Replace changed data: %v", data)
	}
}

func TestReplaceStopsOnError(t *testing.T) {
	data := decode(t, `{"form": {"docs": ["ok", "bad", "later"]}}`)
	refused := errors.New("refused")
	var seen []string
	err := filefields.Replace(data, parse(t, "form.docs[*]"), func(s string) (string, error) {
		seen = append(seen, s)
		if s == "bad" {
			return "", refused
		}
		return s + "!", nil
	})
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v, want it to wrap fn's error", err)
	}
	if !strings.Contains(err.Error(), "form.docs[*]") {
		t.Errorf("err = %q, want it to name the path", err)
	}
	if !reflect.DeepEqual(seen, []string{"ok", "bad"}) {
		t.Errorf("fn saw %q, want it to stop at the error", seen)
	}
}

func TestReplaceRebased(t *testing.T) {
	payload := decode(t, `{"invoice": "k1", "other": "k2"}`)
	var seen []string
	var rebased []filefields.Path
	for _, p := range parse(t, "traderinput.invoice", "userform.other") {
		if r, ok := p.Rebase("traderinput"); ok {
			rebased = append(rebased, r)
		}
	}
	if err := filefields.Replace(payload, rebased, mark(&seen)); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !reflect.DeepEqual(seen, []string{"k1"}) {
		t.Errorf("fn saw %q, want only the step's own field", seen)
	}
}
