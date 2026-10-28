// Package caseparty records who holds which logical role on a case (case_parties): in
// TNSW, the consignment's trader company and the CHA company it selected. A role is a
// catalog logical role name ("trader", "cha", ...), so a new role needs no schema change.
// A case has at most one party per role.
//
// Task authorization asks it which roles the caller's company holds on a task's case
// (internal/tasks/authzgate); the consignment reads its trader and CHA companies from it.
package caseparty

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Party types: what party_id names.
const (
	TypeCompany = "company" // company_records.id
	TypeUser    = "user"    // user_records.id
)

// Party is the holder of one role on a case: a row of case_parties.
type Party struct {
	CaseID string `gorm:"type:text;column:case_id;primaryKey"`
	Role   string `gorm:"type:text;column:role;primaryKey"`
	Type   string `gorm:"type:text;column:party_type;not null"`
	ID     string `gorm:"type:text;column:party_id;not null"`
	// AssignedBy is the user who gave the party its role, when one did.
	AssignedBy *string   `gorm:"type:text;column:assigned_by"`
	CreatedAt  time.Time `gorm:"type:timestamptz;column:created_at;not null;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"type:timestamptz;column:updated_at;not null;autoUpdateTime"`
}

func (Party) TableName() string {
	return "case_parties"
}

// Ref names a party by its type and id.
type Ref struct {
	Type string
	ID   string
}

// Company names a company party.
func Company(id string) Ref {
	return Ref{Type: TypeCompany, ID: id}
}

// Assign makes p the holder of p.Role on p.CaseID, replacing any earlier holder. db may
// be a transaction, so the assignment commits with the write that caused it.
func Assign(ctx context.Context, db *gorm.DB, p Party) error {
	if p.CaseID == "" || p.Role == "" || p.Type == "" || p.ID == "" {
		return fmt.Errorf("caseparty: case id, role, party type and party id are required")
	}
	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "case_id"}, {Name: "role"}},
		DoUpdates: clause.AssignmentColumns([]string{"party_type", "party_id", "assigned_by", "updated_at"}),
	}).Create(&p).Error
	if err != nil {
		return fmt.Errorf("caseparty: assign %q on case %s: %w", p.Role, p.CaseID, err)
	}
	return nil
}

// Repository reads case parties.
type Repository struct {
	db *gorm.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// RolesOf reports the roles any of parties holds on the case. A case that does not exist,
// or has no matching party, holds none: the map is empty, not an error, so a caller is
// denied rather than failed.
func (r *Repository) RolesOf(ctx context.Context, caseID string, parties ...Ref) (map[string]bool, error) {
	roles := map[string]bool{}
	if caseID == "" || len(parties) == 0 {
		return roles, nil
	}
	q := r.db.WithContext(ctx).Model(&Party{}).Where("case_id = ?", caseID)
	match := r.db.Where("party_type = ? AND party_id = ?", parties[0].Type, parties[0].ID)
	for _, p := range parties[1:] {
		match = match.Or("party_type = ? AND party_id = ?", p.Type, p.ID)
	}
	var held []string
	if err := q.Where(match).Pluck("role", &held).Error; err != nil {
		return nil, fmt.Errorf("caseparty: roles on case %s: %w", caseID, err)
	}
	for _, role := range held {
		roles[role] = true
	}
	return roles, nil
}
