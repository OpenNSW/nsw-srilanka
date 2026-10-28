package payloadschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// noFiles is the file lookup of a form with no x-file rules: it fails, so a
// test that looks a file up when it shouldn't fails with it.
func noFiles(context.Context, string) (storage.File, error) {
	return storage.File{}, errors.New("no file should be looked up")
}

// compileForTest compiles schema for a test of rules other than x-file's: a
// file it looks up fails the lookup.
func compileForTest(schema []byte) (*jsonschema.Schema, error) {
	return compile(schema, newStoredFiles(context.Background(), noFiles))
}

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// fakeFiles serves the files storage holds from a map, and counts the lookups
// of each. "unreachable" can't be looked up.
type fakeFiles struct {
	files   map[string]storage.File
	lookups map[string]int
}

func newFakeFiles() *fakeFiles {
	return &fakeFiles{
		files: map[string]storage.File{
			"invoice.pdf": {Extension: ".pdf", MediaType: "application/pdf"},
			"scan.PDF":    {Extension: ".PDF", MediaType: "application/pdf"},
			"photo.png":   {Extension: ".png", MediaType: "image/png"},
			"sheet.xlsx":  {Extension: ".xlsx", MediaType: xlsxType},
		},
		lookups: map[string]int{},
	}
}

func (f *fakeFiles) fileOf(_ context.Context, value string) (storage.File, error) {
	f.lookups[value]++
	if value == "unreachable" {
		return storage.File{}, errors.New("storage is down")
	}
	file, ok := f.files[value]
	if !ok {
		return storage.File{}, fmt.Errorf("%w: %s", corestorage.ErrNotFound, value)
	}
	return file, nil
}

// filesForm has a file field for each way x-file is written, one without it,
// one in a list's rows, and one in a conditional part.
const filesForm = `{"schema": {
	"type": "object",
	"properties": {
		"invoice": {"type": "string", "format": "file", "x-file": {"accept": ".pdf"}},
		"photos": {"type": "array", "items": {"type": "string", "format": "file"}, "x-file": {"accept": "image/*", "maxFiles": 2}},
		"sheet": {"type": "string", "format": "file", "x-file": {"accept": "` + xlsxType + `, .csv"}},
		"anything": {"type": "string", "format": "file", "x-file": {"accept": "*/*"}},
		"unlimited": {"type": "string", "format": "file"},
		"rows": {"type": "array", "items": {"type": "object", "properties": {
			"document": {"type": "string", "format": "file", "x-file": {"accept": ".pdf"}}
		}}},
		"kind": {"type": "string"}
	},
	"if": {"properties": {"kind": {"const": "export"}}, "required": ["kind"]},
	"then": {"properties": {"permit": {"type": "string", "format": "file", "x-file": {"accept": ".pdf"}}}}
}}`

