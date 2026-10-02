package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/htmlgen"
	corestorage "github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTemplates serves templates from a map.
type fakeTemplates map[string]string

func (f fakeTemplates) HTMLTemplate(_ context.Context, id string) ([]byte, error) {
	src, ok := f[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return []byte(src), nil
}

// fakeSaver records what it was asked to store.
type fakeSaver struct {
	err      error
	calls    int
	filename string
	mime     string
	content  []byte
}

func (f *fakeSaver) Save(_ context.Context, filename, mime string, content []byte) (*corestorage.FileMetadata, error) {
	f.calls++
	f.filename, f.mime, f.content = filename, mime, content
	if f.err != nil {
		return nil, f.err
	}
	return &corestorage.FileMetadata{Key: "0f8e7c1a-0000-4000-8000-000000000001.html", Name: filename, MimeType: mime, Size: int64(len(content))}, nil
}

const permitTemplate = `<p>Permit No: {{ .refid.reference_id }}</p>` +
	`<p>Applicant: {{ .application.applicant_name }}</p>` +
	`<p>Fee: {{ decimal .application.fee_amount 2 }}</p>`

func htmlDocCtx(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context:         context.Background(),
		Inputs:          inputs,
		Record:          &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		OutputNamespace: "permit_doc",
	}
}

func TestHTMLDocumentGenerator_RendersStoresAndRecordsKey(t *testing.T) {
	files := &fakeSaver{}
	p := NewHTMLDocumentGeneratorPlugin(fakeTemplates{"permit": permitTemplate}, files)
	ctx := htmlDocCtx(map[string]any{
		"refid":       map[string]any{"reference_id": "PRM-2026-00555"},
		"application": map[string]any{"applicant_name": "Smith & Co", "fee_amount": json.Number("1400")},
	})

	require.NoError(t, p.Execute(ctx, json.RawMessage(`{"template_id": "permit", "filename": "permit.html"}`)))

	assert.Equal(t, `<p>Permit No: PRM-2026-00555</p><p>Applicant: Smith &amp; Co</p><p>Fee: 1400.00</p>`, string(files.content))
	assert.Equal(t, "permit.html", files.filename)
	assert.Equal(t, "text/html; charset=utf-8", files.mime)
	assert.Equal(t, map[string]any{
		"key":       "0f8e7c1a-0000-4000-8000-000000000001.html",
		"name":      "permit.html",
		"mime_type": "text/html; charset=utf-8",
		"size":      int64(len(files.content)),
	}, ctx.Record.Data["permit_doc"])
}

func TestHTMLDocumentGenerator_ReusesExistingDocument(t *testing.T) {
	files := &fakeSaver{}
	p := NewHTMLDocumentGeneratorPlugin(fakeTemplates{"permit": permitTemplate}, files)
	ctx := htmlDocCtx(nil)
	existing := map[string]any{"key": "already-stored.html"}
	ctx.Record.Data["permit_doc"] = existing

	require.NoError(t, p.Execute(ctx, json.RawMessage(`{"template_id": "permit"}`)))

	assert.Zero(t, files.calls, "a retried activity must not store the document again")
	assert.Equal(t, existing, ctx.Record.Data["permit_doc"])
}

func TestHTMLDocumentGenerator_Filename(t *testing.T) {
	cases := map[string]struct{ props, want string }{
		"defaults to the template id": {`{"template_id": "permit"}`, "permit.html"},
		"adds a missing extension":    {`{"template_id": "permit", "filename": "PRM-001"}`, "PRM-001.html"},
		"keeps an .html extension":    {`{"template_id": "permit", "filename": "PRM-001.HTML"}`, "PRM-001.HTML"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			files := &fakeSaver{}
			p := NewHTMLDocumentGeneratorPlugin(fakeTemplates{"permit": `<p>x</p>`}, files)
			require.NoError(t, p.Execute(htmlDocCtx(nil), json.RawMessage(tc.props)))
			assert.Equal(t, tc.want, files.filename)
		})
	}
}

func TestHTMLDocumentGenerator_Failures(t *testing.T) {
	storeErr := errors.New("bucket unavailable")
	cases := map[string]struct {
		props     string
		templates fakeTemplates
		saveErr   error
		namespace string
		wantIs    error
	}{
		"missing template_id":  {props: `{}`},
		"invalid config":       {props: `[]`},
		"unknown template":     {props: `{"template_id": "nope"}`},
		"missing namespace":    {props: `{"template_id": "permit"}`, namespace: "-"},
		"render failure":       {props: `{"template_id": "permit"}`, templates: fakeTemplates{"permit": `{{ .application }}`}, wantIs: htmlgen.ErrUnsupportedValue},
		"unsafe value refused": {props: `{"template_id": "permit"}`, templates: fakeTemplates{"permit": `<a href="{{ .link }}">x</a>`}, wantIs: htmlgen.ErrUnsafeValue},
		"storage failure":      {props: `{"template_id": "permit"}`, saveErr: storeErr, wantIs: storeErr},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			templates := tc.templates
			if templates == nil {
				templates = fakeTemplates{"permit": `<p>x</p>`}
			}
			files := &fakeSaver{err: tc.saveErr}
			ctx := htmlDocCtx(map[string]any{"application": map[string]any{"a": "b"}, "link": "javascript:alert(1)"})
			if tc.namespace == "-" {
				ctx.OutputNamespace = ""
			}

			err := NewHTMLDocumentGeneratorPlugin(templates, files).Execute(ctx, json.RawMessage(tc.props))

			require.Error(t, err)
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
			}
			assert.NotContains(t, ctx.Record.Data, "permit_doc", "nothing is recorded for a document that was not stored")
		})
	}
}

func TestHTMLTemplate_ParseValidates(t *testing.T) {
	var tmpl HTMLTemplate
	require.NoError(t, tmpl.Parse([]byte(permitTemplate)))
	assert.Equal(t, permitTemplate, string(tmpl.Source))

	assert.ErrorIs(t, new(HTMLTemplate).Parse([]byte(`<p>{{ .a </p>`)), htmlgen.ErrParseTemplate)
	assert.ErrorIs(t, new(HTMLTemplate).Parse([]byte(`<a href="{{ .u }}`)), htmlgen.ErrUnsafeTemplate)
}
