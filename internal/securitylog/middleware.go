// Package securitylog records every inbound HTTP request as a slog line with
// category=security. The line names who called, and carries the request
// payload and the response. The slog timestamp is when.
//
// Authorization headers are not logged. Non-text bodies, and responses copied
// with io.ReaderFrom (file downloads), record their size only. Text bodies are
// capped at maxLoggedBodyBytes. The handler still receives the full request body.
package securitylog

import (
	"bytes"
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
// the security line.
const maxLoggedBodyBytes = 64 << 10

// Middleware logs one security line after next returns, including when next
// panics. logger nil uses slog.Default. The logger must already be the process
// handler so the line picks up the request trace id.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("category", "security"))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller := &authn.RequestCaller{}
			r = r.WithContext(authn.WithRequestCaller(r.Context(), caller))

			reqPrefix, reqTruncated, bodyRead := peekBody(r, maxLoggedBodyBytes)
			resp := &responseLog{max: maxLoggedBodyBytes}
			wrapped := httpsnoop.Wrap(w, resp.hooks())
			start := time.Now()

			defer func() {
				kind, id := "anonymous", ""
				if caller.Principal != nil {
					kind = string(caller.Principal.Kind)
					id = caller.Principal.Subject()
				}
				status := resp.status
				if status == 0 {
					status = http.StatusOK
				}
				read := int64(0)
				if bodyRead != nil {
					read = bodyRead.n
				}
				reqSize := requestSize(r.ContentLength, reqPrefix, reqTruncated, read)

				attrs := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"callerKind", kind,
					"callerID", id,
					"status", status,
					"durationMs", time.Since(start).Milliseconds(),
				}
				if r.URL.RawQuery != "" {
					attrs = append(attrs, "query", r.URL.RawQuery)
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
			}()

			next.ServeHTTP(wrapped, r)
		})
	}
}

// requestSize is the request body size to log. A declared Content-Length wins.
// Otherwise a fully peeked body (the read hit EOF) is that prefix, and a
// truncated chunked body uses however much the handler went on to read.
func requestSize(contentLength int64, prefix []byte, truncated bool, read int64) int {
	if contentLength >= 0 {
		return int(contentLength)
	}
	if truncated && read > int64(len(prefix)) {
		return int(read)
	}
	return len(prefix)
}

// peekBody copies up to max bytes for the log and leaves r.Body readable from
// the start for the handler. The returned counter tracks bytes the handler reads.
func peekBody(r *http.Request, max int) (prefix []byte, truncated bool, read *readCounter) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, false, nil
	}
	buf := make([]byte, max+1)
	n, err := io.ReadFull(r.Body, buf)
	if n > 0 {
		prefix = buf[:n]
		if n > max {
			prefix = buf[:max]
			truncated = true
		}
		read = &readCounter{ReadCloser: &readCloser{
			Reader: io.MultiReader(bytes.NewReader(buf[:n]), r.Body),
			Closer: r.Body,
		}}
		r.Body = read
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF && n == 0 {
		return nil, false, nil
	}
	return prefix, truncated, read
}

type readCloser struct {
	io.Reader
	io.Closer
}

type readCounter struct {
	io.ReadCloser
	n int64
}

func (c *readCounter) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.n += int64(n)
	return n, err
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
	attrs := []any{prefix + "Body", string(body)}
	if truncated {
		attrs = append(attrs, prefix+"BodyTruncated", true)
	}
	return attrs
}

func loggableText(contentType string, body []byte) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
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
		return utf8.Valid(body) && !bytes.Contains(body, []byte{0})
	default:
		return false
	}
}
