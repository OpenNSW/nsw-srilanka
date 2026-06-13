package consignment

import (
	"fmt"
	"time"

	"github.com/OpenNSW/core/pagination"
)

// ── Domain types ──────────────────────────────────────────────────────────────

// Flow represents the flow type of consignment.
// Keep values in sync with workflow/model.ConsignmentFlow — the workflow
// package keeps its own copy to avoid importing this package.
type Flow string

const (
	FlowImport Flow = "IMPORT"
	FlowExport Flow = "EXPORT"
)

// State represents the lifecycle state of a consignment.
type State string

const (
	InProgress State = "IN_PROGRESS"
	Finished   State = "FINISHED"
)

// ── Entity ────────────────────────────────────────────────────────────────────

// Consignment represents a consignment in the system.
type Consignment struct {
	ID        string    `gorm:"type:text;column:id;primaryKey;not null" json:"id"`
	CreatedAt time.Time `gorm:"type:timestamptz;column:created_at;not null;autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"type:timestamptz;column:updated_at;not null;autoUpdateTime" json:"updatedAt"`

	// Core attributes
	Flow  Flow  `gorm:"type:varchar(50);column:flow;not null" json:"flow"`   // IMPORT or EXPORT
	State State `gorm:"type:varchar(50);column:state;not null" json:"state"` // IN_PROGRESS → FINISHED

	// Trader
	TraderID        string `gorm:"type:varchar(100);column:trader_id;not null" json:"traderId"`                // Trader user who created the consignment
	TraderCompanyID string `gorm:"type:varchar(100);column:trader_company_id;not null" json:"traderCompanyId"` // Company the trader belongs to

	// CHA (nil for direct-start consignments where CHA selection happens inside the workflow)
	CHACompanyID *string `gorm:"type:varchar(100);column:cha_company_id" json:"chaCompanyId,omitempty"` // CHA company selected by the trader
	CHAID        *string `gorm:"type:varchar(100);column:cha_id" json:"chaId,omitempty"`                // CHA who claimed the consignment
}

func (c *Consignment) TableName() string {
	return "consignments"
}

// ── Request DTOs ──────────────────────────────────────────────────────────────

// CreateConsignmentDTO represents the request body for POST /consignments.
type CreateConsignmentDTO struct {
	Flow         Flow   `json:"flow"`
	ChaCompanyID string `json:"chaCompanyId"`
}

func (d *CreateConsignmentDTO) Validate() error {
	if d.ChaCompanyID == "" {
		return fmt.Errorf("chaCompanyId is required")
	}
	if d.Flow != FlowImport && d.Flow != FlowExport {
		return fmt.Errorf("flow must be IMPORT or EXPORT")
	}
	return nil
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

// SummaryDTO represents the consignment data returned in list responses.
type SummaryDTO struct {
	ID              string `json:"id"`              // Consignment ID
	Flow            Flow   `json:"flow"`            // e.g., IMPORT, EXPORT
	State           State  `json:"state"`           // State of the consignment
	TraderID        string `json:"traderId"`        // Trader user who created the consignment
	TraderCompanyID string `json:"traderCompanyId"` // Company the trader belongs to
	ChaCompanyID    string `json:"chaCompanyId"`    // CHA company selected at Stage 1
	ChaID           string `json:"chaId,omitempty"` // CHA assigned at Stage 2 (empty until claimed)
	CreatedAt       string `json:"createdAt"`       // Timestamp of consignment creation
	UpdatedAt       string `json:"updatedAt"`       // Timestamp of last consignment update
}

// ListResult is the pagination envelope returned by the list consignments endpoint.
type ListResult = pagination.Page[SummaryDTO]

// DetailDTO represents the full consignment data returned in detailed responses.
type DetailDTO struct {
	ID              string                    `json:"id"`              // Consignment ID
	Flow            Flow                      `json:"flow"`            // e.g., IMPORT, EXPORT
	State           State                     `json:"state"`           // State of the consignment
	TraderID        string                    `json:"traderId"`        // Trader user who created the consignment
	TraderCompanyID string                    `json:"traderCompanyId"` // Company the trader belongs to
	ChaCompanyID    string                    `json:"chaCompanyId"`    // CHA company selected at Stage 1
	ChaID           string                    `json:"chaId,omitempty"` // CHA assigned at Stage 2 (empty until claimed)
	CreatedAt       string                    `json:"createdAt"`       // Timestamp of consignment creation
	UpdatedAt       string                    `json:"updatedAt"`       // Timestamp of last consignment update
	WorkflowNodes   []WorkflowNodeResponseDTO `json:"workflowNodes"`   // Associated workflow nodes with template details
}

// WorkflowNodeResponseDTO represents a workflow node in the response.
type WorkflowNodeResponseDTO struct {
	ID                   string                          `json:"id"`                   // Workflow Node ID
	CreatedAt            string                          `json:"createdAt"`            // Timestamp of node creation
	UpdatedAt            string                          `json:"updatedAt"`            // Timestamp of last node update
	WorkflowNodeTemplate WorkflowNodeTemplateResponseDTO `json:"workflowNodeTemplate"` // Workflow node template details
	State                WorkflowNodeState               `json:"state"`                // State of the workflow node
}

type WorkflowNodeState string

const (
	WorkflowNodeStateInProgress WorkflowNodeState = "IN_PROGRESS" // Node is currently active and in progress
	WorkflowNodeStateCompleted  WorkflowNodeState = "COMPLETED"   // Node has been completed
	WorkflowNodeStateFailed     WorkflowNodeState = "FAILED"      // Node has failed
)

// WorkflowNodeTemplateResponseDTO represents workflow node template details in the response.
type WorkflowNodeTemplateResponseDTO struct {
	Name        string `json:"name"`        // Name of the workflow node template
	Description string `json:"description"` // Description of the workflow node template
	Type        string `json:"type"`        // Type of the workflow node template
}

// ── Query ─────────────────────────────────────────────────────────────────────

// Filter scopes a batch consignment query.
// Use TraderCompanyID for role=trader and CHACompanyID for role=cha so that
// colleagues at the same company see each other's consignments.
type Filter struct {
	TraderCompanyID *string `json:"traderCompanyId,omitempty"`
	CHACompanyID    *string `json:"chaCompanyId,omitempty"`
	Flow            *Flow   `json:"flow,omitempty"`
	State           *State  `json:"state,omitempty"`
	Query           *string `json:"q,omitempty"`
	Offset          *int    `json:"offset,omitempty"`
	Limit           *int    `json:"limit,omitempty"`
}

// ── Internal ──────────────────────────────────────────────────────────────────

// WorkflowStatus represents the lifecycle status received from the workflow engine,
// used to propagate state changes to the consignment domain.
type WorkflowStatus string

const (
	WorkflowStatusCompleted WorkflowStatus = "COMPLETED"
)
