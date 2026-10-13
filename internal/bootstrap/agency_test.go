package bootstrap

import "testing"

// An agency's catalog requires no particular role: the gate owns whichever roles the
// catalog maps, so a catalog without an officer role still builds.
func TestNewAgencyTaskGate(t *testing.T) {
	for name, roles := range map[string]map[string]string{
		"officer only":       {"officer": "Officer"},
		"officer + reviewer": {"officer": "Officer", "reviewer": "Reviewer"},
		"no officer":         {"reviewer": "Reviewer"},
	} {
		gate, err := newAgencyTaskGate(nil, roles)
		if err != nil || gate == nil {
			t.Errorf("%s: gate = %v, err = %v, want a gate", name, gate, err)
		}
	}
}
