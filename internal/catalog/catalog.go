// Package catalog holds the deployment's global catalog: the single source of
// truth mapping the stable logical names used across configuration onto one
// deployment's concrete identifiers — IdP token roles and OAuth2 client ids
// today, further sections later — and says which service each machine client is.
//
// It is loaded once at bootstrap (configs/catalog.json, server.catalogConfigPath) and
// handed to consumers as data; no consumer reads the file itself. Unknown
// top-level keys are ignored, so the file may grow new sections ahead of the code
// that reads them.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Catalog is the parsed catalog file.
type Catalog struct {
	// Roles maps a logical principal name ("trader") to the IdP token role
	// carried in a user's access token ("Trader").
	Roles map[string]string `json:"roles"`
	// Clients maps a logical principal name ("fcau") to the OAuth2 client id
	// presented by an M2M caller ("FCAU_TO_NSW").
	Clients map[string]string `json:"clients"`
	// ClientToService maps a client's logical name in clients ("cda") to the id
	// of the service in config.yaml's services that client is ("cda"). It lists
	// the deployment's storage peers: only a listed client may send files, the
	// files it sends are stored as its service's, and a file sent to that
	// service is issued for its client.
	ClientToService map[string]string `json:"clientToService"`
}

// Load reads, parses, and validates the catalog file at path.
func Load(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("catalog: read %s: %w", path, err)
	}
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("catalog: parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("catalog %s: %w", path, err)
	}
	return &c, nil
}

// RequireRoles reports an error unless roles — the catalog's Roles map — defines
// every one of names with a non-empty token role.
//
// It exists because some callers depend on specific logical names rather than on
// whatever the catalog happens to define: consignment scopes queries by "trader"
// and "cha", and task ownership resolution maps each of those to its own database
// column. For them a missing name is not a narrower catalog but a silent denial of
// every request in that role, so they call this at construction to fail loudly
// instead. An empty mapping counts as missing: it can never match a real JWT role
// claim, and honoring it would let a comparison against an empty string succeed.
func RequireRoles(roles map[string]string, names ...string) error {
	var missing []string
	for _, name := range names {
		if role, ok := roles[name]; !ok || role == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("catalog is missing required role(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// ServiceFor returns the service the machine client clientID is, and whether
// clientToService lists it.
func (c *Catalog) ServiceFor(clientID string) (service string, ok bool) {
	if c == nil || clientID == "" {
		return "", false
	}
	for name, id := range c.Clients {
		if id != clientID {
			continue
		}
		if service, ok = c.ClientToService[name]; ok && service != "" {
			return service, true
		}
	}
	return "", false
}

// ClientFor returns the machine client id service calls this deployment as, and
// whether clientToService lists it.
func (c *Catalog) ClientFor(service string) (clientID string, ok bool) {
	if c == nil || service == "" {
		return "", false
	}
	for name, s := range c.ClientToService {
		if s == service {
			id := c.Clients[name]
			return id, id != ""
		}
	}
	return "", false
}

// Validate reports whether the catalog is internally consistent: it must define
// at least one entry, every logical name must map to a non-empty identifier, and
// clientToService must name only clients, each of a service of its own. Messages are unprefixed so Load can
// qualify them with the file path.
func (c *Catalog) Validate() error {
	if c == nil {
		return errors.New("nil catalog")
	}
	if len(c.Roles) == 0 && len(c.Clients) == 0 {
		return errors.New("defines no roles or clients")
	}
	for name, role := range c.Roles {
		if role == "" {
			return fmt.Errorf("roles[%q] must map to a non-empty token role", name)
		}
	}
	for name, clientID := range c.Clients {
		if clientID == "" {
			return fmt.Errorf("clients[%q] must map to a non-empty client id", name)
		}
	}
	return c.validateClientToService()
}

// validateClientToService checks clientToService names only clients, each with a
// service, and that no service or client id appears twice: both lookups, from a
// client to its service and from a service to its client, must have one answer.
func (c *Catalog) validateClientToService() error {
	services := make(map[string]string, len(c.ClientToService))
	clients := make(map[string]string, len(c.ClientToService))
	for name, service := range c.ClientToService {
		clientID, ok := c.Clients[name]
		if !ok {
			return fmt.Errorf("clientToService[%q] names a client that is not in clients", name)
		}
		if service == "" {
			return fmt.Errorf("clientToService[%q] must map to a non-empty service id", name)
		}
		if other, dup := services[service]; dup {
			return fmt.Errorf("clientToService maps both %q and %q to service %q", other, name, service)
		}
		services[service] = name
		if other, dup := clients[clientID]; dup {
			return fmt.Errorf("clients[%q] and clients[%q] are the same client id %q, so a file it sends has no one service", other, name, clientID)
		}
		clients[clientID] = name
	}
	return nil
}
