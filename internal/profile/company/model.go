package company

import (
	"encoding/json"
	"time"

	"github.com/OpenNSW/nsw/backend/pkg/pagination"
)

// Record represents a company's persisted profile in the database.
type Record struct {
	ID        string          `gorm:"type:varchar(100);column:id;primaryKey;not null" json:"id"`
	Name      string          `gorm:"type:varchar(255);column:name;not null" json:"name"`
	OUHandle  string          `gorm:"type:varchar(255);column:ou_handle;unique;not null" json:"ouHandle"`
	HasCHA    bool            `gorm:"column:has_cha;not null;default:false" json:"hasCha"`
	Data      json.RawMessage `gorm:"type:jsonb;column:data;not null;default:'{}'" json:"data"`
	CreatedAt time.Time       `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time       `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (r *Record) TableName() string {
	return "company_records"
}

// ListFilter narrows the set returned by Service.ListCompanies. Nil fields mean "no filter".
type ListFilter struct {
	// HasCHA filters to companies whose has_cha column matches the pointed-to value when non-nil.
	HasCHA *bool
	// Name filters to companies whose name matches a case-insensitive substring when non-empty.
	Name   *string
	Offset *int
	Limit  *int
}

// Summary is the trimmed projection of a company returned by the list endpoint. It carries only
// the fields callers (e.g. the trader-app CHA-company picker) actually consume, so the response
// stays small and the storage shape of Record can evolve independently.
type Summary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	HasCHA bool   `json:"hasCha"`
}

// ListResult is the pagination envelope returned by GET /api/v1/companies.
type ListResult = pagination.Page[Summary]
