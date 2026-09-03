package ephyto

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

// stubFiles serves document content by storage key, as the storage service does.
type stubFiles struct {
	content     map[string][]byte
	contentType string
	err         error
	asked       []string
}

func (s *stubFiles) Download(_ context.Context, key string) (io.ReadCloser, string, error) {
	s.asked = append(s.asked, key)
	if s.err != nil {
		return nil, "", s.err
	}
	body, ok := s.content[key]
	if !ok {
		return nil, "", errors.New("no such object")
	}
	return io.NopCloser(bytes.NewReader(body)), s.contentType, nil
}

// sending builds the submit inputs for a certificate that carries documents.
func sending(extra map[string]any) map[string]any {
	inputs := submitInputs()
	for k, v := range extra {
		inputs[k] = v
	}
	return inputs
}

// A document the trader said yes to travels inside the certificate: base64
// content, the filename, and the MIME type the Hub's validator requires.
func TestBuildEnvelope_AttachesTheSelectedDocuments(t *testing.T) {
	pdf := []byte("%PDF-1.7 fumigation certificate")
	files := &stubFiles{
		content:     map[string][]byte{"storage/certs/fumigation.pdf": pdf},
		contentType: "application/pdf",
	}

	envelope, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		"treatment_certificate_url":  "storage/certs/fumigation.pdf",
		"send_treatment_certificate": true,
	}))
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}

	if got := files.asked; len(got) != 1 || got[0] != "storage/certs/fumigation.pdf" {
		t.Fatalf("storage reads = %v", got)
	}
	for _, want := range []string{
		`mimeCode="application/pdf"`,
		`filename="fumigation.pdf"`,
		base64.StdEncoding.EncodeToString(pdf),
		"<ram:ID>Treatment Certificate</ram:ID>",
	} {
		if !strings.Contains(envelope, want) {
			t.Errorf("envelope is missing %q", want)
		}
	}
}

// Nothing is read for a document the trader said no to.
func TestBuildEnvelope_ReadsNothingForADocumentNotSent(t *testing.T) {
	files := &stubFiles{content: map[string][]byte{"storage/certs/fumigation.pdf": []byte("%PDF")}}

	_, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		"treatment_certificate_url":  "storage/certs/fumigation.pdf",
		"send_treatment_certificate": false,
	}))
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if len(files.asked) != 0 {
		t.Errorf("storage was read for a document the trader excluded: %v", files.asked)
	}
}

// A certificate must not claim to carry a document that could not be read.
func TestBuildEnvelope_RefusesWhenADocumentCannotBeRead(t *testing.T) {
	files := &stubFiles{err: errors.New("connection reset")}

	_, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		"invoice_file_url": "storage/docs/invoice.pdf",
		"send_invoice":     true,
	}))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	var be *buildError
	if !errors.As(err, &be) {
		t.Fatalf("want a trader-facing build error, got %T", err)
	}
	for _, want := range []string{"could not be attached", "Commercial Invoice", "invoice.pdf"} {
		if !strings.Contains(be.msg, want) {
			t.Errorf("message %q is missing %q", be.msg, want)
		}
	}
}

// The ePhyto Guidelines cap an attachment at 3 MB and the Hub validates it on
// receipt, so an oversized document is refused here — where the reason reaches
// the trader — rather than there, as a validation fault they never see.
func TestBuildEnvelope_RefusesAnOversizedDocument(t *testing.T) {
	files := &stubFiles{
		content:     map[string][]byte{"storage/docs/scan.pdf": bytes.Repeat([]byte("x"), maxAttachmentBytes+1)},
		contentType: "application/pdf",
	}

	_, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		"invoice_file_url": "storage/docs/scan.pdf",
		"send_invoice":     true,
	}))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "3.0 MB") {
		t.Errorf("the message should name the limit: %v", err)
	}
}

// A deployment with no storage configured says so rather than sending a
// certificate that claims documents it has not enclosed.
func TestBuildEnvelope_RefusesWithNoStorageConfigured(t *testing.T) {
	_, err := NewHubInterpreter(nil).BuildEnvelope(OpSubmit, sending(map[string]any{
		"invoice_file_url": "storage/docs/invoice.pdf",
		"send_invoice":     true,
	}))
	if err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("want a refusal naming storage, got %v", err)
	}
}

// Storage that reports no content type falls back to the extension, because the
// Hub's validator rejects an empty mimeCode.
func TestResolveAttachments_FallsBackToTheExtension(t *testing.T) {
	files := &stubFiles{content: map[string][]byte{"k/permit.PNG": []byte("png bytes")}}

	envelope, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		"additional_file_url":      "k/permit.PNG",
		"send_additional_document": true,
	}))
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if !strings.Contains(envelope, `mimeCode="image/png"`) {
		t.Error("expected the extension to supply the MIME type")
	}
}

// A certificate with nothing attached carries no storage key either — Key is
// resolved away, never emitted.
func TestResolveAttachments_NeverEmitsTheStorageKey(t *testing.T) {
	files := &stubFiles{content: map[string][]byte{"secret/internal/path/invoice.pdf": []byte("%PDF")}}

	envelope, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		"invoice_file_url": "secret/internal/path/invoice.pdf",
		"send_invoice":     true,
	}))
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if strings.Contains(envelope, "secret/internal/path") {
		t.Error("the storage key leaked into the certificate")
	}
}

// A branch the consignment never took leaves its upload field absent, and the
// ePhyto form still offers the tick — it lists every document the flow can
// produce, not the ones this consignment did. Ticking one that was never
// uploaded is not an error: there is nothing to attach and nothing to refuse,
// so the certificate goes without it.
func TestBuildEnvelope_SkipsADocumentTickedButNeverUploaded(t *testing.T) {
	files := &stubFiles{content: map[string][]byte{"storage/docs/invoice.pdf": []byte("%PDF")}}

	envelope, err := NewHubInterpreter(files).BuildEnvelope(OpSubmit, sending(map[string]any{
		// The treatment step never ran, so no URL was mapped in — but the
		// trader ticked its box.
		"send_treatment_certificate": true,
		"invoice_file_url":           "storage/docs/invoice.pdf",
		"send_invoice":               true,
	}))
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if strings.Contains(envelope, "Treatment Certificate") {
		t.Error("a document that was never uploaded was named on the certificate")
	}
	if !strings.Contains(envelope, "Commercial Invoice") {
		t.Error("the document that does exist should still travel")
	}
	if len(files.asked) != 1 || files.asked[0] != "storage/docs/invoice.pdf" {
		t.Errorf("storage reads = %v, want only the invoice", files.asked)
	}
}
