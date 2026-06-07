package trade

import (
	"encoding/json"
	"fmt"

	flowplugins "github.com/OpenNSW/nsw-task-flow/plugins"
	"github.com/OpenNSW/nsw/backend/internal/consignment"
	"github.com/OpenNSW/nsw/backend/internal/profile/cha"
	"gorm.io/gorm"
)

// CHAPersistPlugin is a synchronous SYSTEM-task plugin that writes the CHA selected
// during trade_1_cha_selection back onto the consignment row (cha_id, cha_company_id).
//
// Direct-start consignments (e.g. trade-export-v1) are created with a nil CHA, and CHA
// selection happens inside the workflow as the traderinput.cha_id/trade.cha_id workflow
// variable — never persisted to the consignments table. Without this write-back, the
// existing CHA-role visibility filter (which queries consignments by cha_company_id)
// would never match these consignments. This plugin runs immediately after CHA selection
// completes, resolves the chosen CHA's company, and updates the row.
//
// It is synchronous — it returns nil (not ErrSuspended) so the engine advances
// immediately without waiting for any user or external action.
type CHAPersistPlugin struct {
	db         *gorm.DB
	chaService cha.Service
}

// NewCHAPersistPlugin creates a CHAPersistPlugin backed by db and chaService.
func NewCHAPersistPlugin(db *gorm.DB, chaService cha.Service) *CHAPersistPlugin {
	return &CHAPersistPlugin{db: db, chaService: chaService}
}

func (p *CHAPersistPlugin) Execute(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	chaID, ok := ctx.Inputs["cha_id"].(string)
	if !ok || chaID == "" {
		return fmt.Errorf("cha_persist: cha_id not found in inputs")
	}

	record, err := p.chaService.GetByID(ctx.Context, chaID)
	if err != nil {
		return fmt.Errorf("cha_persist: failed to look up CHA %q: %w", chaID, err)
	}

	if ctx.Record == nil {
		return fmt.Errorf("cha_persist: task record is nil")
	}
	// ParentWorkflowID is the root workflow's ID, which equals the consignment ID
	// for any task running outside a SPLIT_TASK branch (see taskv2/store/model.go).
	consignmentID := ctx.Record.ParentWorkflowID
	if consignmentID == "" {
		return fmt.Errorf("cha_persist: parent workflow id is empty")
	}

	if err := p.db.WithContext(ctx.Context).
		Model(&consignment.Consignment{}).
		Where("id = ?", consignmentID).
		Updates(map[string]any{"cha_id": chaID, "cha_company_id": record.CompanyID}).Error; err != nil {
		return fmt.Errorf("cha_persist: failed to update consignment %q: %w", consignmentID, err)
	}

	return nil
}
