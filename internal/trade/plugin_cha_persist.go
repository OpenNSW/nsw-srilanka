package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"gorm.io/gorm"
)

// CHAPersistPlugin is a synchronous SYSTEM-task plugin that writes the CHA company selected
// during trade_1_cha_selection back onto the consignment row (cha_company_id).
//
// Direct-start consignments (e.g. trade-export) are created with a nil CHA company, and
// CHA selection happens inside the workflow as the traderinput.cha_company_id/trade.cha_company_id
// workflow variable — never persisted to the consignments table. Without this write-back, the
// existing CHA-role visibility filter (which queries consignments by cha_company_id) would
// never match these consignments. This plugin runs immediately after CHA selection completes,
// validates the chosen company is CHA-enabled, and updates the row.
//
// Access to the consignment is company-wide — any user at the selected CHA company can act on
// it — so only cha_company_id is recorded; no individual CHA user is assigned.
//
// It also hands the chosen CHA's details on as the task's "cha" output (see chaSeed), so the
// agency flows can prefill the declarant's details from them. Which of the company's data
// fields go with them is the task template's choice (see chaPersistConfig).
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

func (p *CHAPersistPlugin) Execute(ctx plugins.PluginContext, config json.RawMessage) error {
	cfg, err := parseCHAPersistConfig(config)
	if err != nil {
		return err
	}

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
	// Read before anything is written, so unreadable data fails the step with nothing saved.
	cha, err := chaSeed(record, cfg.CHADataFields)
	if err != nil {
		return fmt.Errorf("cha_persist: CHA company %q: %w", chaCompanyID, err)
	}

	if ctx.Record == nil {
		return fmt.Errorf("cha_persist: task record is nil")
	}
	// Every flow of a consignment runs with the consignment ID as its case ID, at any depth
	// of nesting (see internal/caseflow).
	consignmentID := ctx.Record.CaseID
	if consignmentID == "" {
		return fmt.Errorf("cha_persist: case id is empty")
	}

	name, _ := ctx.Inputs["consignment_name"].(string)
	if strings.TrimSpace(name) == "" {
		name = ""
	}
	if len([]rune(name)) > 255 {
		return fmt.Errorf("cha_persist: consignment name exceeds maximum length of 255 characters")
	}

	if err := p.assignCHA(ctx.Context, consignmentID, chaCompanyID, name); err != nil {
		return err
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data["cha"] = cha
	return nil
}

// assignCHA records the CHA company on the consignment and, when name is not empty, names its
// case, in one transaction. The name lives on the consignment's cases row; the CHA company on
// the consignment.
func (p *CHAPersistPlugin) assignCHA(ctx context.Context, consignmentID, chaCompanyID, name string) error {
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&consignment.Consignment{}).
			Where("id = ?", consignmentID).
			Update("cha_company_id", chaCompanyID)
		if result.Error != nil {
			return fmt.Errorf("cha_persist: failed to update consignment %q: %w", consignmentID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("cha_persist: consignment %q not found", consignmentID)
		}
		if name == "" {
			return nil
		}
		if err := tx.Exec(`UPDATE cases SET name = ?, updated_at = now() WHERE id = ?`, name, consignmentID).Error; err != nil {
			return fmt.Errorf("cha_persist: failed to name consignment %q: %w", consignmentID, err)
		}
		return nil
	})
}

// chaPersistConfig is the plugin_properties of the task template that runs CHAPersistPlugin.
type chaPersistConfig struct {
	// CHADataFields are the company data fields the CHA's details carry into the agency flows,
	// e.g. ["tin_no", "tel"]. A company's data holds more, such as the SLPA CMS key it was
	// issued, which must not reach a trader's workflow, so this is an allowlist: with none
	// listed, the details carry only the CHA's id and name. Listing a field here is all it
	// takes for a flow to read another one.
	CHADataFields []string `json:"cha_data_fields"`
}

// parseCHAPersistConfig reads the plugin's config. No config, {} and null all mean no data
// fields; a field list that isn't strings, or holds a blank one, is an error.
func parseCHAPersistConfig(config json.RawMessage) (chaPersistConfig, error) {
	var cfg chaPersistConfig
	if len(config) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return cfg, fmt.Errorf("cha_persist: invalid plugin config: %w", err)
	}
	for i, field := range cfg.CHADataFields {
		if strings.TrimSpace(field) == "" {
			return cfg, fmt.Errorf("cha_persist: invalid plugin config: cha_data_fields[%d] is blank", i)
		}
	}
	return cfg, nil
}

// chaSeed is the CHA's details as the "cha" workflow variable, in the shape of a company
// record ({id, name, data}) so an artifact reads them the way it reads the trader's. data
// holds the listed fields whose value is a string. Fields the company's data lacks, or holds
// only whitespace in, are left out. No data at all counts as {}, as in
// company.Service.ReplaceCompanyData; data that isn't a JSON object, null included, is an
// error.
func chaSeed(record *company.Record, fields []string) (map[string]any, error) {
	data := map[string]any{}
	var all map[string]any
	if len(record.Data) > 0 {
		if err := json.Unmarshal(record.Data, &all); err != nil {
			return nil, fmt.Errorf("data is not a JSON object: %w", err)
		}
		// null decodes into a nil map without an error.
		if all == nil {
			return nil, fmt.Errorf("data is null, not a JSON object")
		}
	}
	for _, field := range fields {
		// Company data isn't trimmed when it's written, so a blank value can hold spaces.
		if value, ok := all[field].(string); ok && strings.TrimSpace(value) != "" {
			data[field] = value
		}
	}
	return map[string]any{"id": record.ID, "name": record.Name, "data": data}, nil
}
