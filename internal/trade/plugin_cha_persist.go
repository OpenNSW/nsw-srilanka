package trade

import (
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw/backend/srilanka/internal/consignment"
	"github.com/OpenNSW/nsw/backend/internal/profile/company"
	"gorm.io/gorm"
)

// CHAPersistPlugin is a synchronous SYSTEM-task plugin that writes the CHA company selected
// during trade_1_cha_selection back onto the consignment row (cha_company_id).
//
// Direct-start consignments (e.g. trade-export-v1) are created with a nil CHA company, and
// CHA selection happens inside the workflow as the traderinput.cha_company_id/trade.cha_company_id
// workflow variable — never persisted to the consignments table. Without this write-back, the
// existing CHA-role visibility filter (which queries consignments by cha_company_id) would
// never match these consignments. This plugin runs immediately after CHA selection completes,
// validates the chosen company is CHA-enabled, and updates the row.
//
// Access to the consignment is company-wide — any user at the selected CHA company can act on
// it — so only cha_company_id is recorded; no individual CHA user is assigned.
//
// It is synchronous — it returns nil (not ErrSuspended) so the engine advances
// immediately without waiting for any user or external action.
type CHAPersistPlugin struct {
	db             *gorm.DB
	companyService company.Service
}

// NewCHAPersistPlugin creates a CHAPersistPlugin backed by db and companyService.
func NewCHAPersistPlugin(db *gorm.DB, companyService company.Service) *CHAPersistPlugin {
	if db == nil {
		panic("db is nil")
	}
	if companyService == nil {
		panic("companyService is nil")
	}
	return &CHAPersistPlugin{db: db, companyService: companyService}
}

func (p *CHAPersistPlugin) Execute(ctx plugins.PluginContext, _ json.RawMessage) error {
	chaCompanyID, ok := ctx.Inputs["cha_company_id"].(string)
	if !ok || chaCompanyID == "" {
		return fmt.Errorf("cha_persist: cha_company_id not found in inputs")
	}

	record, err := p.companyService.GetCompanyByID(ctx.Context, chaCompanyID)
	if err != nil {
		return fmt.Errorf("cha_persist: failed to look up CHA company %q: %w", chaCompanyID, err)
	}
	if !record.HasCHA {
		return fmt.Errorf("cha_persist: company %q is not enabled as a CHA", chaCompanyID)
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

	result := p.db.WithContext(ctx.Context).
		Model(&consignment.Consignment{}).
		Where("id = ?", consignmentID).
		Updates(map[string]any{"cha_company_id": chaCompanyID})
	if result.Error != nil {
		return fmt.Errorf("cha_persist: failed to update consignment %q: %w", consignmentID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("cha_persist: consignment %q not found", consignmentID)
	}

	return nil
}
