package plugins

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/OpenNSW/core/remote"
)

// TaskTypeRemoteFileArchiver stores a copy of a document another service hands
// out by link, so the trader opens it from this deployment's storage.
const TaskTypeRemoteFileArchiver = "REMOTE_FILE_ARCHIVER"

// archiveSourceInput is the input holding the document to store: a URL on the
// configured service, or a data: URI the service sent the document inline as.
const archiveSourceInput = "source"

// defaultArchiveMIME is the type a document is stored as when its source names none.
const defaultArchiveMIME = "application/octet-stream"

// RemoteFetcher downloads from a registered service. *remote.Manager satisfies it,
// and refuses an absolute URL on any host but the service's own.
type RemoteFetcher interface {
	CallRaw(ctx context.Context, serviceID string, req remote.Request) (*remote.RawResponse, error)
}

// RemoteFileArchiverPlugin is a synchronous plugin that stores the document its
// "source" input points at:
//
//	{"task_type": "REMOTE_FILE_ARCHIVER", "output_namespace": "gate_pass_file",
//	 "plugin_properties": {"service_id": "slpa", "filename": "gate-pass"}}
//
// A service like SLPA's CMS hands out its documents as signed links that expire,
// so a trader who opens one later finds nothing, and the document never reaches
// this deployment. This step downloads it while the link is good and stores it,
// and the render config links the stored copy.
//
// The source is fetched through the service's own client, so the download goes
// only to that service's host, with its credentials. A data: URI is decoded
// instead, since the document is already in hand.
//
// The stored file's metadata is written to <output_namespace> as {key, name,
// mime_type, size}, as HTML_DOCUMENT_GENERATOR writes it. Nothing is written when
// there is no source, or the source is neither a URL nor a data: URI (a barcode
// sent as a plain code, say), so the page keeps showing what the service sent.
//
// A source the service refuses (a 4xx: an expired or unknown link) is recorded as
// {error} and the step goes on: the trader still has the service's own link, and
// a refusal would only repeat on a retry. A transport failure or a 5xx is
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
	// Filename names the stored document. Without an extension, the stored
	// type's is added.
	Filename string `json:"filename"`
}

// Execute stores the source document and returns nil (not ErrSuspended), so the
// engine advances immediately.
func (p *RemoteFileArchiverPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg remoteFileArchiverConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("remote_file_archiver: invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ServiceID) == "" {
		return errors.New("remote_file_archiver: plugin_properties.service_id is required")
	}
	if strings.TrimSpace(cfg.Filename) == "" {
		return errors.New("remote_file_archiver: plugin_properties.filename is required")
	}
	if ctx.OutputNamespace == "" {
		return errors.New("remote_file_archiver: output_namespace is required to hold the document's storage key")
	}
	if ctx.Record == nil {
		return errors.New("remote_file_archiver: task record is nil")
	}

	source, _ := ctx.Inputs[archiveSourceInput].(string)
	source = strings.TrimSpace(source)

	var (
		content  []byte
		mimeType string
		err      error
	)
	switch {
	case strings.HasPrefix(source, "data:"):
		content, mimeType, err = decodeDataURI(source)
		if err != nil {
			return p.refused(ctx, cfg, err)
		}
	case strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "http://"):
		content, mimeType, err = p.download(ctx.Context, cfg.ServiceID, source)
		var refusal sourceRefusedError
		if errors.As(err, &refusal) {
			return p.refused(ctx, cfg, err)
		}
		if err != nil {
			return err
		}
	default:
		// No source, or one that names no document: nothing to store.
		return nil
	}

	name := archiveFilename(cfg.Filename, mimeType)
	meta, err := p.files.Save(ctx.Context, name, mimeType, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return fmt.Errorf("remote_file_archiver: store %s: %w", name, err)
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

// sourceRefusedError is a source no retry can fetch: a 4xx from the service, or a
// data: URI that does not decode.
type sourceRefusedError struct{ msg string }

func (e sourceRefusedError) Error() string { return e.msg }

// download fetches source from serviceID. A 4xx is a sourceRefusedError; a
// transport failure or another non-2xx status is an error to retry.
func (p *RemoteFileArchiverPlugin) download(ctx context.Context, serviceID, source string) ([]byte, string, error) {
	resp, err := p.remote.CallRaw(ctx, serviceID, remote.Request{Method: http.MethodGet, Path: source})
	if err != nil {
		return nil, "", fmt.Errorf("remote_file_archiver: download from %s: %w", serviceID, err)
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return nil, "", sourceRefusedError{fmt.Sprintf("%s refused the download with status %d", serviceID, resp.StatusCode)}
	default:
		return nil, "", fmt.Errorf("remote_file_archiver: download from %s: status %d", serviceID, resp.StatusCode)
	}
	if len(resp.Body) == 0 {
		return nil, "", sourceRefusedError{fmt.Sprintf("%s sent an empty document", serviceID)}
	}
	return resp.Body, contentType(resp.Header.Get("Content-Type")), nil
}

// refused records a source that cannot be stored and lets the step go on.
func (p *RemoteFileArchiverPlugin) refused(ctx pluginContext, cfg remoteFileArchiverConfig, cause error) error {
	slog.WarnContext(ctx.Context, "remote_file_archiver: document not stored",
		"taskId", ctx.Record.TaskID, "serviceId", cfg.ServiceID, "filename", cfg.Filename, "error", cause)
	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{"error": cause.Error()}
	return nil
}

// decodeDataURI returns a base64 data: URI's content and type.
func decodeDataURI(uri string) ([]byte, string, error) {
	header, payload, ok := strings.Cut(strings.TrimPrefix(uri, "data:"), ",")
	if !ok {
		return nil, "", sourceRefusedError{"the data: URI has no data"}
	}
	params := strings.Split(header, ";")
	if params[len(params)-1] != "base64" {
		return nil, "", sourceRefusedError{"the data: URI is not base64"}
	}
	content, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", sourceRefusedError{fmt.Sprintf("the data: URI does not decode: %v", err)}
	}
	if len(content) == 0 {
		return nil, "", sourceRefusedError{"the data: URI is empty"}
	}
	return content, contentType(strings.Join(params[:len(params)-1], ";")), nil
}

// contentType is the media type a document is stored as, without parameters a
// file needs none of; defaultArchiveMIME when the source names none.
func contentType(header string) string {
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil || mediaType == "" {
		return defaultArchiveMIME
	}
	return mediaType
}

// archiveFilename is the stored document's name. Storage takes the key's
// extension from it, so a name without one gets the stored type's.
func archiveFilename(filename, mimeType string) string {
	name := strings.TrimSpace(filename)
	if filepath.Ext(name) != "" {
		return name
	}
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		return name + preferredExt(mimeType, exts)
	}
	return name
}

// preferredExt picks the usual extension for a type: mime lists several for some
// (".jpe", ".jpeg", ".jpg"), in an order that depends on the platform's tables.
func preferredExt(mimeType string, exts []string) string {
	switch mimeType {
	case "application/pdf":
		return ".pdf"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	}
	return exts[0]
}
