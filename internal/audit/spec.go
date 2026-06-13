package audit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Spec declares the audit semantics of a route.
type Spec struct {
	EventType        string
	TargetType       string
	TargetIDFromPath string // name of path value, e.g. "id" or "key"
	TargetIDFromResp string // name of JSON field in response body, e.g. "id"
}

// Wrap returns an HTTP middleware that logs the request/response to Argus based on the Spec.
func (r *Recorder) Wrap(spec Spec) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if r == nil || r.client == nil || !r.client.IsEnabled() {
				next.ServeHTTP(w, req)
				return
			}

			// Capture target ID from path parameters if specified
			var targetID string
			if spec.TargetIDFromPath != "" {
				targetID = req.PathValue(spec.TargetIDFromPath)
			}

			crw := &captureWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
				capture:        spec.TargetIDFromResp != "" && targetID == "",
			}

			next.ServeHTTP(crw, req)

			// If target ID was not in the path, try to fetch it from the response body JSON
			if targetID == "" && spec.TargetIDFromResp != "" {
				targetID = jsonField(crw.body.Bytes(), spec.TargetIDFromResp)
			}

			r.Record(req.Context(), Event{
				EventType:  spec.EventType,
				Action:     actionFromMethod(req.Method),
				TargetType: spec.TargetType,
				TargetID:   targetID,
				Failure:    crw.status >= 400,
				Metadata: map[string]any{
					"path":   req.URL.Path,
					"status": crw.status,
				},
			})
		})
	}
}

type captureWriter struct {
	http.ResponseWriter
	status  int
	body    bytes.Buffer
	capture bool
}

func (cw *captureWriter) WriteHeader(code int) {
	cw.status = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	if cw.capture {
		cw.body.Write(b)
	}
	return cw.ResponseWriter.Write(b)
}

func (cw *captureWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}

func jsonField(body []byte, field string) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if v, ok := m[field]; ok {
		switch val := v.(type) {
		case string:
			return val
		case float64:
			return strconv.FormatFloat(val, 'f', -1, 64)
		}
	}
	return ""
}

func actionFromMethod(method string) string {
	switch strings.ToUpper(method) {
	case "POST":
		return ActionCreate
	case "PUT", "PATCH":
		return ActionUpdate
	case "DELETE":
		return ActionDelete
	default:
		return ActionRead
	}
}
