package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	corestorage "github.com/OpenNSW/core/storage"
)

// uploadPDF uploads a pdf of size bytes through stack and returns the value to
// store for it.
func uploadPDF(t *testing.T, stack *Stack, size int) string {
	t.Helper()
	meta, err := stack.Service.Upload(context.Background(), "sample.pdf", int64(size), "application/pdf")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	putFile(t, meta.UploadURL, "application/pdf", size)
	return meta.Key
}

func TestFiles_Backend(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	stack, err := New(context.Background(), localConfig(t, srv.URL), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stack.LocalContent.RegisterRoutes(mux)
	ctx := context.Background()

	key := uploadPDF(t, stack, 15)
	got, err := stack.Files.Describe(ctx, key)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if want := (File{Extension: ".pdf", MediaType: "application/pdf"}); got != want {
		t.Errorf("Describe = %+v, want %+v", got, want)
	}

	// A key whose upload was never made.
	meta, err := stack.Service.Upload(ctx, "sample.pdf", 10, "application/pdf")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if _, err := stack.Files.Describe(ctx, meta.Key); !errors.Is(err, corestorage.ErrNotFound) {
		t.Errorf("Describe(never uploaded) = %v, want ErrNotFound", err)
	}
}

// Behind a proxy the owning service's download route describes the file. The
// owner here answers that route as this application's FileHandler does.
func TestFiles_Proxy(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/storage/held" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"file not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"download_url":"https://files.example/k","expires_at":4102444800,"extension":".pdf","mime_type":"application/pdf"}`))
	}))
	t.Cleanup(owner.Close)
	stack, err := New(context.Background(), Config{
		Config: corestorage.Config{Type: TypeProxy},
		Proxy:  defaultProxyConfig(),
	}, newRegistry(t, owner.URL, ownerToken))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	got, err := stack.Files.Describe(ctx, "held")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if want := (File{Extension: ".pdf", MediaType: "application/pdf"}); got != want {
		t.Errorf("Describe = %+v, want %+v", got, want)
	}

	if _, err := stack.Files.Describe(ctx, "never-uploaded"); !errors.Is(err, corestorage.ErrNotFound) {
		t.Errorf("Describe(never uploaded) = %v, want ErrNotFound", err)
	}
}

// Any other failure of the owning service is not a missing file.
func TestFiles_ProxyOwnerFailureIsNotNotFound(t *testing.T) {
	stack := newProxyStack(t, "wrong-token")
	_, err := stack.Files.Describe(context.Background(), "3f2a1c9e-0000-4000-8000-000000000000.pdf")
	if err == nil || errors.Is(err, corestorage.ErrNotFound) {
		t.Errorf("Describe with refused credentials = %v, want an error other than ErrNotFound", err)
	}
	if !isUpstreamFailure(err) {
		t.Errorf("Describe with refused credentials = %v, want an upstream failure", err)
	}
}
