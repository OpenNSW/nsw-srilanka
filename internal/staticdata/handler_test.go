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
	if got.Total != 2 || got.Offset != 0 || got.Limit != 1 || len(got.Items) != 1 || values(t, got.Items, "title")[0] != "Colombo" {
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/names?version=1.0.0&q=&parent=Tea&limit=1", nil)
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
	if got.Total != 2 || len(got.Items) != 1 || values(t, got.Items, "const")[0] != "a" {
		t.Fatalf("expected parent to scope the page and total, got %+v", got)
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
	if got.Total != 2 || got.Offset != 1 || len(got.Items) != 1 || values(t, got.Items, "title")[0] != "Port of Colombo" {
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

func TestHandler_HandleGet_SearchKeys(t *testing.T) {
	body := []byte(`{"data":[{"foo":"A1","boo":"Alpha"},{"foo":"B2","boo":"Beta"}]}`)
	reg := newTestRegistry(t, "codes", "1", "refdata/codes/1.json", body)
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/codes?version=1&q=alp&searchKeys=boo,foo", nil)
	req.SetPathValue("id", "codes")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got SearchResult
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("expected one match, got %+v", got)
	}
	expectValues(t, values(t, got.Items, "foo"), []string{"A1"})
	expectValues(t, values(t, got.Items, "boo"), []string{"Alpha"})
}

func TestHandler_HandleGet_InvalidSearchKeys(t *testing.T) {
	reg := newTestRegistry(t, "codes", "1", "refdata/codes/1.json", []byte(`{"data":[]}`))
	h := NewHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-data/codes?version=1&q=a&searchKeys=boo,foo.bar", nil)
	req.SetPathValue("id", "codes")
	w := httptest.NewRecorder()
	h.HandleGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestParseSearchKeys(t *testing.T) {
	cases := []struct {
		param   string
		want    []string
		wantErr bool
	}{
		{param: "", want: nil},
		{param: "boo,foo", want: []string{"boo", "foo"}},
		{param: " boo , ,foo,", want: []string{"boo", "foo"}},
		{param: "title_2", want: []string{"title_2"}},
		{param: "foo.bar", wantErr: true},
		{param: "boo,{foo}", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.param, func(t *testing.T) {
			got, err := parseSearchKeys(tc.param)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			expectValues(t, got, tc.want)
		})
	}
}
