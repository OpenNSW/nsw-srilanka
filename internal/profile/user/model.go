package user

import (
	"encoding/json"
	"time"
)

// Record represents a user's persisted profile in the database.
// This model intentionally excludes roles; roles are derived from the authentication principal.
type Record struct {
	ID          string          `gorm:"type:varchar(100);column:id;primaryKey;not null" json:"id"`
	IDPUserID   string          `gorm:"type:varchar(255);column:idp_user_id;unique;not null" json:"idpUserId"`
	Email       string          `gorm:"type:varchar(255);column:email" json:"email"`
	PhoneNumber string          `gorm:"type:varchar(20);column:phone_number" json:"phoneNumber"`
	OUID        string          `gorm:"type:varchar(255);column:ou_id" json:"ouId"`
	OUHandle    string          `gorm:"type:varchar(255);column:ou_handle;not null" json:"ouHandle"`
	Data        json.RawMessage `gorm:"type:jsonb;column:data" json:"data"`
	CreatedAt   time.Time       `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time       `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

// TableName specifies the database table for this model.
func (r *Record) TableName() string {
	return "user_records"
}
