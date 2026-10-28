package payloadschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	corestorage "github.com/OpenNSW/core/storage"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/message"

	"github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// FileOf describes the file stored under value, a file field's stored value.
// A value no file is stored under is an error wrapping corestorage.ErrNotFound.
type FileOf func(ctx context.Context, value string) (storage.File, error)

// storedFiles looks up the files a submission names for its x-file rules, each
// once. Validation can't fail on its own, so the first lookup that fails other
// than with a missing file is kept in err, and no file is looked up after it.
type storedFiles struct {
	ctx    context.Context
	fileOf FileOf
	seen   map[string]*storage.File
	err    error
}

func newStoredFiles(ctx context.Context, fileOf FileOf) *storedFiles {
	return &storedFiles{ctx: ctx, fileOf: fileOf, seen: map[string]*storage.File{}}
}

// describe returns the file stored under value, nil when there is none. ok is
// false once a lookup has failed.
func (s *storedFiles) describe(value string) (file *storage.File, ok bool) {
	if s.err != nil {
		return nil, false
	}
	if f, seen := s.seen[value]; seen {
		return f, true
	}
	f, err := s.fileOf(s.ctx, value)
	switch {
	case err == nil:
		file = &f
	case !errors.Is(err, corestorage.ErrNotFound):
		s.err = err
		return nil, false
	}
	s.seen[value] = file
	return file, true
}

// vocabulary returns the forms' x-file keyword as the trader app's file
// control reads it: on a file field, one file ({"type": "string", "format":
// "file"}) or a list of them ({"type": "array", "items": {"type": "string",
// "format": "file"}}), as {"x-file": {"accept": ".pdf,image/*", "maxFiles": 10}}.
// Each file must be stored and match accept, and a list may hold at most
// maxFiles. maxSize is left to the browser; storage holds every file to the
// deployment's upload limit. A rule the field doesn't set isn't checked: the
// deployment's upload limits apply to it, and the file control's defaults
// and uiSchema options apply in the browser only.
func (s *storedFiles) vocabulary() *jsonschema.Vocabulary {
	return &jsonschema.Vocabulary{
		URL:     "https://github.com/OpenNSW/ui-packages#x-file",
		Compile: s.compileXFile,
	}
}

// compileXFile returns the x-file rules obj, a schema, sets on a file field,
// and none for any other schema. A rule of the wrong type means what it means
// to the file control, in JavaScript: an accept that isn't text accepts no
// file, as matching one against it throws, and a maxFiles given as text is
// the number it reads as, or no limit when it reads as none.
func (s *storedFiles) compileXFile(_ *jsonschema.CompilerContext, obj map[string]any) (jsonschema.SchemaExt, error) {
	opts, ok := obj["x-file"].(map[string]any)
	if !ok || !isFileField(obj) {
		return nil, nil
	}
	rules := &xFile{files: s}
	switch accept := opts["accept"].(type) {
	case nil:
	case string:
		rules.accept = &accept
	default:
		rules.acceptsNone = true
	}
	rules.maxFiles = maxFilesOf(opts["maxFiles"])
	return rules, nil
}

// maxFilesOf returns the most files a list may hold under maxFiles, as the
// file control lets them be added: one at a time while the list holds fewer
// than maxFiles. It is nil for no limit: a maxFiles that is absent, or that
// JavaScript reads as no number.
func maxFilesOf(maxFiles any) *int {
	var n float64
	switch v := maxFiles.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil
		}
		n = f
	case string:
		// JavaScript reads blank text as 0.
		if t := strings.TrimSpace(v); t != "" {
			f, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return nil
			}
			n = f
		}
	default:
		return nil
	}
	// No length reaches Infinity, and none compares with NaN.
	if math.IsInf(n, 1) || math.IsNaN(n) {
		return nil
	}
	most := int(math.Max(math.Ceil(n), 0))
	return &most
}

// isFileField reports whether s, a schema, is one the trader app renders with
// its file control.
func isFileField(s map[string]any) bool {
	isFile := func(s map[string]any) bool { return s["type"] == "string" && s["format"] == "file" }
	if isFile(s) {
		return true
	}
	items, _ := s["items"].(map[string]any)
	return s["type"] == "array" && items != nil && isFile(items)
}

// xFile is the x-file rules on one file field.
type xFile struct {
	files       *storedFiles
	accept      *string
	acceptsNone bool
	maxFiles    *int
}

// Validate checks the files of a field: one file, or a list of them.
func (x *xFile) Validate(ctx *jsonschema.ValidatorContext, v any) {
	switch v := v.(type) {
	case string:
		if kind := x.check(v); kind != nil {
			ctx.AddError(kind)
		}
	case []any:
		if x.maxFiles != nil && len(v) > *x.maxFiles {
			files := "files"
			if *x.maxFiles == 1 {
				files = "file"
			}
			ctx.AddError(&xFileKind{keyword: "maxFiles", message: fmt.Sprintf("more than %d %s", *x.maxFiles, files)})
		}
		for i, el := range v {
			value, ok := el.(string)
			if !ok {
				continue
			}
			if kind := x.check(value); kind != nil {
				ctx.AddErr(&jsonschema.ValidationError{
					InstanceLocation: append(slices.Clone(ctx.ValueLocation()), strconv.Itoa(i)),
					ErrorKind:        kind,
				})
			}
		}
	}
}

// check returns how the file stored under value breaks the field's rules, or
// nil. An empty value names no file, so it is refused, apart from a file that
// isn't stored: the file control drops a field it holds no file for rather
// than sending one. Swapping file references for stored values passes an
// empty value through, so this is where it is refused.
func (x *xFile) check(value string) *xFileKind {
	if value == "" {
		return &xFileKind{keyword: "empty", message: "empty value instead of a file"}
	}
	file, ok := x.files.describe(value)
	switch {
	case !ok:
		return nil
	case file == nil:
		return &xFileKind{keyword: "stored", message: "file not found"}
	case x.acceptsNone:
		return &xFileKind{keyword: "accept", message: "file type not accepted (accept is not text)"}
	case x.accept != nil && !accepts(*x.accept, *file):
		return &xFileKind{keyword: "accept", message: "file type not accepted (accept: " + strconv.Quote(*x.accept) + ")"}
	}
	return nil
}

// accepts ports the trader app's matchesAccept: accept is a comma-separated
// list, of which file must match one entry. "*" and "*/*" match any file,
// "<type>/*" a type that starts with "<type>/", ".<ext>" the file's extension
// whatever its case, and any other entry the type exactly. The extension is
// the stored one, which storage matched to the type on upload.
func accepts(accept string, file storage.File) bool {
	for _, entry := range strings.Split(accept, ",") {
		entry = strings.TrimSpace(entry)
		switch {
		case entry == "":
		case entry == "*" || entry == "*/*":
			return true
		case strings.HasSuffix(entry, "/*"):
			if strings.HasPrefix(file.MediaType, strings.TrimSuffix(entry, "*")) {
				return true
			}
		case strings.HasPrefix(entry, "."):
			if strings.EqualFold(file.Extension, entry) {
				return true
			}
		case file.MediaType == entry:
			return true
		}
	}
	return false
}

// xFileKind is a file field that breaks one of its x-file rules.
type xFileKind struct {
	keyword, message string
}

func (k *xFileKind) KeywordPath() []string { return []string{"x-file", k.keyword} }

func (k *xFileKind) LocalizedString(*message.Printer) string { return k.message }
