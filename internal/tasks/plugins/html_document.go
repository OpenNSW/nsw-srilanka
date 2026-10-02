package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/htmlgen"
	corestorage "github.com/OpenNSW/core/storage"
)

// TaskTypeHTMLDocumentGenerator renders a print-ready HTML document from an
// html_template artifact and the task's inputs, and stores it.
const TaskTypeHTMLDocumentGenerator = "HTML_DOCUMENT_GENERATOR"

// HTMLTemplateKind is the artifact kind of an htmlgen template: the target
// HTML with html/template actions in it, stored as the file itself rather
// than wrapped in JSON.
const HTMLTemplateKind artifact.Kind = "html_template"

// htmlDocumentMIME is the type the document is stored and served as.
const htmlDocumentMIME = "text/html; charset=utf-8"

// HTMLTemplate is an html_template artifact. Parse validates it with htmlgen,
// so a broken template fails when it is loaded rather than part-way through a
// render.
type HTMLTemplate struct {
	Source []byte
}

// Kind identifies the artifact kind.
func (HTMLTemplate) Kind() artifact.Kind { return HTMLTemplateKind }

// Parse validates raw as an htmlgen template that calls no resolvers.
func (t *HTMLTemplate) Parse(raw []byte) error {
	if err := htmlgen.Validate(raw); err != nil {
		return err
	}
	t.Source = raw
	return nil
}

// HTMLTemplateSource returns an html_template's source by id.
type HTMLTemplateSource interface {
	HTMLTemplate(ctx context.Context, id string) ([]byte, error)
}

// RegistryHTMLTemplates serves html_template artifacts from the artifact
// registry, taking the latest version of each.
type RegistryHTMLTemplates struct {
	Registry *artifact.Registry
}

// HTMLTemplate loads the latest version of the template id.
func (r RegistryHTMLTemplates) HTMLTemplate(ctx context.Context, id string) ([]byte, error) {
	t, err := artifact.Latest[HTMLTemplate](ctx, r.Registry, id)
	if err != nil {
		return nil, err
	}
	return t.Source, nil
}

// DocumentSaver stores a document this service produced. The storage service
// satisfies it.
type DocumentSaver interface {
	Save(ctx context.Context, filename, mime string, content []byte) (*corestorage.FileMetadata, error)
}

// HTMLDocumentGeneratorPlugin is a synchronous plugin that renders the
// html_template named in the subtask template's plugin_properties against the
// task's inputs, stores the document, and records where it went:
//
//	{"task_type": "HTML_DOCUMENT_GENERATOR", "output_namespace": "permit_doc",
//	 "plugin_properties": {"template_id": "npqs_permit_html", "filename": "permit.html"}}
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

	// StartSubTask runs as a Temporal activity, so it can run again after the
	// record holding this document was saved. Storing it again would leave the
	// first copy orphaned under a key nothing refers to.
	if existing, ok := ctx.Record.Data[ctx.OutputNamespace].(map[string]any); ok {
		if key, ok := existing["key"].(string); ok && key != "" {
			slog.Info("html_document_generator: document already generated for this task; reusing it",
				"taskId", ctx.Record.TaskID, "templateId", cfg.TemplateID, "key", key)
			return nil
		}
	}

	tmpl, err := p.templates.HTMLTemplate(ctx.Context, cfg.TemplateID)
	if err != nil {
		return fmt.Errorf("html_document_generator: load template %q: %w", cfg.TemplateID, err)
	}

	// Inputs go to htmlgen as JSON so it decodes them with UseNumber: a number
	// that reached the inputs as json.Number keeps its exact text.
	data, err := json.Marshal(ctx.Inputs)
	if err != nil {
		return fmt.Errorf("html_document_generator: encode inputs: %w", err)
	}
	doc, err := htmlgen.Generate(ctx.Context, tmpl, data)
	if err != nil {
		return fmt.Errorf("html_document_generator: render template %q: %w", cfg.TemplateID, err)
	}

	meta, err := p.files.Save(ctx.Context, documentFilename(cfg), htmlDocumentMIME, doc)
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
