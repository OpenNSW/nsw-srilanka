// Package securitylog records sparse category=security events worth alerting
// on (authn failures, authz denials, bad callback tokens). Lines name who
// called, which route, the status, and a short reason. Request and response
// payloads are never logged: forms are JSON Schema–driven (jsonforms), so
// field-level redaction cannot list every PII key.
package securitylog

import (
	"context"
	"log/slog"
)

// Category is the slog attribute value on every security event. Collectors
// filter on this string; keep a single definition here.
const Category = "security"

// Event writes one security line at warn. logger nil uses slog.Default.
// Callers must not pass request or response bodies.
func Event(ctx context.Context, logger *slog.Logger, msg string, attrs ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.With(slog.String("category", Category)).WarnContext(ctx, msg, attrs...)
}
