package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkflowStatus represents the status of a workflow instance.
type WorkflowStatus string

const (
	WorkflowStatusInProgress WorkflowStatus = "IN_PROGRESS"
	WorkflowStatusCompleted  WorkflowStatus = "COMPLETED"
	WorkflowStatusFailed     WorkflowStatus = "FAILED"
)

// Workflow represents a generic workflow instance.
// The ID is set by the caller (e.g., ConsignmentID or PreConsignmentID) to maintain a 1:1 relationship.
type Workflow struct {
	BaseModel
	Status        WorkflowStatus `gorm:"type:varchar(50);column:status;not null" json:"status"`
	GlobalContext map[string]any `gorm:"type:jsonb;column:global_context;serializer:json;not null" json:"globalContext"`
	EndNodeID     *string        `gorm:"type:text;column:end_node_id" json:"endNodeId,omitempty"`

	// Relationships
	WorkflowNodes []WorkflowNode `gorm:"foreignKey:WorkflowID;references:ID" json:"workflowNodes,omitempty"`
}

func (w *Workflow) TableName() string {
	return "workflows"
}

// BeforeCreate overrides BaseModel.BeforeCreate to preserve caller-set IDs.
// When the caller sets ID (e.g., ConsignmentID), it won't be overwritten.
func (w *Workflow) BeforeCreate(tx *gorm.DB) error {
	if w.ID == "" {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		w.ID = id.String()
	}
	w.CreatedAt = time.Now().UTC()
	w.UpdatedAt = time.Now().UTC()
	return nil
}
