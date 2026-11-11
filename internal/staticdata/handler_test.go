package staticdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
)

func newTestRequest(id, manifestVersion string) *http.Request {
	target := "/api/v1/static-data/" + id
	if manifestVersion != "" {
		target += "?manifestVersion=" + manifestVersion
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("id", id)
	return req
}

func TestHandler_HandleGet_Success(t *testing.T) {
	reg := newTestRegistry(t, "hs-codes", "1.0.0", "refdata/hs-codes/1.0.0.json", []byte(`{"data":["0101"]}`))
	h := NewHandler(reg)

	w := httptest.NewRecorder()
	h.HandleGet(w, newTestRequest("hs-codes", "1"))

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
	h.HandleGet(w, newTestRequest("missing", "1"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandler_HandleGet_Search(t *testing.T) {
	body := []byte(`{"data":[{"const":"1","title":"Port of Colombo"},{"const":"2","title":"Colombo"},{"const":"3","title":"Galle"}]}`)
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", body)
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?manifestVersion=1&q=colombo&limit=1", nil)
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

func TestHandler_HandleGet_SearchParent(t *testing.T) {
	body := []byte(`{"data":[
		{"const":"a","title":"Tea Plant","parents":["Tea"]},
		{"const":"b","title":"Tea Bush","parents":["Tea"]},
		{"const":"c","title":"Coconut","parents":["Palm"]}
	]}`)
	reg := newTestRegistry(t, "names", "1.0.0", "refdata/names/1.0.0.json", body)
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/names?manifestVersion=1&q=&parent=Tea&limit=1", nil)
	req.SetPathValue("id", "names")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got SearchResult
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Total != 2 || len(got.Items) != 1 || got.Items[0].Const != "a" {
		t.Fatalf("expected parent to scope the page and total, got %+v", got)
	}
}

func TestHandler_HandleGet_SearchOffset(t *testing.T) {
	body := []byte(`{"data":[{"const":"1","title":"Port of Colombo"},{"const":"2","title":"Colombo"}]}`)
	reg := newTestRegistry(t, "ports", "1.0.0", "refdata/ports/1.0.0.json", body)
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?manifestVersion=1&q=colombo&offset=1&limit=20", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/missing?manifestVersion=1&q=colombo", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/ports?manifestVersion=1&limit=0", nil)
	req.SetPathValue("id", "ports")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func serve(t *testing.T, h *Handler, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/"+id+"?"+query, nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h.HandleGet(w, req)
	return w
}

func TestHandler_HandleGet_ManifestVersion(t *testing.T) {
	h := NewHandler(newTwoManifestSet(t))

	for query, want := range map[string]string{"manifestVersion=1": `{"data":["CMB"]}`, "manifestVersion=2": `{"data":["CMB","HRI"]}`} {
		w := serve(t, h, "ports", query)
		if w.Code != http.StatusOK || w.Body.String() != want {
			t.Errorf("%s: got %d %s; want 200 %s", query, w.Code, w.Body.String(), want)
		}
		if cc := w.Header().Get("Cache-Control"); cc != cacheControlImmutable {
			t.Errorf("%s: expected immutable cache-control, got %s", query, cc)
		}
	}
}

func TestHandler_HandleGet_ManifestVersionSearch(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"ports_1.json":     []byte(`{"data":[{"const":"CMB","title":"Colombo"}]}`),
		"ports_2.json":     []byte(`{"data":[{"const":"CMB","title":"Colombo"},{"const":"HRI","title":"Hambantota"}]}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","path":"ports_1.json"}]}`),
		"manifest_v2.json": []byte(`{"artifacts":[{"id":"ports","kind":"static_data","path":"ports_2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(set)

	for query, want := range map[string]int64{"manifestVersion=1&q=hamb": 0, "manifestVersion=2&q=hamb": 1} {
		w := serve(t, h, "ports", query)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", query, w.Code, w.Body.String())
		}
		var got SearchResult
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("%s: failed to decode response: %v", query, err)
		}
		if got.Total != want {
			t.Errorf("%s: expected %d matches, got %+v", query, want, got)
		}
	}
}

func TestHandler_HandleGet_ManifestVersionErrors(t *testing.T) {
	h := NewHandler(newTwoManifestSet(t))
	cases := map[string]int{
		"manifestVersion=3":             http.StatusNotFound,
		"manifestVersion=0":             http.StatusBadRequest,
		"manifestVersion=-1":            http.StatusBadRequest,
		"manifestVersion=abc":           http.StatusBadRequest,
		"manifestVersion=":              http.StatusBadRequest,
		"manifestVersion=3&q=cmb":       http.StatusNotFound,
		"manifestVersion=abc&q=cmb":     http.StatusBadRequest,
		"manifestVersion=1&limit=0&q=c": http.StatusBadRequest,
	}
	for query, want := range cases {
		if w := serve(t, h, "ports", query); w.Code != want {
			t.Errorf("%s: expected %d, got %d", query, want, w.Code)
		}
	}
	if w := serve(t, h, "missing", "manifestVersion=1"); w.Code != http.StatusNotFound {
		t.Errorf("unknown id: expected 404, got %d", w.Code)
	}
}
