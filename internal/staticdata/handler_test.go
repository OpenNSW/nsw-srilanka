package staticdata

import (
	"encoding/json"
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

func TestHandler_HandleGet_Search(t *testing.T) {
	body := []byte(`{"data":[{"const":"1","title":"Port of Colombo"},{"const":"2","title":"Colombo"},{"const":"3","title":"Galle"}]}`)
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", body)
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?version=1.0.0&q=colombo&limit=1", nil)
	req.SetPathValue("id", "ports")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != cacheControlImmutable {
		t.Fatalf("expected immutable cache-control, got %s", cc)
	}
	var got SearchResult
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Total != 2 || got.Offset != 0 || got.Limit != 1 || len(got.Items) != 1 || got.Items[0].Title != "Colombo" {
		t.Fatalf("unexpected page: %+v", got)
	}
}

func TestHandler_HandleGet_SearchOffset(t *testing.T) {
	body := []byte(`{"data":[{"const":"1","title":"Port of Colombo"},{"const":"2","title":"Colombo"}]}`)
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", body)
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?version=1.0.0&q=colombo&offset=1&limit=20", nil)
	req.SetPathValue("id", "ports")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got SearchResult
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Total != 2 || got.Offset != 1 || len(got.Items) != 1 || got.Items[0].Title != "Port of Colombo" {
		t.Fatalf("unexpected page: %+v", got)
	}
}

func TestHandler_HandleGet_SearchNotFound(t *testing.T) {
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", []byte(`{"data":[]}`))
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/missing?version=1.0.0&q=colombo", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandler_HandleGet_SearchMissingVersion(t *testing.T) {
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", []byte(`{"data":[]}`))
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?q=colombo", nil)
	req.SetPathValue("id", "ports")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandler_HandleGet_InvalidLimit(t *testing.T) {
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", []byte(`{"data":[]}`))
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?version=1.0.0&limit=0", nil)
	req.SetPathValue("id", "ports")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
