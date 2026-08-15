// Package cusdec holds the Sri Lanka Customs (SLC Edge) Customs Declaration
// integration: the outbound submission (§6.1), built here on top of the
// generic API-call plugin, and the inbound integration-result and event
// callbacks (§6.2, §6.5) handled by the webhook service.
package cusdec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/OpenNSW/core/remote"
)

// SLC Edge submission statuses that count as accepted. RECEIVED is what the
// spec (§6.1.4) and the live endpoint return; the other two predate it and are
// kept so a service still on the old vocabulary is not read as a rejection.
const (
	statusQueued   = "QUEUED"
	statusAccepted = "ACCEPTED"
	statusReceived = "RECEIVED"
)

// capturedFields are the SLC Edge response fields surfaced to the workflow
// (and the UI) — the lifecycle ids plus either-shape error detail. edgeId is
// the correlation ID the integration-result callback is matched back to a
// workflow by (§2.1), so losing it here strands the declaration.
var capturedFields = []string{
	"edgeId", "status", "receivedAt", "acceptedAt", "errors", "nswId",
	"message", "error", "detail", "title", "code", "fieldRef",
}

// FileFetcher retrieves an uploaded file's content by storage key. It is the
// subset of the storage service this package needs, so the interpreter can be
// tested without one. Download returns the content and its MIME type.
type FileFetcher interface {
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
}

// maxAttachmentBytes caps a single attachment, matching the 300 KB per-file
// limit in §8. Enforced here so an oversized file is reported to the trader as
// a named document rather than as an opaque rejection of the whole submission.
const maxAttachmentBytes = 300 * 1024

// CusdecInterpreter adapts the generic API-call plugin to the SLC Edge Customs
// Declaration submission API (§6.1): it maps the trader form onto the Annex A
// payload, attaches the declared supporting documents, and interprets the
// acknowledgement into an acceptance flag plus the fields to record (with a
// trader-facing error message on rejection).
//
// The submission is multipart/form-data — a JSON payload part alongside the
// files — so this interpreter implements BuildParts and the plugin routes it
// through the multipart transport rather than sending a JSON body.
type CusdecInterpreter struct {
	files FileFetcher
}

// NewCusdecInterpreter returns the SLC Edge CusDec interpreter. files may be
// nil, in which case a declaration carrying supporting documents is rejected
// before it is sent rather than arriving without its attachments.
func NewCusdecInterpreter(files FileFetcher) *CusdecInterpreter {
	return &CusdecInterpreter{files: files}
}

// BuildRequest satisfies the Interpreter interface. The CusDec submission is
// always multipart, so the plugin calls BuildParts instead; this returns the
// mapped payload so the interface contract still holds if that ever changes.
func (c CusdecInterpreter) BuildRequest(inputs map[string]any) any {
	payload, _, err := buildFromInputs(inputs)
	if err != nil {
		return inputs
	}
	return payload
}

// BuildParts assembles the §6.1.1 multipart body: exactly one payload part
// carrying the declaration JSON, a fileinfo part holding the attachment count,
// and one contiguous fileN part per declared document.
func (c CusdecInterpreter) BuildParts(ctx context.Context, inputs map[string]any) ([]remote.Part, error) {
	payload, docs, err := buildFromInputs(inputs)
	if err != nil {
		return nil, err
	}

	payloadPart, err := remote.JSONPart("payload", payload)
	if err != nil {
		return nil, &buildError{err.Error()}
	}

	parts := make([]remote.Part, 0, len(docs)+2)
	parts = append(parts, payloadPart,
		// §6.1.1: the count must equal the number of file parts, and 0 is the
		// correct value when there is nothing attached.
		remote.Part{Name: "fileinfo", Content: []byte(strconv.Itoa(len(docs)))})

	for i, doc := range docs {
		content, mime, err := c.fetch(ctx, doc)
		if err != nil {
			return nil, err
		}
		parts = append(parts, remote.Part{
			// fileN is numbered from 1 and must be contiguous (§6.1.1).
			Name:        fmt.Sprintf("file%d", i+1),
			FileName:    doc.FileName,
			ContentType: mime,
			Content:     content,
		})
	}

	return parts, nil
}

func (c CusdecInterpreter) fetch(ctx context.Context, doc SupportDoc) ([]byte, string, error) {
	if c.files == nil {
		return nil, "", &buildError{"Supporting documents cannot be attached in this environment."}
	}

	body, mime, err := c.files.Download(ctx, doc.storageKey)
	if err != nil {
		return nil, "", &buildError{fmt.Sprintf("Attachment %q could not be read. Please re-upload it and submit again.", doc.FileName)}
	}
	defer func() { _ = body.Close() }()

	// Read one byte past the limit so an oversized file is detected rather
	// than silently truncated into an invalid PDF.
	content, err := io.ReadAll(io.LimitReader(body, maxAttachmentBytes+1))
	if err != nil {
		return nil, "", &buildError{fmt.Sprintf("Attachment %q could not be read. Please re-upload it and submit again.", doc.FileName)}
	}
	if len(content) > maxAttachmentBytes {
		return nil, "", &buildError{fmt.Sprintf("Attachment %q is larger than the %d KB limit Sri Lanka Customs accepts.", doc.FileName, maxAttachmentBytes/1024)}
	}
	if mime == "" {
		mime = "application/pdf"
	}
	return content, mime, nil
}

