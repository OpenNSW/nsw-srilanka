// Package securitylog records every inbound HTTP request as a slog line with
// category=security, except GET /health. The line names who called, and carries
// the request payload and the response. The slog timestamp is when.
//
// Authorization headers are not logged. Credential fields (signed storage URLs,
// tokens, passwords, signatures) are redacted. The callback token in
// /api/v1/callbacks/{token} is replaced in the path. Non-text bodies, and
// responses copied with io.ReaderFrom (file downloads), record their size only.
// Text bodies are capped at maxLoggedBodyBytes. The body is copied as the
// handler reads it, so a request rejected before any read does not have its
// body pulled in. The handler still receives the full request body.
package securitylog

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/felixge/httpsnoop"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// maxLoggedBodyBytes is the most request or response body text copied onto
// the security line. Kept small so a task-view poll does not emit a line large
// enough for a log shipper to split.
const maxLoggedBodyBytes = 4 << 10

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

			body := captureRequestBody(r, maxLoggedBodyBytes)
			resp := &responseLog{max: maxLoggedBodyBytes}
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
				read := int64(0)
				var reqPrefix []byte
				reqTruncated := false
				if body != nil {
					read = body.n
					reqPrefix = body.prefix
					reqTruncated = body.truncated
				}
				reqSize := requestSize(r.ContentLength, read)

				attrs := []any{
					"method", r.Method,
					"path", loggedPath(r),
					"callerKind", kind,
					"callerID", id,
					"status", status,
					"durationMs", time.Since(start).Milliseconds(),
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					attrs = append(attrs, "origin", origin)
				}
				if q := loggedQuery(r.URL.RawQuery); q != "" {
					attrs = append(attrs, "query", q)
				}
				if reqSize > 0 {
					attrs = append(attrs, "requestBytes", reqSize)
				}
				if r.Pattern != "" {
					attrs = append(attrs, "pattern", r.Pattern)
				}
				attrs = append(attrs, bodyAttrs("request", r.Header.Get("Content-Type"), reqPrefix, reqTruncated)...)
				if resp.streamed && len(resp.prefix) == 0 {
					attrs = append(attrs, "responseBodyOmitted", true)
				} else {
					attrs = append(attrs, bodyAttrs("response", wrapped.Header().Get("Content-Type"), resp.prefix, resp.truncated)...)
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

// requestSize is the request body size to log. A declared Content-Length wins.
// A chunked body has no length, so the size is however much the handler read.
func requestSize(contentLength, read int64) int {
	if contentLength >= 0 {
		return int(contentLength)
	}
	return int(read)
}

// captureRequestBody records up to max bytes as the handler reads r.Body.
// Nothing is read here: authentication can reject the request without pulling
// a slow or chunked body onto the connection.
func captureRequestBody(r *http.Request, max int) *capturedBody {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	body := &capturedBody{
		ReadCloser: r.Body,
		max:        max,
		prefix:     make([]byte, 0, prefixCap(r.ContentLength, max)),
	}
	r.Body = body
	return body
}

func prefixCap(contentLength int64, max int) int {
	if contentLength <= 0 || max <= 0 {
		return 0
	}
	if contentLength < int64(max) {
		return int(contentLength)
	}
	return max
}

type capturedBody struct {
	io.ReadCloser
	max       int
	prefix    []byte
	truncated bool
	n         int64
}

func (c *capturedBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		c.n += int64(n)
		c.add(p[:n])
	}
	return n, err
}

func (c *capturedBody) add(b []byte) {
	if len(c.prefix) >= c.max {
		c.truncated = true
		return
	}
	room := c.max - len(c.prefix)
	if len(b) > room {
		b = b[:room]
		c.truncated = true
	}
	c.prefix = append(c.prefix, b...)
	if c.n > int64(len(c.prefix)) {
		c.truncated = true
	}
}

// responseLog records the status and a prefix of bytes passed to Write or
// WriteString. ReadFrom is delegated unchanged so a file download can still
// use sendfile; those bytes are counted and not copied onto the log line.
type responseLog struct {
	status    int
	wroteHdr  bool
	max       int
	n         int
	prefix    []byte
	truncated bool
	streamed  bool
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
				rec.add(p[:n])
				return n, err
			}
		},
		WriteString: func(next httpsnoop.WriteStringFunc) httpsnoop.WriteStringFunc {
			return func(s string) (int, error) {
				n, err := next(s)
				if n > 0 {
					rec.add([]byte(s[:n]))
				}
				return n, err
			}
		},
		ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
			return func(src io.Reader) (int64, error) {
				n, err := next(src)
				if n > 0 {
					rec.n += int(n)
					rec.streamed = true
					rec.wroteHdr = true
				}
				return n, err
			}
		},
	}
}

func (rec *responseLog) add(b []byte) {
	if len(b) == 0 {
		return
	}
	rec.n += len(b)
	rec.wroteHdr = true
	if len(rec.prefix) >= rec.max {
		rec.truncated = true
		return
	}
	room := rec.max - len(rec.prefix)
	if len(b) > room {
		b = b[:room]
		rec.truncated = true
	}
	rec.prefix = append(rec.prefix, b...)
}

func bodyAttrs(prefix, contentType string, body []byte, truncated bool) []any {
	if len(body) == 0 {
		return nil
	}
	if !loggableText(contentType, body) {
		attrs := []any{prefix + "BodyOmitted", true}
		if truncated {
			attrs = append(attrs, prefix+"BodyTruncated", true)
		}
		return attrs
	}
	logged := redactLogged(contentType, body)
	attrs := []any{prefix + "Body", string(logged)}
	if truncated {
		attrs = append(attrs, prefix+"BodyTruncated", true)
	}
	return attrs
}

func loggableText(contentType string, body []byte) bool {
	ct := mediaType(contentType)
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/json" || strings.HasSuffix(ct, "+json"):
		return true
	case ct == "application/xml" || strings.HasSuffix(ct, "+xml"):
		return true
	case ct == "application/x-www-form-urlencoded":
		return true
	case ct == "":
		return utf8.Valid(body) && !bytesContainsNUL(body)
	default:
		return false
	}
}

func bytesContainsNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}
