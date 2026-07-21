package asycuda

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// HandleCusdecIntegrationResult handles POST /webhooks/asycuda/cusdec/result
//
// This is the §5 callback pushed by ASYCUDA when CusDec integration succeeds or
// fails. The handler unmarshals and validates the payload, delegates to the
// service layer, and returns 202 Accepted.
func (h *HTTPHandler) HandleCusdecIntegrationResult(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer func() { _ = r.Body.Close() }()

	var req CusdecIntegrationResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "asycuda: failed to decode CusDec integration result payload", "error", err)
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		slog.WarnContext(r.Context(), "asycuda: CusDec integration result validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.cusdecService.ProcessIntegrationResult(r.Context(), req); err != nil {
		slog.ErrorContext(r.Context(), "asycuda: failed to process CusDec integration result",
			"edge_id", req.EdgeID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
