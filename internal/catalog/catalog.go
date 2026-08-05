// Package catalog holds the deployment's global catalog: the single source of
// truth mapping the stable logical names used across configuration onto one
// deployment's concrete identifiers — IdP token roles and OAuth2 client ids
// today, further sections later.
//
// It is loaded once at bootstrap (configs/catalog.json, CATALOG_CONFIG_PATH) and
// handed to consumers as data; no consumer reads the file itself. Unknown
// top-level keys are ignored, so the file may grow new sections ahead of the code
// that reads them.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Catalog is the parsed catalog file.
type Catalog struct {
	// Roles maps a logical principal name ("trader") to the IdP token role
	// carried in a user's access token ("Trader").
	Roles map[string]string `json:"roles"`
	// Clients maps a logical principal name ("fcau") to the OAuth2 client id
	// presented by an M2M caller ("FCAU_TO_NSW").
	Clients map[string]string `json:"clients"`
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

// Validate reports whether the catalog is internally consistent: it must define
// at least one entry, and every logical name must map to a non-empty identifier.
// Messages are unprefixed so Load can qualify them with the file path.
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
	return nil
}
