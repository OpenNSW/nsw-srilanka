package autogen

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry stores and manages reference ID format specifications and controlled parameter lists.
type Registry struct {
	mu      sync.RWMutex
	lists   map[string]map[string]bool
	formats map[string]map[string]FormatConfig // issuer -> idType -> FormatConfig
}

// NewRegistry initializes an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		lists:   make(map[string]map[string]bool),
		formats: make(map[string]map[string]FormatConfig),
	}
}

// LoadFromYAML parses YAML content into the registry.
func (r *Registry) LoadFromYAML(data []byte) error {
	var cfg RegistryConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("autogen registry: unmarshal yaml: %w", err)
	}
	return r.LoadConfig(cfg)
}

// LoadConfig populates the registry from a RegistryConfig structure.
func (r *Registry) LoadConfig(cfg RegistryConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newLists := make(map[string]map[string]bool)
	for name, values := range cfg.Lists {
		valMap := make(map[string]bool, len(values))
		for _, v := range values {
			valMap[strings.TrimSpace(v)] = true
		}
		newLists[strings.TrimSpace(name)] = valMap
	}

	newFormats := make(map[string]map[string]FormatConfig)
	for _, iss := range cfg.Issuers {
		issuerKey := strings.ToUpper(strings.TrimSpace(iss.Issuer))
		if issuerKey == "" {
			continue
		}
		if _, ok := newFormats[issuerKey]; !ok {
			newFormats[issuerKey] = make(map[string]FormatConfig)
		}
		for _, fmtCfg := range iss.Formats {
			idTypeKey := strings.ToLower(strings.TrimSpace(fmtCfg.IDType))
			if idTypeKey == "" {
				continue
			}
			newFormats[issuerKey][idTypeKey] = fmtCfg
		}
	}

	r.lists = newLists
	r.formats = newFormats
	return nil
}

// GetFormat retrieves a FormatConfig by issuer and idType.
func (r *Registry) GetFormat(issuer, idType string) (FormatConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	issuerKey := strings.ToUpper(strings.TrimSpace(issuer))
	idTypeKey := strings.ToLower(strings.TrimSpace(idType))

	if issMap, ok := r.formats[issuerKey]; ok {
		fmtCfg, found := issMap[idTypeKey]
		return fmtCfg, found
	}
	return FormatConfig{}, false
}

// ValidateListParam checks if a value is present in a registered controlled list.
func (r *Registry) ValidateListParam(listName, value string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listName = strings.TrimSpace(listName)
	valMap, exists := r.lists[listName]
	if !exists {
		return fmt.Errorf("autogen registry: controlled list %q is not defined", listName)
	}

	value = strings.TrimSpace(value)
	if !valMap[value] {
		return fmt.Errorf("autogen registry: value %q is not valid for list %q", value, listName)
	}
	return nil
}
