package model

import (
	"encoding/json"
)

type WorkflowNodeTemplateType string

type WorkflowNodeState string

const (
	WorkflowNodeStateLocked     WorkflowNodeState = "LOCKED"      // Node is locked and cannot be activated because previous nodes are incomplete
	WorkflowNodeStateReady      WorkflowNodeState = "READY"       // Node is ready to be activated
	WorkflowNodeStateInProgress WorkflowNodeState = "IN_PROGRESS" // Node is currently active and in progress
	WorkflowNodeStateCompleted  WorkflowNodeState = "COMPLETED"   // Node has been completed
	WorkflowNodeStateFailed     WorkflowNodeState = "FAILED"      // Node has failed
)

// WorkflowNodeTemplate represents a template for a workflow node.
type WorkflowNodeTemplate struct {
	BaseModel
	Name        string                   `gorm:"type:varchar(255);column:name;not null" json:"name"`              // Human-readable name of the workflow node template
	Description string                   `gorm:"type:text;column:description" json:"description"`                 // Optional description of the workflow node template
	Type        WorkflowNodeTemplateType `gorm:"type:varchar(50);column:type;not null" json:"type"`               // Type of the workflow node
	Config      json.RawMessage          `gorm:"type:jsonb;column:config;not null;serializer:json" json:"config"` // Configuration specific to the workflow node type
}

func (wnt *WorkflowNodeTemplate) TableName() string {
	return "workflow_node_templates"
}

// WorkflowNode represents an instance of a workflow node within a workflow.
type WorkflowNode struct {
	BaseModel
	WorkflowID             string            `gorm:"type:text;column:workflow_id;not null" json:"workflowId"`                           // Reference to the parent Workflow
	WorkflowNodeTemplateID string            `gorm:"type:text;column:workflow_node_template_id;not null" json:"workflowNodeTemplateId"` // Reference to the WorkflowNodeTemplate
	State                  WorkflowNodeState `gorm:"type:varchar(50);column:state;not null" json:"state"`                               // State of the workflow node
	ExtendedState          *string           `gorm:"type:text;column:extended_state" json:"extendedState"`                              // Optional extended state information (e.g., error details)
	Outcome                *string           `gorm:"type:varchar(100);column:outcome" json:"outcome,omitempty"`                         // Outcome sub-state when COMPLETED (e.g., APPROVED, REJECTED)

	// Relationships
	Workflow             *Workflow            `gorm:"foreignKey:WorkflowID;references:ID" json:"-"`                                // Associated Workflow
	WorkflowNodeTemplate WorkflowNodeTemplate `gorm:"foreignKey:WorkflowNodeTemplateID;references:ID" json:"workflowNodeTemplate"` // Associated WorkflowNodeTemplate
}

func (wn *WorkflowNode) TableName() string {
	return "workflow_nodes"
}

// WorkflowNodeResponseDTO represents a workflow node in the response.
type WorkflowNodeResponseDTO struct {
	ID                   string                          `json:"id"`                      // Workflow Node ID
	CreatedAt            string                          `json:"createdAt"`               // Timestamp of node creation
	UpdatedAt            string                          `json:"updatedAt"`               // Timestamp of last node update
	WorkflowNodeTemplate WorkflowNodeTemplateResponseDTO `json:"workflowNodeTemplate"`    // Workflow node template details
	State                WorkflowNodeState               `json:"state"`                   // State of the workflow node
	ExtendedState        *string                         `json:"extendedState,omitempty"` // Optional extended state information (e.g., error details)
	Outcome              *string                         `json:"outcome,omitempty"`       // Outcome sub-state when COMPLETED
}

// WorkflowNodeTemplateResponseDTO represents workflow node template details in the response.
type WorkflowNodeTemplateResponseDTO struct {
	Name        string `json:"name"`        // Name of the workflow node template
	Description string `json:"description"` // Description of the workflow node template
	Type        string `json:"type"`        // Type of the workflow node template
}
