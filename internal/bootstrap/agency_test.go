package bootstrap

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

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

// A catalog with no role starts but admits nobody, so building the gate says so.
func TestNewAgencyTaskGateWarnsWithoutRoles(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if _, err := newAgencyTaskGate(nil, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "agency catalog maps no roles") {
		t.Errorf("logs = %q, want a no-roles warning", logs.String())
	}

	logs.Reset()
	if _, err := newAgencyTaskGate(nil, map[string]string{"officer": "Officer"}); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Errorf("logs = %q, want none for a catalog with a role", logs.String())
	}
}
