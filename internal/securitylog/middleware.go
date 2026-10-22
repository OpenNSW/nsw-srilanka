// Package securitylog records every inbound HTTP request as a slog line with
// category=security. The line names who called, and carries the request
// payload and the response. The slog timestamp is when. Argus is not used.
//
// Authorization headers are not logged. Non-text bodies are omitted; only
// their size is recorded. Text bodies are capped at maxLoggedBodyBytes.
package securitylog

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

// maxLoggedBodyBytes is the most request or response body text copied onto
// the security line. The handler still receives the full request body.
const maxLoggedBodyBytes = 64 << 10

// Middleware logs one security line after next returns. logger nil uses
// slog.Default. The logger must already be the process handler so the line
// picks up the request trace id.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("category", "security"))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller := &authn.RequestCaller{}
			r = r.WithContext(authn.WithRequestCaller(r.Context(), caller))

			reqPrefix, reqTruncated := peekBody(r, maxLoggedBodyBytes)
			rec := &capture{ResponseWriter: w, max: maxLoggedBodyBytes}
			start := time.Now()
			next.ServeHTTP(rec, r)

			kind, id := "anonymous", ""
			if caller.Principal != nil {
				kind = string(caller.Principal.Kind)
				id = caller.Principal.Subject()
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}

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
			if r.ContentLength >= 0 {
				attrs = append(attrs, "requestBytes", r.ContentLength)
			}
			if r.Pattern != "" {
				attrs = append(attrs, "pattern", r.Pattern)
			}
			attrs = append(attrs, bodyAttrs("request", r.Header.Get("Content-Type"), reqPrefix, reqTruncated)...)
			attrs = append(attrs, bodyAttrs("response", rec.Header().Get("Content-Type"), rec.prefix, rec.truncated)...)
			if rec.n > 0 {
				attrs = append(attrs, "responseBytes", rec.n)
			}

			logger.InfoContext(r.Context(), "http request", attrs...)
		})
	}
}

// peekBody copies up to max bytes for the log and leaves r.Body readable
// from the start for the handler.
func peekBody(r *http.Request, max int) (prefix []byte, truncated bool) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, false
	}
	buf := make([]byte, max+1)
	n, err := io.ReadFull(r.Body, buf)
	if n > 0 {
		prefix = buf[:n]
		if n > max {
			prefix = buf[:max]
			truncated = true
		}
		r.Body = &readCloser{
			Reader: io.MultiReader(bytes.NewReader(buf[:n]), r.Body),
			Closer: r.Body,
		}
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF && n == 0 {
		return nil, false
	}
	return prefix, truncated
}

type readCloser struct {
	io.Reader
	io.Closer
}

// capture records status and a prefix of the response body while writing
// through to the client.
type capture struct {
	http.ResponseWriter
	status    int
	max       int
	n         int
	prefix    []byte
	truncated bool
}

func (c *capture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *capture) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.n += len(b)
	if len(c.prefix) < c.max {
		room := c.max - len(c.prefix)
		chunk := b
		if len(chunk) > room {
			chunk = chunk[:room]
			c.truncated = true
		}
		c.prefix = append(c.prefix, chunk...)
	} else if len(b) > 0 {
		c.truncated = true
	}
	return c.ResponseWriter.Write(b)
}

func (c *capture) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *capture) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func bodyAttrs(prefix, contentType string, body []byte, truncated bool) []any {
	if len(body) == 0 {
		return nil
	}
	name := prefix + "Body"
	if !loggableText(contentType, body) {
		attrs := []any{prefix + "BodyOmitted", true}
		if truncated {
			attrs = append(attrs, prefix+"BodyTruncated", true)
		}
		return attrs
	}
	attrs := []any{name, string(body)}
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
