package config

import "fmt"

// Mode is the kind of system this deployment runs as. The modes are exclusive, not
// additive: each picks its own entry point, task ownership model and routes, so one
// process is either TNSW or an agency, never both. See docs/agency.md.
type Mode string

const (
	// ModeTNSW is the trade single window: traders create consignments, and trader/CHA
	// companies own their tasks.
	ModeTNSW Mode = "tnsw"
	// ModeAgency runs the backend as a government agency: external systems inject
	// workflows, which officers work grouped into cases.
	ModeAgency Mode = "agency"
)

// Validate reports whether m names a Mode. There is no default: a config.yaml
// must say which system it runs, so an agency cannot start as TNSW by omission.
func (m Mode) Validate() error {
	switch m {
	case ModeTNSW, ModeAgency:
		return nil
	case "":
		return fmt.Errorf("invalid mode: mode is required (%q or %q)", ModeTNSW, ModeAgency)
	default:
		return fmt.Errorf("invalid mode %q: must be %q or %q", string(m), ModeTNSW, ModeAgency)
	}
}
