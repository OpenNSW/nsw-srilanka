package bootstrap

import (
	"slices"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

func TestInternalUploadTypesByClient(t *testing.T) {
	cat := &catalog.Catalog{Clients: map[string]string{"npqs": "NPQS_TO_NSW", "cda": "CDA_TO_NSW"}}

	got, err := internalUploadTypesByClient(nswstorage.Config{AllowedInternalUploadTypes: map[string][]string{"npqs": {"text/html"}}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"text/html"}; !slices.Equal(got["NPQS_TO_NSW"], want) || len(got) != 1 {
		t.Errorf("types by client = %v, want NPQS_TO_NSW: %v", got, want)
	}

	if got, err := internalUploadTypesByClient(nswstorage.Config{}, cat); err != nil || len(got) != 0 {
		t.Errorf("with none configured: %v, %v, want an empty map", got, err)
	}

	_, err = internalUploadTypesByClient(nswstorage.Config{AllowedInternalUploadTypes: map[string][]string{"nobody": {"text/html"}}}, cat)
	if err == nil || !strings.Contains(err.Error(), `"nobody" is not a client in the catalog`) {
		t.Errorf("an unknown client: %v, want an error naming it", err)
	}

	// A proxy serves no internal upload route, so it reads none.
	proxy := nswstorage.Config{AllowedInternalUploadTypes: map[string][]string{"nobody": {"text/html"}}}
	proxy.Type = nswstorage.TypeProxy
	if got, err := internalUploadTypesByClient(proxy, cat); err != nil || len(got) != 0 {
		t.Errorf("in proxy mode: %v, %v, want an empty map", got, err)
	}
}
