package replay_e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/OpenNSW/core/temporal"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

const signalPollInterval = 300 * time.Millisecond

// temporalSignaller delivers Temporal signals to every child of a parent flow.
// It stands in for a sibling branch's SIGNALING EMIT (e.g. Customs emitting
// cusdec_assessed) so agency replay flows can advance past WAIT nodes without
// driving the full CusDec path.
type temporalSignaller struct {
	client client.Client
	logf   func(string, ...any)
}

func newTemporalSignaller(t *testing.T, cfg *config.Config) *temporalSignaller {
	t.Helper()
	c, err := temporal.NewClient(cfg.Temporal)
	if err != nil {
		t.Fatalf("signaller: temporal client: %v", err)
	}
	t.Cleanup(c.Close)
	return &temporalSignaller{client: c, logf: t.Logf}
}

// BroadcastToChildren implements replay.Signaller.
func (s *temporalSignaller) BroadcastToChildren(ctx context.Context, parentWorkflowID, name string, payload map[string]any, timeout time.Duration) error {
	if payload == nil {
		payload = map[string]any{}
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(signalPollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		children, err := s.childWorkflowIDs(ctx, parentWorkflowID)
		if err != nil {
			lastErr = err
		} else if len(children) > 0 {
			signaled := 0
			for _, id := range children {
				if err := s.client.SignalWorkflow(ctx, id, "", name, payload); err != nil {
					lastErr = fmt.Errorf("signal %q to %s: %w", name, id, err)
					continue
				}
				signaled++
				s.logf("signaller: delivered %q to child %s", name, id)
			}
			if signaled > 0 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("signaller: no child of %q accepted %q within %s: %w", parentWorkflowID, name, timeout, lastErr)
			}
			return fmt.Errorf("signaller: parent %q had no child workflows to signal with %q within %s", parentWorkflowID, name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *temporalSignaller) childWorkflowIDs(ctx context.Context, parentWorkflowID string) ([]string, error) {
	val, err := s.client.QueryWorkflow(ctx, parentWorkflowID, "", "GetStatus")
	if err != nil {
		return nil, err
	}
	var instance workflow.WorkflowInstance
	if err := val.Get(&instance); err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, n := range instance.NodeInfo {
		if n != nil {
			out = append(out, n.ChildWorkflowIDs...)
		}
	}
	return out, nil
}
