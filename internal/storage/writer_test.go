package storage

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	corestorage "github.com/OpenNSW/core/storage"
)

// storedKey is the shape the portal treats as a stored file rather than a link.
var storedKey = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.pdf$`)

// readBack downloads key through svc and checks it holds content.
func readBack(t *testing.T, svc Service, key string, content []byte) {
	t.Helper()
	body, contentType, err := svc.Download(context.Background(), key)
	if err != nil {
		t.Fatalf("Download(%s): %v", key, err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if !bytes.Equal(got, content) {
		t.Errorf("Download body = %q, want %q", got, content)
	}
	if contentType != "application/pdf" {
		t.Errorf("Download content type = %q, want application/pdf", contentType)
	}
}

func TestWriter_BackendStoresWhatTheServiceServes(t *testing.T) {
	stack, err := New(context.Background(), Config{Config: corestorage.Config{
		Type:           "local",
		LocalBaseDir:   t.TempDir(),
		LocalPublicURL: "http://localhost:8080",
		LocalPutSecret: "secret",
		PresignTTL:     15 * time.Minute,
	}}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	content := []byte("%PDF-1.4 payment slip")
	key, err := stack.Writer.Store(context.Background(), "slpa-payment-slip.pdf", content, "application/pdf")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !storedKey.MatchString(key) {
		t.Fatalf("Store key = %q, want a uuid .pdf key", key)
	}
	readBack(t, stack.Service, key, content)
}

// In proxy mode the owning service allocates the key and accepts the bytes
// only at the URL it presigns, so Store does what a client would.
func TestWriter_ProxyStoresThroughTheOwningService(t *testing.T) {
	stack := newProxyStack(t, ownerToken)

	content := []byte("%PDF-1.4 gate pass")
	key, err := stack.Writer.Store(context.Background(), "slpa-gate-pass.pdf", content, "application/pdf")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !storedKey.MatchString(key) {
		t.Fatalf("Store key = %q, want the owning service's uuid .pdf key", key)
	}
	readBack(t, stack.Service, key, content)
}

func TestWriter_ProxyReportsAnOwnerThatRefuses(t *testing.T) {
	stack := newProxyStack(t, "wrong-token")

	if _, err := stack.Writer.Store(context.Background(), "slpa-gate-pass.pdf", []byte("%PDF-1.4"), "application/pdf"); err == nil {
		t.Fatal("Store: expected an error when the owning service refuses our credentials")
	}
}
