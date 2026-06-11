package model

type WorkflowTemplate struct {
	BaseModel
	Name              string      `gorm:"type:varchar(100);column:name;not null" json:"name"`                       // Name of the workflow template
	Description       string      `gorm:"type:text;column:description" json:"description"`                          // Description of the workflow template
	Version           string      `gorm:"type:varchar(50);column:version;not null" json:"version"`                  // Version of the workflow template
	NodeTemplates     StringArray `gorm:"type:jsonb;column:nodes;not null;serializer:json" json:"nodes"`            // Array of workflow node template IDs
	EndNodeTemplateID *string     `gorm:"type:text;column:end_node_template_id" json:"endNodeTemplateId,omitempty"` // Optional end node template ID. If set, workflow is complete when this node is completed.
}

func (wt *WorkflowTemplate) TableName() string {
	return "workflow_templates"
}

func (wt *WorkflowTemplate) GetNodeTemplateIDs() []string {
	return wt.NodeTemplates
}
