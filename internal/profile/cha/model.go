package cha

import (
	"time"
)

// Record represents a Customs House Agent's persisted profile in the database.
type Record struct {
	ID          string    `gorm:"type:varchar(100);column:id;primaryKey;not null" json:"id"`
	Name        string    `gorm:"type:varchar(255);column:name;not null" json:"name"`
	Description string    `gorm:"type:text;column:description" json:"description"`
	Email       string    `gorm:"type:varchar(255);column:email" json:"email,omitempty"`
	CompanyID   string    `gorm:"type:varchar(100);column:company_id;not null" json:"companyId"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (r *Record) TableName() string {
	return "customs_house_agents"
}
