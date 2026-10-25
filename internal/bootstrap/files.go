package bootstrap

import (
	"context"
	"fmt"
	"slices"

	"github.com/OpenNSW/core/remote"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileref"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
	taskplugins "github.com/OpenNSW/nsw-srilanka/internal/tasks/plugins"
)

// fileServices is everything the application builds to keep and hand out
// files: the storage backend, the reader of any file a stored value names, the
// tokens callers hold in place of stored values, and what the plugins that send
// files to other services issue.
type fileServices struct {
	Stack  *nswstorage.Stack
	Files  *nswstorage.Files
	Access *fileaccess.Access
	OGA    taskplugins.OGAFiles
}

// newFileServices builds the file services from the storage settings, the
// catalog and the services registry rm holds, which services lists.
//
// The services the catalog's clientToService names are this deployment's storage
// peers: the only services whose files it opens. Each must be in the registry,
// where its URL and credentials are, and be nameable in a stored value, or
// building fails: a client that is no service could send files that nothing
// could open.
func newFileServices(ctx context.Context, storage nswstorage.Config, services []remote.ServiceConfig, cat *catalog.Catalog, rm *remote.Manager) (*fileServices, error) {
	peers, err := storagePeers(cat, services, rm)
	if err != nil {
		return nil, err
	}
	stack, err := nswstorage.New(ctx, storage)
	if err != nil {
		return nil, err
	}
	access, err := newFileAccess(storage, cat.ServiceFor)
	if err != nil {
		return nil, fmt.Errorf("file tokens: %w", err)
	}

	// Every deployment issues the file tokens a peer is sent: the client each
	// service calls back as is the one its clientToService entry names, and
	// the token lets that client open the file and nothing more.
	oga := taskplugins.OGAFiles{
		ClientFor: cat.ClientFor,
		// A peer opens a file it is sent; it never submits one back.
		IssueForClient: func(clientID, value string) (string, error) {
			return access.IssueForClient(clientID, value, fileaccess.ScopeRead)
		},
	}
	return &fileServices{
		Stack:  stack,
		Files:  nswstorage.NewFiles(stack.Service, rm, peers, storage.InlineTypes),
		Access: access,
		OGA:    oga,
	}, nil
}

// storagePeers returns the storage peers of a deployment, each service with the
// base URL it is registered under, after checking every service the catalog's
// clientToService names is one.
func storagePeers(cat *catalog.Catalog, services []remote.ServiceConfig, rm *remote.Manager) (map[string]string, error) {
	registered := rm.ListServices()
	peers := make(map[string]string, len(cat.ClientToService))
	for client, service := range cat.ClientToService {
		if !fileref.ValidNamespace(service) {
			return nil, fmt.Errorf("catalog clientToService[%q]: %q cannot name a service in a stored file value: it is a letter followed by letters, digits, _ or -", client, service)
		}
		if !slices.Contains(registered, service) {
			return nil, fmt.Errorf("catalog clientToService[%q]: service %q is not in the services registry", client, service)
		}
		for _, s := range services {
			if s.ID == service {
				peers[service] = s.URL
			}
		}
	}
	return peers, nil
}

// newFileAccess builds the file tokens this deployment issues from its storage
// settings. serviceFor says which service each machine client is.
func newFileAccess(cfg nswstorage.Config, serviceFor func(clientID string) (string, bool)) (*fileaccess.Access, error) {
	keys, err := filetoken.ParseKeyset(cfg.TokenKeyset)
	if err != nil {
		return nil, fmt.Errorf("storage.tokenKeyset: %w", err)
	}
	codec, err := filetoken.NewCodec(keys, nil)
	if err != nil {
		return nil, err
	}
	return fileaccess.New(codec, cfg.TokenTTL(), fileaccess.WithClientServices(serviceFor))
}
