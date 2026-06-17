package replay_e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenNSW/nsw-srilanka/internal/replay"
)

// TestReplay_TradeUpToHSCode runs the trade-export flow through CHA selection
// and HS-code selection, asserting the agency split fires (the FCAU application
// task appears). This is the first replay-driven flow; it touches no external
// service.
func TestReplay_TradeUpToHSCode(t *testing.T) {
	skipUnlessE2E(t)
	h := newHarness(t)
	runFlow(t, h, "trade_up_to_hscode.json")
}

// runFlow loads a flow file from flows/ and executes it against the harness.
func runFlow(t *testing.T, h *harness, file string) {
	t.Helper()
	flowPath := filepath.Join(findRepoRoot(), "integration", "replay", "flows", file)
	flow, err := replay.LoadFlow(flowPath)
	if err != nil {
		t.Fatalf("load flow %s: %v", file, err)
	}

	r := replay.New(h.server.URL, h.client)
	r.Logf = t.Logf

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := r.Run(ctx, flow); err != nil {
		t.Fatalf("flow %q failed: %v", flow.Name, err)
	}
}
