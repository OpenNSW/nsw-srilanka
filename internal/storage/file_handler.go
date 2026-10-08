package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/OpenNSW/core/httputil"
	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

// FileRefs issues the file references callers hold in place of stored values,
// and resolves them back. *fileaccess.Access satisfies it, and its errors are
// the ones FileHandler maps.
type FileRefs interface {
	IssueFor(p *authn.Principal, value string) (string, error)
	Resolve(p *authn.Principal, ref string) (string, error)
}

// FileHandler serves the storage routes, in either mode: an upload returns a
// file reference issued to the caller, and a download resolves one for the
// caller before asking Service for the file. A stored value is never returned
// to the caller, though a presigned upload URL may name it: it grants nothing
// on its own.
//
// Authentication is the route middleware's job; the handler only reads the
// principal it attached.
type FileHandler struct {
	svc  Service
	refs FileRefs
	// proxied is set when svc is a ProxyService, whose failures other than
	// the rejections it relays are the owning service's: a gateway error,
	// not this service's.
	proxied bool
}

// NewFileHandler creates a FileHandler over svc, resolving references with
// refs.
func NewFileHandler(svc Service, refs FileRefs) *FileHandler {
	_, proxied := svc.(*ProxyService)
	return &FileHandler{svc: svc, refs: refs, proxied: proxied}
}

// uploadResponse is what an upload returns: the reference to keep for the
// file, and where to upload it.
type uploadResponse struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	UploadURL string `json:"upload_url"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
}

// Upload reserves a stored file and returns a reference to it for the caller,
// with the URL to upload the file to. A user's reference expires; a machine
// client's, such as an agency proxying its own upload, does not.
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, "authentication required")
		return
	}
	var req struct {
		Filename string `json:"filename"`
		MimeType string `json:"mime_type"`
		Size     int64  `json:"size"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Filename == "" {
		httputil.Error(w, r, http.StatusBadRequest, "filename is required")
		return
	}
	if req.MimeType == "" {
		httputil.Error(w, r, http.StatusBadRequest, "mime_type is required")
		return
	}

	meta, err := h.svc.Upload(r.Context(), req.Filename, req.Size, req.MimeType)
	if err != nil {
		h.writeServiceError(w, r, err, "failed to prepare upload")
		return
	}
	ref, err := h.refs.IssueFor(p, meta.Key)
	if err != nil {
		httputil.InternalServerError(w, r, "storage: failed to issue a file reference", err)
		return
	}
	httputil.JSON(w, http.StatusOK, uploadResponse{
		Key:       ref,
		Name:      meta.Name,
		UploadURL: meta.UploadURL,
		Size:      meta.Size,
		MimeType:  meta.MimeType,
	})
}

// Download returns a time-limited URL to download the file the reference in
// the path stands for, and when it expires.
func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("key")
	if ref == "" {
		httputil.Error(w, r, http.StatusBadRequest, "key is required")
		return
	}
	p, ok := authn.FromContext(r.Context())
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, "authentication required")
		return
	}

	value, err := h.refs.Resolve(p, ref)
	switch {
	case err == nil:
	case errors.Is(err, fileaccess.ErrInvalid), errors.Is(err, fileaccess.ErrNotYours):
		// One answer for both, so the route says nothing about a value it
		// will not serve. A raw storage key is one: it grants nothing.
		httputil.Error(w, r, http.StatusForbidden, "this is not a file reference issued to you")
		return
	case errors.Is(err, fileaccess.ErrExpired):
		httputil.Error(w, r, http.StatusGone, "this file reference has expired; reload to get a new one")
		return
	default:
		httputil.InternalServerError(w, r, "storage: failed to resolve a file reference", err)
		return
	}

	downloadURL, expiresAt, err := h.svc.DownloadURL(r.Context(), value)
	if err != nil {
		h.writeServiceError(w, r, err, "failed to generate access")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"download_url": downloadURL,
		"expires_at":   expiresAt,
	})
}

// writeServiceError answers a failed Service call the way core/storage's and
// the proxy's own handlers do.
func (h *FileHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	var tooLarge *corestorage.FileTooLargeError
	switch {
	case errors.Is(err, corestorage.ErrInvalidKey):
		httputil.Error(w, r, http.StatusBadRequest, "invalid key format")
	case errors.Is(err, corestorage.ErrInvalidSize):
		httputil.Error(w, r, http.StatusBadRequest, "size must be greater than 0")
	case errors.As(err, &tooLarge):
		httputil.Error(w, r, http.StatusBadRequest, fmt.Sprintf("file size exceeds %s limit", formatSize(tooLarge.Limit)))
	case errors.Is(err, corestorage.ErrContentTypeNotAllowed):
		httputil.Error(w, r, http.StatusUnsupportedMediaType, "invalid or prohibited file type")
	case h.proxied:
		writeProxyError(w, r, err, fallback)
	default:
		httputil.InternalServerError(w, r, "storage: "+fallback, err)
	}
}

// formatSize writes a byte count the way core/storage's messages do.
func formatSize(n int64) string {
	if n%(1<<20) == 0 {
		return fmt.Sprintf("%dMB", n>>20)
	}
	return fmt.Sprintf("%d bytes", n)
}
