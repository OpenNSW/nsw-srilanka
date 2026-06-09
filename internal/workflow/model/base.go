package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BaseModel defines the base model structure with common fields.
type BaseModel struct {
	ID        string    `gorm:"type:text;column:id;not null;primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"type:timestamptz;column:created_at;not null" json:"createdAt"`
	UpdatedAt time.Time `gorm:"type:timestamptz;column:updated_at;not null" json:"updatedAt"`
}

// BeforeCreate is a GORM hook that is triggered before a new record is created.
func (base *BaseModel) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	base.ID = id.String()
	base.CreatedAt = time.Now().UTC()
	base.UpdatedAt = time.Now().UTC()
	return nil
}

// BeforeUpdate is a GORM hook that is triggered before an existing record is updated.
func (base *BaseModel) BeforeUpdate(tx *gorm.DB) error {
	base.UpdatedAt = time.Now().UTC()
	return nil
}
