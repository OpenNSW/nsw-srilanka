package agency

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrCaseNotFound is returned when no cases row exists for an id, or the caller owns none.
var ErrCaseNotFound = errors.New("case not found")

// Case is one row of the generic cases table.
type Case struct {
	ID        string    `gorm:"column:id"`
	Name      *string   `gorm:"column:name"`
	State     string    `gorm:"column:state"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// Owner is who reads a case: an OU, by its handle, in any of a set of catalog roles. A
// case is theirs when case_parties records the OU in one of the roles on it.
type Owner struct {
	OUHandle string
	Roles    []string
}

// CaseRepository reads the cases an owner owns.
type CaseRepository interface {
	// ListCases returns one page of the cases owner owns, newest first, and their total.
	ListCases(ctx context.Context, owner Owner, offset, limit int) ([]Case, int64, error)
	// GetCase returns case id if owner owns it, else ErrCaseNotFound.
	GetCase(ctx context.Context, owner Owner, id string) (*Case, error)
}

type repository struct {
	db *gorm.DB
}

// NewCaseRepository creates a CaseRepository on the app's shared *gorm.DB.
func NewCaseRepository(db *gorm.DB) CaseRepository {
	return &repository{db: db}
}

// owned is the condition that the case in cases is the owner's.
const owned = `EXISTS (
	SELECT 1 FROM case_parties
	WHERE case_parties.case_id = cases.id AND case_parties.party_type = 'ou'
	AND case_parties.party_id = ? AND case_parties.role IN ?
)`

func (r *repository) ListCases(ctx context.Context, owner Owner, offset, limit int) ([]Case, int64, error) {
	if owner.OUHandle == "" || len(owner.Roles) == 0 {
		return []Case{}, 0, nil
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM cases WHERE `+owned, owner.OUHandle, owner.Roles).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	q := `
		SELECT id, name, state, created_at, updated_at
		FROM cases
		WHERE ` + owned + `
		ORDER BY created_at DESC
		OFFSET ? LIMIT ?
	`
	var cases []Case
	if err := r.db.WithContext(ctx).Raw(q, owner.OUHandle, owner.Roles, offset, limit).Scan(&cases).Error; err != nil {
		return nil, 0, err
	}
	return cases, total, nil
}

func (r *repository) GetCase(ctx context.Context, owner Owner, id string) (*Case, error) {
	if owner.OUHandle == "" || len(owner.Roles) == 0 {
		return nil, ErrCaseNotFound
	}
	q := `SELECT id, name, state, created_at, updated_at FROM cases WHERE id = ? AND ` + owned
	var c Case
	result := r.db.WithContext(ctx).Raw(q, id, owner.OUHandle, owner.Roles).Scan(&c)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCaseNotFound
	}
	return &c, nil
}
