package config

import "fmt"

// Mode is the kind of system this deployment runs as. The modes are exclusive, not
// additive: each picks its own entry point, task ownership model and routes, so one
// process is either TNSW or an agency, never both. See docs/agency.md.
type Mode string

const (
	// ModeTNSW is the trade single window: traders create consignments, and trader/CHA
	// companies own their tasks. It is the default.
	ModeTNSW Mode = "tnsw"
	// ModeAgency runs the backend as a government agency: external systems inject
	// workflows, which officers work grouped into cases.
	ModeAgency Mode = "agency"
)

// parseMode returns the Mode named by s, defaulting to ModeTNSW when s is empty so a
// config.yaml without a mode key keeps running as TNSW.
func parseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeTNSW:
		return ModeTNSW, nil
	case ModeAgency:
		return ModeAgency, nil
	default:
		return "", fmt.Errorf("invalid mode %q: must be %q or %q", s, ModeTNSW, ModeAgency)
	}
}
