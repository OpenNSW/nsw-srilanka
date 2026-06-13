package middleware

import (
	"context"
	"net/http"

	"github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// TraceMiddleware extracts a trace ID from incoming headers and injects it into the request context.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var traceID string
		if tid := r.Header.Get("X-Trace-ID"); tid != "" {
			traceID = tid
		} else if cid := r.Header.Get("X-Correlation-ID"); cid != "" {
			traceID = cid
		} else if rid := r.Header.Get("X-Request-ID"); rid != "" {
			traceID = rid
		}

		if traceID != "" {
			w.Header().Set("X-Trace-ID", traceID)
			ctx := context.WithValue(r.Context(), audit.TraceIDKey, traceID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}