func TestExtension_Execute_XFile(t *testing.T) {
	notAcceptedPDF := `file type not accepted (accept: ".pdf")`
	tests := []struct {
		name    string
		payload string
		want    []Violation
	}{
		{name: "accepted files", payload: `{"invoice": "invoice.pdf", "photos": ["photo.png"], "sheet": "sheet.xlsx", "anything": "sheet.xlsx", "rows": [{"document": "invoice.pdf"}]}`},
		{name: "an extension in another case", payload: `{"invoice": "scan.PDF"}`},
		{name: "an extension not accepted", payload: `{"invoice": "sheet.xlsx"}`,
			want: []Violation{{"/invoice", notAcceptedPDF}}},
		{name: "a type outside a wildcard", payload: `{"photos": ["photo.png", "invoice.pdf"]}`,
			want: []Violation{{"/photos/1", `file type not accepted (accept: "image/*")`}}},
		{name: "a type not listed", payload: `{"sheet": "photo.png"}`,
			want: []Violation{{"/sheet", `file type not accepted (accept: "` + xlsxType + `, .csv")`}}},
		{name: "more files than allowed", payload: `{"photos": ["photo.png", "photo.png", "photo.png"]}`,
			want: []Violation{{"/photos", "more than 2 files"}}},
		{name: "a file never uploaded", payload: `{"invoice": "never-uploaded"}`,
			want: []Violation{{"/invoice", "file not found"}}},
		{name: "an empty value names no file", payload: `{"invoice": "", "photos": [""]}`,
			want: []Violation{{"/invoice", "empty value instead of a file"}, {"/photos/0", "empty value instead of a file"}}},
		{name: "a field without x-file is not checked", payload: `{"unlimited": "never-uploaded"}`},
		{name: "a file in a list's row", payload: `{"rows": [{"document": "invoice.pdf"}, {"document": "photo.png"}]}`,
			want: []Violation{{"/rows/1/document", notAcceptedPDF}}},
		{name: "a conditional part that applies", payload: `{"kind": "export", "permit": "photo.png"}`,
			want: []Violation{{"/permit", notAcceptedPDF}}},
		{name: "a conditional part that doesn't apply", payload: `{"kind": "import", "permit": "photo.png"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := newFakeFiles()
			ext := NewExtension(fakeForms{"files-form": filesForm}.form, namespace("form"), files.fileOf)
			err := ext.Execute(context.Background(), &store.TaskRecord{}, decode(t, tt.payload), naming("files-form"))

			if tt.want == nil {
				if err != nil {
					t.Fatalf("Execute() = %v; want nil", err)
				}
				return
			}
			var invalid *InvalidPayloadError
			if !errors.As(err, &invalid) {
				t.Fatalf("Execute() = %v; want an *InvalidPayloadError", err)
			}
			if !sameViolations(invalid.Violations, tt.want) {
				t.Errorf("violations = %v, want %v", invalid.Violations, tt.want)
			}
		})
	}
}

// A file storage can't look up isn't known to be missing, so the submission
// fails as a failed lookup, not as one that doesn't match its form.
func TestExtension_Execute_XFileLookupFails(t *testing.T) {
	files := newFakeFiles()
	ext := NewExtension(fakeForms{"files-form": filesForm}.form, namespace("form"), files.fileOf)
	err := ext.Execute(context.Background(), &store.TaskRecord{}, decode(t, `{"invoice": "sheet.xlsx", "photos": ["unreachable", "photo.png"]}`), naming("files-form"))
	if err == nil || errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("Execute() = %v; want a non-validation error", err)
	}
	if files.lookups["photo.png"] != 0 {
		t.Errorf("looked up a file after a failed lookup")
	}
}

func TestExtension_Execute_XFileLookups(t *testing.T) {
	files := newFakeFiles()
	ext := NewExtension(fakeForms{"files-form": filesForm}.form, namespace("form"), files.fileOf)

	// A draft is not held to the form's rules, so it looks no file up.
	if err := ext.Execute(WithDraft(context.Background()), &store.TaskRecord{}, decode(t, `{"invoice": "sheet.xlsx"}`), naming("files-form")); err != nil {
		t.Fatalf("Execute(draft) = %v; want nil", err)
	}
	if len(files.lookups) != 0 {
		t.Errorf("a draft looked up %v", files.lookups)
	}

	// A file named twice is looked up once.
	if err := ext.Execute(context.Background(), &store.TaskRecord{}, decode(t, `{"photos": ["photo.png", "photo.png"]}`), naming("files-form")); err != nil {
		t.Fatalf("Execute() = %v; want nil", err)
	}
	if files.lookups["photo.png"] != 1 {
		t.Errorf("photo.png looked up %d times, want 1", files.lookups["photo.png"])
	}
}

// Rules of the wrong type mean what they mean to the file control, and x-file
// on a field that isn't a file field is not read.
func TestExtension_Execute_XFileRulesOfTheWrongType(t *testing.T) {
	const form = `{"schema": {"type": "object", "properties": {
		"count_as_text": {"type": "array", "items": {"type": "string", "format": "file"}, "x-file": {"maxFiles": " 1 "}},
		"count_not_a_number": {"type": "array", "items": {"type": "string", "format": "file"}, "x-file": {"maxFiles": "ten"}},
		"accept_as_list": {"type": "string", "format": "file", "x-file": {"accept": [".pdf"]}},
		"note": {"type": "string", "x-file": {"accept": ".pdf"}}
	}}}`
	ext := NewExtension(fakeForms{"form": form}.form, namespace("form"), newFakeFiles().fileOf)
	payload := `{"count_as_text": ["photo.png", "photo.png"], "count_not_a_number": ["photo.png", "photo.png"], "accept_as_list": "invoice.pdf", "note": "photo.png"}`
	err := ext.Execute(context.Background(), &store.TaskRecord{}, decode(t, payload), naming("form"))

	want := []Violation{
		{"/accept_as_list", "file type not accepted (accept is not text)"},
		{"/count_as_text", "more than 1 file"},
	}
	var invalid *InvalidPayloadError
	if !errors.As(err, &invalid) {
		t.Fatalf("Execute() = %v; want an *InvalidPayloadError", err)
	}
	if !sameViolations(invalid.Violations, want) {
		t.Errorf("violations = %v, want %v", invalid.Violations, want)
	}
}

// sameViolations reports whether got and want hold the same violations, in any
// order: a form's properties are checked in no set order.
func sameViolations(got, want []Violation) bool {
	byPath := func(a, b Violation) int { return strings.Compare(a.Path+a.Message, b.Path+b.Message) }
	got, want = slices.Clone(got), slices.Clone(want)
	slices.SortFunc(got, byPath)
	slices.SortFunc(want, byPath)
	return slices.Equal(got, want)
}

func TestMaxFilesOf(t *testing.T) {
	tests := []struct {
		maxFiles any
		want     *int
	}{
		{nil, nil},
		{json.Number("10"), ptr(10)},
		{json.Number("2.5"), ptr(3)},
		{json.Number("-1"), ptr(0)},
		{"4", ptr(4)},
		{"", ptr(0)},
		{"ten", nil},
		{"Infinity", nil},
		{"NaN", nil},
		{"-Infinity", ptr(0)},
		{true, nil},
	}
	for _, tt := range tests {
		got := maxFilesOf(tt.maxFiles)
		if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
			t.Errorf("maxFilesOf(%#v) = %v, want %v", tt.maxFiles, deref(got), deref(tt.want))
		}
	}
}

func ptr(n int) *int { return &n }

func deref(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}

func TestAccepts(t *testing.T) {
	pdf := storage.File{Extension: ".pdf", MediaType: "application/pdf"}
	png := storage.File{Extension: ".png", MediaType: "image/png"}
	tests := []struct {
		accept string
		file   storage.File
		want   bool
	}{
		{"*", pdf, true},
		{"*/*", pdf, true},
		{"image/*", png, true},
		{"image/*", pdf, false},
		{".pdf", pdf, true},
		{".PDF", pdf, true},
		{".png", pdf, false},
		{"application/pdf", pdf, true},
		{"application/*", pdf, true},
		{"application/pdf", storage.File{Extension: ".pdf", MediaType: "application/pdf; version=2"}, false},
		{" .png , .pdf ", pdf, true},
		{"", pdf, false},
		{"pdf", pdf, false},
	}
	for _, tt := range tests {
		if got := accepts(tt.accept, tt.file); got != tt.want {
			t.Errorf("accepts(%q, %+v) = %v, want %v", tt.accept, tt.file, got, tt.want)
		}
	}
}
