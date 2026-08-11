package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusWriter_RetainsFirstFinalStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &StatusWriter{ResponseWriter: rec}

	sw.WriteHeader(http.StatusContinue)
	sw.WriteHeader(http.StatusCreated)
	sw.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusCreated, sw.Status)
	// httptest.ResponseRecorder commits the first WriteHeader call (the 1xx).
	assert.Equal(t, http.StatusContinue, rec.Code)
}

func TestStatusWriter_IgnoresLaterFinalStatuses(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &StatusWriter{ResponseWriter: rec}

	sw.WriteHeader(http.StatusOK)
	sw.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusOK, sw.Status)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestStatusWriter_CaptureBodyRespectsLimit(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &StatusWriter{
		ResponseWriter: rec,
		CaptureBody:    true,
		MaxBodyBytes:   4,
	}

	_, _ = sw.Write([]byte("abcdef"))

	assert.Equal(t, []byte("abcd"), sw.Body)
	assert.Equal(t, "abcdef", rec.Body.String())
}

func TestStatusWriter_DoesNotCaptureBodyByDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &StatusWriter{ResponseWriter: rec}

	_, _ = sw.Write([]byte("payload"))

	assert.Empty(t, sw.Body)
}
