package consignment

import (
	"fmt"
	"time"

	"github.com/OpenNSW/core/pagination"
	"github.com/OpenNSW/nsw/backend/internal/workflow/model"
)

// Flow represents the flow type of consignment.
// Keep values in sync with workflow/model.ConsignmentFlow — the workflow
// package keeps its own copy to avoid importing this package.
type Flow string

const (
	FlowImport Flow = "IMPORT"
	FlowExport Flow = "EXPORT"
)

// State represents the state of a consignment.
type State string

const (
	Initialized State = "INITIALIZED"
	InProgress  State = "IN_PROGRESS"
	Finished    State = "FINISHED"
)

// Consignment represents a consignment in the system.
type Consignment struct {
	ID        string    `gorm:"type:text;column:id;primaryKey;not null" json:"id"`
	CreatedAt time.Time `gorm:"type:timestamptz;column:created_at;not null;autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"type:timestamptz;column:updated_at;not null;autoUpdateTime" json:"updatedAt"`

	// Core attributes
	Flow  Flow  `gorm:"type:varchar(50);column:flow;not null" json:"flow"`   // IMPORT or EXPORT
	State State `gorm:"type:varchar(50);column:state;not null" json:"state"` // INITIALIZED → IN_PROGRESS → FINISHED

	// Trader (set at Stage 1)
	TraderID        string `gorm:"type:varchar(100);column:trader_id;not null" json:"traderId"`                // Trader user who created the consignment
	TraderCompanyID string `gorm:"type:varchar(100);column:trader_company_id;not null" json:"traderCompanyId"` // Company the trader belongs to

	// CHA (company chosen at Stage 1; specific CHA assigned at Stage 2). Both are nil for
	// direct-start consignments (e.g. trade-export-v1), where CHA selection happens inside
	// the workflow itself rather than as an upfront trader/CHA handoff.
	CHACompanyID *string `gorm:"type:varchar(100);column:cha_company_id" json:"chaCompanyId,omitempty"` // CHA company selected by the trader at Stage 1
	CHAID        *string `gorm:"type:varchar(100);column:cha_id" json:"chaId,omitempty"`                // CHA who claimed the consignment at Stage 2

	// Relationships
	Workflow *model.Workflow `gorm:"foreignKey:ID;references:ID" json:"-"` // Associated Workflow (1:1, same ID)
}

func (c *Consignment) TableName() string {
	return "consignments"
}

// InitializeConsignmentDTO is the request body for PUT /consignments/{id} (Stage 2 – CHA selects Workflow Template).
type InitializeConsignmentDTO struct {
	WorkflowTemplateID string `json:"workflowTemplateId" binding:"required"`
}

// CreateConsignmentDTO represents the data required to create a consignment.
// Stage 1 (two-stage flow): provide flow + chaCompanyId → creates shell with state INITIALIZED.
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

// UpdateDTO represents the data required to update a consignment.
type UpdateDTO struct {
	ConsignmentID         string         `json:"consignmentId" binding:"required"` // Consignment ID
	State                 *State         `json:"state,omitempty"`                  // New state of the consignment (optional)
	AppendToGlobalContext map[string]any `json:"appendToGlobalContext,omitempty"`  // Additional global context to append to the consignment (optional)
}

// DetailDTO represents the full consignment data returned in detailed responses.
type DetailDTO struct {
	ID              string                          `json:"id"`              // Consignment ID
	Flow            Flow                            `json:"flow"`            // e.g., IMPORT, EXPORT
	State           State                           `json:"state"`           // State of the consignment
	TraderID        string                          `json:"traderId"`        // Trader user who created the consignment
	TraderCompanyID string                          `json:"traderCompanyId"` // Company the trader belongs to
	ChaCompanyID    string                          `json:"chaCompanyId"`    // CHA company selected at Stage 1
	ChaID           string                          `json:"chaId,omitempty"` // CHA assigned at Stage 2 (empty until claimed)
	CreatedAt       string                          `json:"createdAt"`       // Timestamp of consignment creation
	UpdatedAt       string                          `json:"updatedAt"`       // Timestamp of last consignment update
	WorkflowNodes   []model.WorkflowNodeResponseDTO `json:"workflowNodes"`   // Associated workflow nodes with template details
}

// SummaryDTO represents the consignment data returned in list responses.
type SummaryDTO struct {
	ID                         string `json:"id"`                         // Consignment ID
	Flow                       Flow   `json:"flow"`                       // e.g., IMPORT, EXPORT
	State                      State  `json:"state"`                      // State of the consignment
	TraderID                   string `json:"traderId"`                   // Trader user who created the consignment
	TraderCompanyID            string `json:"traderCompanyId"`            // Company the trader belongs to
	ChaCompanyID               string `json:"chaCompanyId"`               // CHA company selected at Stage 1
	ChaID                      string `json:"chaId,omitempty"`            // CHA assigned at Stage 2 (empty until claimed)
	CreatedAt                  string `json:"createdAt"`                  // Timestamp of consignment creation
	UpdatedAt                  string `json:"updatedAt"`                  // Timestamp of last consignment update
	WorkflowNodeCount          int    `json:"workflowNodeCount"`          // Total number of workflow nodes
	CompletedWorkflowNodeCount int    `json:"completedWorkflowNodeCount"` // Number of completed workflow nodes
}

// ListResult is the pagination envelope returned by the list consignments endpoint.
type ListResult = pagination.Page[SummaryDTO]

// Filter will be used when querying consignments as batch.
// For GET /consignments?role=trader use TraderCompanyID; for role=cha use CHACompanyID.
// Scoping is company-based so colleagues at the same company see each other's consignments.
type Filter struct {
	TraderCompanyID *string `json:"traderCompanyId,omitempty"`
	CHACompanyID    *string `json:"chaCompanyId,omitempty"`
	Flow            *Flow   `json:"flow,omitempty"`
	State           *State  `json:"state,omitempty"`
	Offset          *int    `json:"offset,omitempty"`
	Limit           *int    `json:"limit,omitempty"`
}
