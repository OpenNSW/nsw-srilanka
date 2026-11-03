package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
)

type cdnNotes struct {
	byEdge  map[string]*cdn.DispatchNote
	updated []cdn.DispatchNote
	err     error
}

func (n *cdnNotes) GetByEdgeID(_ context.Context, edgeID string) (*cdn.DispatchNote, error) {
	return n.byEdge[edgeID], n.err
}

func (n *cdnNotes) Update(_ context.Context, note *cdn.DispatchNote) error {
	n.updated = append(n.updated, *note)
	return nil
}

func cdnCancelCtx(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context:         context.Background(),
		Record:          &store.TaskRecord{TaskID: "task-1"},
		Inputs:          inputs,
		OutputNamespace: "cdn_cancel",
	}
}

// A note waiting on Customs is marked cancelled, and the branch finishes as one
// Customs cancelled does.
func TestCDNCancel_MarksTheNoteCancelled(t *testing.T) {
	for _, status := range []cdn.DispatchNoteStatus{cdn.DispatchNoteStatusIntegrated, cdn.DispatchNoteStatusSubmitted} {
		t.Run(string(status), func(t *testing.T) {
			notes := &cdnNotes{byEdge: map[string]*cdn.DispatchNote{"edge-1": {EdgeID: "edge-1", Status: status}}}
			ctx := cdnCancelCtx(map[string]any{"edge_id": "edge-1"})

			require.NoError(t, NewCDNCancelPlugin(notes).Execute(ctx, nil))

			require.Len(t, notes.updated, 1)
			assert.Equal(t, cdn.DispatchNoteStatusCancelled, notes.updated[0].Status)
			assert.Equal(t, map[string]any{"ack_status": "CANCELLED"}, ctx.Record.Data["cdn_cancel"])
		})
	}
}

// A note Customs already acknowledged or cancelled, or none at all, is left as
// it is; the branch still finishes cancelled.
func TestCDNCancel_LeavesANoteNoLongerWaiting(t *testing.T) {
	for name, inputs := range map[string]map[string]any{
		"acknowledged": {"edge_id": "edge-ack"},
		"not recorded": {"edge_id": "edge-unknown"},
		"no edge id":   {},
	} {
		t.Run(name, func(t *testing.T) {
			notes := &cdnNotes{byEdge: map[string]*cdn.DispatchNote{"edge-ack": {EdgeID: "edge-ack", Status: cdn.DispatchNoteStatusAcknowledged}}}
			ctx := cdnCancelCtx(inputs)

			require.NoError(t, NewCDNCancelPlugin(notes).Execute(ctx, nil))

			assert.Empty(t, notes.updated)
			assert.Equal(t, map[string]any{"ack_status": "CANCELLED"}, ctx.Record.Data["cdn_cancel"])
		})
	}
}

func TestCDNCancel_ReportsAFailedRead(t *testing.T) {
	ctx := cdnCancelCtx(map[string]any{"edge_id": "edge-1"})

	err := NewCDNCancelPlugin(&cdnNotes{err: errors.New("connection refused")}).Execute(ctx, nil)

	require.Error(t, err)
	assert.Nil(t, ctx.Record.Data)
}
