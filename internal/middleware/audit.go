package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authn"
)

type customResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bodyBuf    bytes.Buffer
}

func (crw *customResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}

func (crw *customResponseWriter) Write(b []byte) (int, error) {
	crw.bodyBuf.Write(b)
	return crw.ResponseWriter.Write(b)
}

// AuditMiddleware intercepts POST, PUT, PATCH, and DELETE requests,
// extracts authenticated user contexts, and records audit logs to the Argus service.
func AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.ToUpper(r.Method)
		// Only log write operations
		if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
			next.ServeHTTP(w, r)
			return
		}

		// Read request body to preserve/log, skipping multipart to avoid buffering large files in memory
		var bodyBytes []byte
		if r.Body != nil {
			contentType := r.Header.Get("Content-Type")
			if !strings.HasPrefix(contentType, "multipart/") {
				var err error
				bodyBytes, err = io.ReadAll(r.Body)
				if err == nil {
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
			} else {
				bodyBytes = []byte("[multipart/form-data body omitted]")
			}
		}

		crw := &customResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default status code if WriteHeader is not called
		}

		next.ServeHTTP(crw, r)

		// Extract Trace ID synchronously to avoid data race on r.Header map in the goroutine
		var traceID string
		if tid := r.Header.Get("X-Trace-ID"); tid != "" {
			traceID = tid
		} else if cid := r.Header.Get("X-Correlation-ID"); cid != "" {
			traceID = cid
		} else if rid := r.Header.Get("X-Request-ID"); rid != "" {
			traceID = rid
		}

		// Record the audit log asynchronously to not block the request path
		go func(path string, method string, reqBody []byte, respBody []byte, statusCode int, authCtx *authn.AuthContext, traceID string) {
			// Context for logging
			ctx := context.Background()

			var actorType = "SYSTEM"
			var actorID = "anonymous"
			var roles []string

			if authCtx != nil {
				actorID = authCtx.Subject()
				roles = authCtx.Roles()
				if authCtx.Type() == authn.UserPrincipalType {
					isAdmin := false
					for _, r := range roles {
						if strings.EqualFold(r, "admin") {
							isAdmin = true
							break
						}
					}
					if isAdmin {
						actorType = "ADMIN"
					} else {
						actorType = "MEMBER"
					}
				} else if authCtx.Type() == authn.ClientPrincipalType {
					actorType = "SERVICE"
				}
			}

			// Map HTTP method to event action
			var action = "READ"
			switch method {
			case "POST":
				action = "CREATE"
			case "PUT", "PATCH":
				action = "UPDATE"
			case "DELETE":
				action = "DELETE"
			}

			// Map status code to SUCCESS/FAILURE
			status := audit.StatusSuccess
			if statusCode >= 400 {
				status = audit.StatusFailure
			}

			// Extract target ID
			var targetIDVal string
			// 1. Try path value id/key first (if already set in handler routing context)
			// Since we're in a background goroutine, we cannot access path values of r,
			// but we can parse the path prefix/suffixes.
			parts := strings.Split(strings.Trim(path, "/"), "/")
			if len(parts) > 0 {
				// E.g., /api/v1/consignments/123 -> parts: ["api", "v1", "consignments", "123"]
				lastPart := parts[len(parts)-1]
				// Check if the last part is not a collection name
				if lastPart != "consignments" && lastPart != "tasks" && lastPart != "storage" && lastPart != "payments" {
					targetIDVal = lastPart
				}
			}

			// 2. If it's a create action and targetID is empty, try to parse ID from response body
			if targetIDVal == "" && (statusCode == http.StatusOK || statusCode == http.StatusCreated || statusCode == http.StatusAccepted) {
				var respObj map[string]interface{}
				if err := json.Unmarshal(respBody, &respObj); err == nil {
					if id, ok := respObj["id"].(string); ok && id != "" {
						targetIDVal = id
					} else if key, ok := respObj["key"].(string); ok && key != "" {
						targetIDVal = key
					}
				}
			}

			// 3. Fallback to path if still empty
			if targetIDVal == "" {
				targetIDVal = path
			}

			// Determine TargetType
			targetType := "RESOURCE"

			metadata := map[string]interface{}{
				"path":        path,
				"status_code": statusCode,
				"roles":       roles,
			}

			// Log the event using the global audit middleware client
			auditReq := &audit.AuditLogRequest{
				Timestamp:  audit.CurrentTimestamp(),
				EventType:  "MANAGEMENT_EVENT",
				Action:     action,
				Status:     status,
				ActorType:  actorType,
				ActorID:    actorID,
				TargetType: targetType,
				TargetID:   &targetIDVal,
				Message:    reqBody,
				Metadata:   metadata,
			}

			if traceID != "" {
				auditReq.TraceID = &traceID
			}

			audit.LogAuditEvent(ctx, auditReq)

		}(r.URL.Path, method, bodyBytes, crw.bodyBuf.Bytes(), crw.statusCode, authn.GetAuthContext(r.Context()), traceID)
	})
}
