package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
)

// TaskTypeCustomsCDNCancel records that the trader cancelled a container's
// dispatch note branch after ASYCUDA registered the note, while it waited for
// Customs to acknowledge it. NSW has no call to cancel a registered note in
// ASYCUDA World, so the note stays registered there; here it is marked
// cancelled, so an acknowledgment (§7.3) or cancellation (§7.7) Customs still
// sends for it is acknowledged without action.
const TaskTypeCustomsCDNCancel = "CUSTOMS_CDN_CANCEL"

// CDNNotes reads and updates dispatch notes. cdn's DispatchNoteRepository
// satisfies it.
type CDNNotes interface {
	GetByEdgeID(ctx context.Context, edgeID string) (*cdn.DispatchNote, error)
	Update(ctx context.Context, note *cdn.DispatchNote) error
}

// CDNCancelPlugin marks the note SLC Edge accepted as the edge_id input
// cancelled, and records under its output namespace the ack_status a branch
// cancelled by Customs finishes with (CANCELLED), so the steps after the split
// leave the branch out as they do one Customs cancelled.
type CDNCancelPlugin struct {
	notes CDNNotes
}

// NewCDNCancelPlugin returns the plugin, reading and updating notes in notes.
func NewCDNCancelPlugin(notes CDNNotes) *CDNCancelPlugin {
	return &CDNCancelPlugin{notes: notes}
}

// Execute marks the note cancelled when it is still waiting on Customs. A
// branch with no note recorded, or one already acknowledged or cancelled, is
// left as it is; the branch is cancelled either way. Only a failed read or
// write is an error.
func (p *CDNCancelPlugin) Execute(ctx pluginContext, _ json.RawMessage) error {
	if ctx.Record == nil {
		return fmt.Errorf("customs_cdn_cancel: task record is nil")
	}
	if ctx.OutputNamespace == "" {
		return fmt.Errorf("customs_cdn_cancel: an output namespace is required")
	}
	if edgeID, _ := ctx.Inputs["edge_id"].(string); edgeID != "" {
		if err := p.cancel(ctx.Context, edgeID); err != nil {
			return err
		}
	} else {
		slog.Warn("customs_cdn_cancel: the branch names no dispatch note; nothing to mark", "taskId", ctx.Record.TaskID)
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{"ack_status": string(cdn.DispatchNoteStatusCancelled)}
	return nil
}

func (p *CDNCancelPlugin) cancel(ctx context.Context, edgeID string) error {
	note, err := p.notes.GetByEdgeID(ctx, edgeID)
	if err != nil {
		return fmt.Errorf("customs_cdn_cancel: read dispatch note %s: %w", edgeID, err)
	}
	if note == nil {
		slog.Warn("customs_cdn_cancel: no dispatch note recorded for the branch; nothing to mark", "edge_id", edgeID)
		return nil
	}
	if note.Status != cdn.DispatchNoteStatusSubmitted && note.Status != cdn.DispatchNoteStatusIntegrated {
		slog.Info("customs_cdn_cancel: dispatch note no longer waiting on Customs; left as it is",
			"edge_id", edgeID, "status", note.Status)
		return nil
	}
	note.Status = cdn.DispatchNoteStatusCancelled
	if err := p.notes.Update(ctx, note); err != nil {
		return fmt.Errorf("customs_cdn_cancel: record dispatch note %s as cancelled: %w", edgeID, err)
	}
	slog.Info("customs_cdn_cancel: trader cancelled the dispatch note", "edge_id", edgeID)
	return nil
}
