package audit

import "net/http"

const defaultCaptureBodyLimit = 4096

// StatusWriter captures the HTTP status code written by a handler. Body capture is
// opt-in via CaptureBody because retaining full response bodies on download paths
// can exhaust memory.
type StatusWriter struct {
	http.ResponseWriter
	Status       int
	Body         []byte
	CaptureBody  bool
	MaxBodyBytes int
}

func (w *StatusWriter) WriteHeader(statusCode int) {
	if statusCode >= 100 && statusCode < 200 {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}
	if w.Status == 0 {
		w.Status = statusCode
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *StatusWriter) Write(b []byte) (int, error) {
	if w.Status == 0 {
		w.Status = http.StatusOK
	}
	if w.CaptureBody {
		w.appendBody(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *StatusWriter) appendBody(b []byte) {
	limit := w.MaxBodyBytes
	if limit <= 0 {
		limit = defaultCaptureBodyLimit
	}
	remaining := limit - len(w.Body)
	if remaining <= 0 {
		return
	}
	if len(b) > remaining {
		b = b[:remaining]
	}
	w.Body = append(w.Body, b...)
}

// HTTPStatus returns the captured status, defaulting to 200 when WriteHeader was never called.
func HTTPStatus(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}
