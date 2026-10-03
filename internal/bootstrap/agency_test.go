package bootstrap

import (
	"strings"
	"testing"
)

// An agency's catalog only has to map the officer role, and must: it is the only
// role anyone can act in on an agency's tasks.
func TestNewAgencyTaskGate(t *testing.T) {
	if _, err := newAgencyTaskGate(nil, map[string]string{"officer": "Officer"}); err != nil {
		t.Fatalf("officer-only catalog: %v", err)
	}
	_, err := newAgencyTaskGate(nil, map[string]string{"trader": "Trader", "cha": "CHA"})
	if err == nil || !strings.Contains(err.Error(), "officer") {
		t.Fatalf("catalog without officer: err = %v, want missing officer", err)
	}
}
