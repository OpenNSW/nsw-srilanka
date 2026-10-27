// Package securitylog records every inbound HTTP request as a slog line with
// category=security, except GET /health. The line names who called (caller
// and remote address), the route, status, and duration. Request and response
// payloads are not logged: forms are JSON Schema–driven (jsonforms), so field-
// level redaction cannot list every PII key.
//
// Authorization headers are not logged. The callback token in
// /api/v1/callbacks/{token} is replaced in the path.
package securitylog

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// Middleware logs one security line after next returns, including when next
// panics. logger nil uses slog.Default. The logger must already be the process
// handler so the line picks up the request trace id. GET /health is served and
// not logged.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("category", "security"))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			caller := &authn.RequestCaller{}
			r = r.WithContext(authn.WithRequestCaller(r.Context(), caller))

			resp := &responseLog{}
			wrapped := httpsnoop.Wrap(w, resp.hooks())
			start := time.Now()

			defer func() {
				panicked := recover()

				kind, id := "anonymous", ""
				if caller.Principal != nil {
					kind = string(caller.Principal.Kind)
					id = caller.Principal.Subject()
				}
				// A handler that returns without writing is answered 200 by
				// net/http. A panic before any write is not: the server aborts
				// the connection. Log that as 500, then re-panic so the server
				// still handles it.
				status := resp.status
				if status == 0 {
					if panicked != nil {
						status = http.StatusInternalServerError
					} else {
						status = http.StatusOK
					}
				}

				attrs := []any{
					"method", r.Method,
					"path", loggedPath(r),
					"remoteAddr", r.RemoteAddr,
					"callerKind", kind,
					"callerID", id,
					"status", status,
					"durationMs", time.Since(start).Milliseconds(),
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					attrs = append(attrs, "origin", origin)
				}
				if r.ContentLength > 0 {
					attrs = append(attrs, "requestBytes", r.ContentLength)
				}
				if r.Pattern != "" {
					attrs = append(attrs, "pattern", r.Pattern)
				}
				if resp.n > 0 {
					attrs = append(attrs, "responseBytes", resp.n)
				}
				logger.InfoContext(r.Context(), "http request", attrs...)

				if panicked != nil {
					panic(panicked)
				}
			}()

			next.ServeHTTP(wrapped, r)
		})
	}
}

// loggedPath hides the callback token. The mux records it as the {token} path
// value on this request; the raw path would otherwise keep the secret.
func loggedPath(r *http.Request) string {
	token := r.PathValue("token")
	if token == "" {
		return r.URL.Path
	}
	return strings.Replace(r.URL.Path, token, "{token}", 1)
}

// responseLog records the status and response size. Bodies are not copied.
// ReadFrom is delegated unchanged so a file download can still use sendfile.
type responseLog struct {
	status   int
	wroteHdr bool
	n        int
}

func (rec *responseLog) hooks() httpsnoop.Hooks {
	return httpsnoop.Hooks{
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				next(code)
				// 101 is the final status of an upgrade. Other 1xx codes are
				// interim and must not hide the status that follows.
				if code == http.StatusSwitchingProtocols {
					rec.status = code
					rec.wroteHdr = true
					return
				}
				if code >= 100 && code <= 199 || rec.wroteHdr {
					return
				}
				rec.status = code
				rec.wroteHdr = true
			}
		},
		Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(p []byte) (int, error) {
				n, err := next(p)
				if n > 0 {
					rec.n += n
					rec.wroteHdr = true
				}
				return n, err
			}
		},
		WriteString: func(next httpsnoop.WriteStringFunc) httpsnoop.WriteStringFunc {
			return func(s string) (int, error) {
				n, err := next(s)
				if n > 0 {
					rec.n += n
					rec.wroteHdr = true
				}
				return n, err
			}
		},
		ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
			return func(src io.Reader) (int64, error) {
				n, err := next(src)
				if n > 0 {
					rec.n += int(n)
					rec.wroteHdr = true
				}
				return n, err
			}
		},
	}
}
