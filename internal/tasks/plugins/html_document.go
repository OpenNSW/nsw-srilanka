package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/OpenNSW/core/htmlgen"
	corestorage "github.com/OpenNSW/core/storage"
)

// TaskTypeHTMLDocumentGenerator renders a print-ready HTML document from an
// html_template artifact and the task's inputs, and stores it.
const TaskTypeHTMLDocumentGenerator = "HTML_DOCUMENT_GENERATOR"

// htmlDocumentMIME is the type the document is stored and served as.
const htmlDocumentMIME = "text/html; charset=utf-8"

// HTMLTemplateSource returns an html_template's source by id.
type HTMLTemplateSource interface {
	HTMLTemplate(ctx context.Context, id string) ([]byte, error)
}

// DocumentSaver stores a document this service produced. The storage service
// satisfies it.
type DocumentSaver interface {
	Save(ctx context.Context, filename, mime string, body io.Reader, size int64) (*corestorage.FileMetadata, error)
}

// HTMLDocumentGeneratorPlugin is a synchronous plugin that renders the
// html_template named in the subtask template's plugin_properties against the
// task's inputs, stores the document, and records where it went:
//
//	{"task_type": "HTML_DOCUMENT_GENERATOR", "output_namespace": "permit_doc",
//	 "plugin_properties": {"template_id": "npqs-permit--html", "filename": "permit.html"}}
//
// The node's input_mapping assembles everything the document prints into the
// inputs — form submissions, reviewer decisions, a reference ID generated
// earlier — and the template addresses them by input name, e.g.
// {{ .application.applicant_name }}. The template only arranges and formats
// what it is given.
//
// The stored file's metadata is written to the task's own state at
// <output_namespace> as {key, name, mime_type, size}; the workflow reads the
// key from there through the node's output_mapping, and a client fetches the
// document through the storage API with it.
type HTMLDocumentGeneratorPlugin struct {
	templates HTMLTemplateSource
	files     DocumentSaver
}

// NewHTMLDocumentGeneratorPlugin builds the plugin. Both dependencies must be
// non-nil.
func NewHTMLDocumentGeneratorPlugin(templates HTMLTemplateSource, files DocumentSaver) *HTMLDocumentGeneratorPlugin {
	if templates == nil {
		panic("templates is nil")
	}
	if files == nil {
		panic("files is nil")
	}
	return &HTMLDocumentGeneratorPlugin{templates: templates, files: files}
}

type htmlDocumentGeneratorConfig struct {
	TemplateID string `json:"template_id"`
	// Filename names the stored document; it defaults to <template_id>.html.
	Filename string `json:"filename,omitempty"`
}

// Execute renders and stores the document and returns nil (not ErrSuspended),
// so the engine advances immediately.
func (p *HTMLDocumentGeneratorPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg htmlDocumentGeneratorConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("html_document_generator: invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.TemplateID) == "" {
		return errors.New("html_document_generator: plugin_properties.template_id is required")
	}
	if ctx.OutputNamespace == "" {
		return errors.New("html_document_generator: output_namespace is required to hold the document's storage key")
	}
	if ctx.Record == nil {
		return errors.New("html_document_generator: task record is nil")
	}

	tmpl, err := p.templates.HTMLTemplate(ctx.Context, cfg.TemplateID)
	if err != nil {
		return fmt.Errorf("html_document_generator: load template %q: %w", cfg.TemplateID, err)
	}

	// Inputs go to htmlgen as JSON, which it decodes with UseNumber, so a
	// number prints as plain digits rather than in exponent form. That keeps
	// only the text the number still has here: inputs arrive through Temporal
	// as float64, so 1.50 is already 1.5. A template formats money and
	// quantities with decimal.
	data, err := json.Marshal(ctx.Inputs)
	if err != nil {
		return fmt.Errorf("html_document_generator: encode inputs: %w", err)
	}
	doc, err := htmlgen.Generate(ctx.Context, tmpl, data)
	if err != nil {
		return fmt.Errorf("html_document_generator: render template %q: %w", cfg.TemplateID, err)
	}

	// Every run stores what it rendered, including a Temporal retry and a
	// workflow loop that brings the step back. A loop may arrive with changed
	// inputs, and reusing an earlier document would leave the task pointing at
	// one rendered from the old data. A retry leaves at worst an extra copy in
	// storage that nothing refers to.
	meta, err := p.files.Save(ctx.Context, documentFilename(cfg), htmlDocumentMIME, bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		return fmt.Errorf("html_document_generator: store document: %w", err)
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{
		"key":       meta.Key,
		"name":      meta.Name,
		"mime_type": meta.MimeType,
		"size":      meta.Size,
	}
	return nil
}

// documentFilename is the stored document's name. Storage takes the key's
// extension from it, so it always ends in .html.
func documentFilename(cfg htmlDocumentGeneratorConfig) string {
	name := strings.TrimSpace(cfg.Filename)
	if name == "" {
		name = cfg.TemplateID
	}
	if !strings.EqualFold(filepath.Ext(name), ".html") {
		name += ".html"
	}
	return name
}
