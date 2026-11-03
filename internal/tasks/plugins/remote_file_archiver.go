package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNSW/core/remote"
)

// TaskTypeRemoteFileArchiver stores a copy of a document another service hands
// out by link, so the trader opens it from this deployment's storage.
const TaskTypeRemoteFileArchiver = "REMOTE_FILE_ARCHIVER"

// RemoteFetcher downloads from a registered service. *remote.Manager satisfies it,
// and refuses an absolute URL on any host but the service's own.
type RemoteFetcher interface {
	CallRaw(ctx context.Context, serviceID string, req remote.Request) (*remote.RawResponse, error)
}

// RemoteFileArchiverPlugin is a synchronous plugin that downloads the document
// at the URL in its source input and stores it:
//
//	{"task_type": "REMOTE_FILE_ARCHIVER", "output_namespace": "payment_slip_file",
//	 "plugin_properties": {"service_id": "slpa", "source_input": "payment_slip_url",
//	   "mime_type": "application/pdf", "filename": "slpa-payment-slip.pdf"}}
//
// A service like SLPA's CMS hands out its documents as signed links that expire,
// so a trader who opens one later finds nothing, and the document never reaches
// this deployment. This step downloads it while the link is good and stores it,
// and the render config links the stored copy.
//
// The URL is fetched through the service's own client, so the download goes only
// to that service's host, with its credentials.
//
// The stored file's metadata is written to <output_namespace> as {key, name,
// mime_type, size}, as HTML_DOCUMENT_GENERATOR writes it. Nothing is written when
// the source input is empty, so the page keeps showing what the service sent.
//
// A download the service refuses (a 4xx: an expired or unknown link) is recorded
// as {error} and the step goes on: the trader still has the service's own link,
// and a refusal would only repeat on a retry. A transport failure or a 5xx is
// returned, so the engine retries it.
type RemoteFileArchiverPlugin struct {
	remote RemoteFetcher
	files  DocumentSaver
}

// NewRemoteFileArchiverPlugin builds the plugin. Both dependencies must be non-nil.
func NewRemoteFileArchiverPlugin(fetcher RemoteFetcher, files DocumentSaver) *RemoteFileArchiverPlugin {
	if fetcher == nil {
		panic("fetcher is nil")
	}
	if files == nil {
		panic("files is nil")
	}
	return &RemoteFileArchiverPlugin{remote: fetcher, files: files}
}

type remoteFileArchiverConfig struct {
	// ServiceID is the registered service the source URL must be on.
	ServiceID string `json:"service_id"`
	// SourceInput is the input holding the document's URL.
	SourceInput string `json:"source_input"`
	// MimeType is the type the document is stored as, such as application/pdf.
	MimeType string `json:"mime_type"`
	// Filename names the stored document, extension included: storage takes
	// the key's extension from it.
	Filename string `json:"filename"`
}

func (c remoteFileArchiverConfig) validate() error {
	for _, f := range []struct{ name, value string }{
		{"service_id", c.ServiceID},
		{"source_input", c.SourceInput},
		{"mime_type", c.MimeType},
		{"filename", c.Filename},
	} {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("remote_file_archiver: plugin_properties.%s is required", f.name)
		}
	}
	return nil
}

// Execute stores the source document and returns nil (not ErrSuspended), so the
// engine advances immediately.
func (p *RemoteFileArchiverPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg remoteFileArchiverConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("remote_file_archiver: invalid config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	if ctx.OutputNamespace == "" {
		return errors.New("remote_file_archiver: output_namespace is required to hold the document's storage key")
	}
	if ctx.Record == nil {
		return errors.New("remote_file_archiver: task record is nil")
	}

	source, _ := ctx.Inputs[cfg.SourceInput].(string)
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}

	resp, err := p.remote.CallRaw(ctx.Context, cfg.ServiceID, remote.Request{Method: http.MethodGet, Path: source})
	if err != nil {
		return fmt.Errorf("remote_file_archiver: download from %s: %w", cfg.ServiceID, err)
	}
	switch {
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return p.refused(ctx, cfg, fmt.Sprintf("%s refused the download with status %d", cfg.ServiceID, resp.StatusCode))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("remote_file_archiver: download from %s: status %d", cfg.ServiceID, resp.StatusCode)
	case len(resp.Body) == 0:
		return p.refused(ctx, cfg, fmt.Sprintf("%s sent an empty document", cfg.ServiceID))
	}

	meta, err := p.files.Save(ctx.Context, cfg.Filename, cfg.MimeType, bytes.NewReader(resp.Body), int64(len(resp.Body)))
	if err != nil {
		return fmt.Errorf("remote_file_archiver: store %s: %w", cfg.Filename, err)
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

// refused records a document that cannot be stored and lets the step go on.
func (p *RemoteFileArchiverPlugin) refused(ctx pluginContext, cfg remoteFileArchiverConfig, reason string) error {
	slog.WarnContext(ctx.Context, "remote_file_archiver: document not stored",
		"taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "filename", cfg.Filename, "reason", reason)
	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{"error": reason}
	return nil
}
