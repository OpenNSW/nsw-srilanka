package storage

import (
	"errors"
	"net/http"

	"github.com/OpenNSW/core/httputil"
	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

// FileRefs resolves the file references callers hold to the stored values
// they stand for. *fileaccess.Access satisfies it, and its errors are the
// ones FileHandler maps.
type FileRefs interface {
	Resolve(p *authn.Principal, ref string) (string, error)
}

// FileHandler serves the storage routes that take a file reference, in either
// mode: it resolves the reference for the caller, then asks Service for the
// file. A stored value never reaches the caller.
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
	case errors.Is(err, fileaccess.ErrInvalid):
		// Until every caller holds file tokens, a reference that is not one
		// is taken to be the stored value itself.
		value = ref
	case errors.Is(err, fileaccess.ErrNotYours):
		httputil.Error(w, r, http.StatusForbidden, "this file reference was issued to someone else")
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
	switch {
	case errors.Is(err, corestorage.ErrInvalidKey):
		httputil.Error(w, r, http.StatusBadRequest, "invalid key format")
	case h.proxied:
		writeProxyError(w, r, err, fallback)
	default:
		httputil.InternalServerError(w, r, "storage: "+fallback, err)
	}
}
