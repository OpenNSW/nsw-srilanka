package bootstrap

import (
	"fmt"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// internalUploadTypesByClient turns storage.allowedInternalUploadTypes, keyed by
// a client's name in the global catalog, into the types by client ID. A name the
// catalog does not list is an error: the types would be granted to no one. A
// proxy deployment serves no internal upload route, so it reads none.
func internalUploadTypesByClient(cfg nswstorage.Config, cat *catalog.Catalog) (map[string][]string, error) {
	if cfg.IsProxy() {
		return nil, nil
	}
	byClientID := make(map[string][]string, len(cfg.AllowedInternalUploadTypes))
	for name, types := range cfg.AllowedInternalUploadTypes {
		clientID, ok := cat.Clients[name]
		if !ok {
			return nil, fmt.Errorf("storage.allowedInternalUploadTypes: %q is not a client in the catalog", name)
		}
		byClientID[clientID] = types
	}
	return byClientID, nil
}
