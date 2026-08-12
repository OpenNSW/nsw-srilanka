package storage

import (
	"encoding/json"
	"net/http"

	corestorage "github.com/OpenNSW/core/storage"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// AuditedHandler wraps the core storage HTTPHandler and records audit events
// for upload and download at the handler layer.
type AuditedHandler struct {
	inner *corestorage.HTTPHandler
	audit *nswaudit.Recorder
}

func NewAuditedHandler(inner *corestorage.HTTPHandler, recorder *nswaudit.Recorder) *AuditedHandler {
	return &AuditedHandler{inner: inner, audit: recorder}
}

func (h *AuditedHandler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sw := &nswaudit.StatusWriter{
		ResponseWriter: w,
		CaptureBody:    true,
	}
	h.inner.Upload(sw, r)

	event := nswaudit.Event{
		EventType:  nswaudit.EventStorage,
		Action:     nswaudit.ActionCreate,
		TargetType: nswaudit.TargetStorage,
		Failure:    nswaudit.HTTPStatus(sw.Status) >= http.StatusBadRequest,
		Metadata: map[string]any{
			"status": nswaudit.HTTPStatus(sw.Status),
		},
	}
	if nswaudit.HTTPStatus(sw.Status) == http.StatusOK && len(sw.Body) > 0 {
		var metadata corestorage.FileMetadata
		if err := json.Unmarshal(sw.Body, &metadata); err == nil && metadata.Key != "" {
			event.TargetID = metadata.Key
			event.Message = metadata
		}
	}
	h.audit.Record(ctx, event)
}

func (h *AuditedHandler) Download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.PathValue("key")

	sw := &nswaudit.StatusWriter{ResponseWriter: w}
	h.inner.Download(sw, r)

	h.audit.Record(ctx, nswaudit.Event{
		EventType:  nswaudit.EventStorage,
		Action:     nswaudit.ActionRead,
		TargetType: nswaudit.TargetStorage,
		TargetID:   key,
		Failure:    nswaudit.HTTPStatus(sw.Status) >= http.StatusBadRequest,
		Metadata: map[string]any{
			"status": nswaudit.HTTPStatus(sw.Status),
		},
	})
}

func (h *AuditedHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.inner.Delete(w, r)
}

func (h *AuditedHandler) UploadContentLocal(w http.ResponseWriter, r *http.Request) {
	h.inner.UploadContentLocal(w, r)
}

func (h *AuditedHandler) DownloadContent(w http.ResponseWriter, r *http.Request) {
	h.inner.DownloadContent(w, r)
}
