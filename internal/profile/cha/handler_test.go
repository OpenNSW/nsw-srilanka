package cha

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubService is a minimal Service stub for handler tests.
type stubService struct {
	listRecords []Record
	listErr     error
}

func (s *stubService) GetByID(_ context.Context, _ string) (*Record, error)    { return nil, nil }
func (s *stubService) GetByEmail(_ context.Context, _ string) (*Record, error) { return nil, nil }
func (s *stubService) List(_ context.Context) ([]Record, error)                { return s.listRecords, s.listErr }
func (s *stubService) Health(_ context.Context) error                          { return nil }

func TestHandler_HandleGetCHAs_Success(t *testing.T) {
	records := []Record{
		{ID: "cha-1", Name: "Advantis", Email: "a@example.com"},
		{ID: "cha-2", Name: "Yusen", Email: "y@example.com"},
	}
	h := NewHandler(&stubService{listRecords: records})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chas", nil)
	w := httptest.NewRecorder()
	h.HandleGetCHAs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	var got []Record
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[0].ID != "cha-1" || got[1].ID != "cha-2" {
		t.Fatalf("unexpected records: %+v", got)
	}
}

func TestHandler_HandleGetCHAs_EmptyList(t *testing.T) {
	h := NewHandler(&stubService{listRecords: []Record{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chas", nil)
	w := httptest.NewRecorder()
	h.HandleGetCHAs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got []Record
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d records", len(got))
	}
}

func TestHandler_HandleGetCHAs_ServiceError(t *testing.T) {
	h := NewHandler(&stubService{listErr: errors.New("database down")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chas", nil)
	w := httptest.NewRecorder()
	h.HandleGetCHAs(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

type errorWriter struct{ header http.Header }

func (e *errorWriter) Header() http.Header       { return e.header }
func (e *errorWriter) Write([]byte) (int, error) { return 0, errors.New("write error") }
func (e *errorWriter) WriteHeader(int)           {}

func TestHandler_HandleGetCHAs_EncodeError(t *testing.T) {
	h := NewHandler(&stubService{listRecords: []Record{{ID: "cha-1"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chas", nil)
	h.HandleGetCHAs(&errorWriter{header: http.Header{}}, req)
}
