package staticdata

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestRequest(id, version string) *http.Request {
	target := "/api/v1/static-data/" + id
	if version != "" {
		target += "?version=" + version
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("id", id)
	return req
}

func TestHandler_HandleGet_Success(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":["0101"]}`))
	h := NewHandler(reg)

	w := httptest.NewRecorder()
	h.HandleGet(w, newTestRequest("hs-codes", "1.0.0"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != cacheControlImmutable {
		t.Fatalf("expected immutable cache-control, got %s", cc)
	}
	if w.Body.String() != `{"data":["0101"]}` {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestHandler_HandleGet_MissingVersion(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":[]}`))
	h := NewHandler(reg)

	w := httptest.NewRecorder()
	h.HandleGet(w, newTestRequest("hs-codes", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandler_HandleGet_NotFound(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":[]}`))
	h := NewHandler(reg)

	w := httptest.NewRecorder()
	h.HandleGet(w, newTestRequest("missing", "1.0.0"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