// buildError marks a failure to assemble the submission (a missing file, an
// oversized attachment) as opposed to a transport failure, so Interpret can
// surface the message to the trader verbatim.
type buildError struct{ msg string }

func (e *buildError) Error() string { return e.msg }

func buildFromInputs(inputs map[string]any) (Submission, []SupportDoc, error) {
	form, ok := inputs["payload"].(map[string]any)
	if !ok {
		return Submission{}, nil, &buildError{"The declaration form could not be read."}
	}
	payload, docs, err := BuildPayload(form)
	if err != nil {
		return Submission{}, nil, &buildError{err.Error()}
	}
	return payload, docs, nil
}

// Interpret reports whether the submission was accepted and captures the SLC
// response fields (and a trader-facing error message on rejection).
func (CusdecInterpreter) Interpret(callErr error, resp map[string]any) (bool, map[string]any) {
	accepted := callErr == nil && !hasErrors(resp) && statusIsAccepted(resp)

	out := map[string]any{}
	for _, k := range capturedFields {
		if v, ok := resp[k]; ok {
			out[k] = v
		}
	}

	if !accepted {
		out["error"] = describeFailure(callErr, resp)
	}
	return accepted, out
}

// hasErrors reports whether the response carries a non-empty "errors" array.
func hasErrors(resp map[string]any) bool {
	errs, ok := resp["errors"].([]any)
	return ok && len(errs) > 0
}

// statusIsAccepted reports whether the response's "status" is an explicitly
// accepted value. Success statuses are defined by the spec, so anything else —
// including a missing or non-string status (e.g. a problem+json error body) —
// is treated as not accepted.
func statusIsAccepted(resp map[string]any) bool {
	s, ok := resp["status"].(string)
	if !ok {
		return false
	}
	return s == statusQueued || s == statusAccepted || s == statusReceived
}

// describeFailure builds a trader-facing, markdown message for a rejected
// submission. It prefers the SLC Edge error detail in the response body — the
// §6.1.4 {"error": "<reason>"} string, an "errors" array of
// {code,message,fieldRef}, or a problem+json "detail"/"title" — and falls back
// to distinguishing a transport failure from an unexplained rejection.
func describeFailure(callErr error, resp map[string]any) string {
	const intro = "Your customs declaration was not accepted by Sri Lanka Customs:"
	const outro = "\n\nPlease correct the highlighted fields and resubmit."

	// A local assembly failure never reached Customs, so it is reported on its
	// own terms rather than as their rejection.
	var be *buildError
	if errors.As(callErr, &be) {
		return "Your customs declaration could not be submitted:\n\n- " + be.msg
	}

	bullets := validationBullets(resp)
	if len(bullets) == 0 {
		// §6.1.4 rejects with a single "error" string; the other two shapes
		// predate the spec and are kept for services still sending them.
		for _, key := range []string{"error", "detail", "title"} {
			if s := stringField(resp, key); s != "" {
				bullets = []string{"- " + s}
				break
			}
		}
	}
	if len(bullets) > 0 {
		return intro + "\n\n" + strings.Join(bullets, "\n") + outro
	}

	// A rejection carrying no detail is still a rejection: only treat this as
	// unreachable when the response body was empty too, so a 4xx with an
	// unrecognized shape is not reported to the trader as a network problem.
	if callErr != nil && len(resp) == 0 {
		return "We could not reach Sri Lanka Customs to submit your declaration. Please try again in a few minutes."
	}
	return intro + outro
}

// validationBullets renders each entry of the "errors" array as a markdown
// bullet: the message (or code) with the offending field in italics.
func validationBullets(resp map[string]any) []string {
	errs, ok := resp["errors"].([]any)
	if !ok {
		return nil
	}
	bullets := make([]string, 0, len(errs))
	for _, e := range errs {
		m, ok := e.(map[string]any)
		if !ok {
			bullets = append(bullets, "- "+fmt.Sprintf("%v", e))
			continue
		}
		msg, _ := m["message"].(string)
		if msg == "" {
			msg, _ = m["code"].(string)
		}
		if field, _ := m["fieldRef"].(string); field != "" && msg != "" {
			msg = fmt.Sprintf("%s _(%s)_", msg, field)
		}
		if msg != "" {
			bullets = append(bullets, "- "+msg)
		}
	}
	return bullets
}

// stringField returns resp[key] as a string, or "" if absent or not a string.
func stringField(resp map[string]any, key string) string {
	s, _ := resp[key].(string)
	return s
}
