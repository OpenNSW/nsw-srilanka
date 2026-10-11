package caseflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrFlowNotFound is returned when no case_flows row exists for a workflow ID.
var ErrFlowNotFound = errors.New("case flow not found")

// Status is this package's view of a flow's lifecycle.
type Status string

const (
	// StatusStarting means the row is recorded but the engine start has not been
	// confirmed yet — either in flight or failed. A later StartFlow retries the start.
	StatusStarting Status = "STARTING"
	// StatusStarted means the engine accepted the workflow; later starts are no-ops.
	StatusStarted Status = "STARTED"
	// StatusCompleted means the workflow finished. Later starts are no-ops too.
	StatusCompleted Status = "COMPLETED"
)

// Case states, matching the cases_state_check constraint.
const (
	CaseInProgress = "IN_PROGRESS"
	CaseFinished   = "FINISHED"
)

// Flow is one row per top-level workflow started on a case. WorkflowID is the workflow
// instance ID passed to the engine.
type Flow struct {
	WorkflowID string `gorm:"column:workflow_id" json:"workflowId"`
	CaseID     string `gorm:"column:case_id" json:"caseId"`
	TemplateID string `gorm:"column:template_id" json:"templateId"`
	Status     Status `gorm:"column:status" json:"status"`
	// Payload is the variables the flow was first started with. Not echoed in
	// responses: it may hold business data.
	Payload   json.RawMessage `gorm:"column:payload" json:"-"`
	CreatedAt time.Time       `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time       `gorm:"column:updated_at" json:"updatedAt"`
}

// Repository persists flows and keeps each case's state in step with them.
type Repository interface {
	// Record inserts the row for f.WorkflowID, and its case if new, on first sight and is
	// a no-op afterwards, so the first payload is the one the workflow is started with.
	// A new flow reopens a case that had finished.
	Record(ctx context.Context, f Flow) error
	MarkStarted(ctx context.Context, workflowID string) error
	Get(ctx context.Context, workflowID string) (*Flow, error)
	// ListByCase returns a case's flows, oldest first.
	ListByCase(ctx context.Context, caseID string) ([]Flow, error)
	// MarkCompleted marks workflowID's flow COMPLETED and, once every flow of its case
	// is, the case FINISHED. found is false when workflowID is not a recorded flow. It
	// is idempotent, so a retried completion is harmless.
	MarkCompleted(ctx context.Context, workflowID string) (found bool, err error)
}

type repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository on the app's shared *gorm.DB.
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// EnsureCase inserts the cases row for caseID on first sight and is a no-op afterwards.
// It runs on tx, so a domain row that references the case (a consignment) can be
// created in the same transaction.
func EnsureCase(ctx context.Context, tx *gorm.DB, caseID string) error {
	return tx.WithContext(ctx).Exec(insertCase, caseID).Error
}

// The state takes its IN_PROGRESS default; name is left for the domain to set.
const insertCase = `
	INSERT INTO cases (id, created_at, updated_at)
	VALUES (?, now(), now())
	ON CONFLICT (id) DO NOTHING
`

func (r *repository) Record(ctx context.Context, f Flow) error {
	const insertFlow = `
		INSERT INTO case_flows (workflow_id, case_id, template_id, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, now(), now())
		ON CONFLICT (workflow_id) DO NOTHING
	`
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		caseResult := tx.Exec(insertCase, f.CaseID)
		if caseResult.Error != nil {
			return caseResult.Error
		}
		result := tx.Exec(insertFlow, f.WorkflowID, f.CaseID, f.TemplateID, StatusStarting, f.Payload)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// A repeat workflow ID under a different case must not leave behind the
			// empty case created above. Nothing else can reference that row yet: it is
			// uncommitted, and a concurrent insert of the same id waits on it.
			if caseResult.RowsAffected == 0 {
				return nil
			}
			return tx.Exec(`DELETE FROM cases WHERE id = ?`, f.CaseID).Error
		}
		// A new flow reopens a case that had finished.
		return tx.Exec(`UPDATE cases SET state = ?, updated_at = now() WHERE id = ? AND state <> ?`,
			CaseInProgress, f.CaseID, CaseInProgress).Error
	})
}

func (r *repository) MarkStarted(ctx context.Context, workflowID string) error {
	// Only STARTING moves forward: a fast workflow can already have completed by the
	// time its start is confirmed, and that COMPLETED must not be overwritten.
	const q = `
		UPDATE case_flows
		SET status = CASE WHEN status = ? THEN ? ELSE status END, updated_at = now()
		WHERE workflow_id = ?
	`
	result := r.db.WithContext(ctx).Exec(q, StatusStarting, StatusStarted, workflowID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrFlowNotFound
	}
	return nil
}

const selectFlow = `
	SELECT workflow_id, case_id, template_id, status, payload, created_at, updated_at
	FROM case_flows
`

func (r *repository) Get(ctx context.Context, workflowID string) (*Flow, error) {
	var f Flow
	result := r.db.WithContext(ctx).Raw(selectFlow+` WHERE workflow_id = ?`, workflowID).Scan(&f)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrFlowNotFound
	}
	return &f, nil
}

func (r *repository) ListByCase(ctx context.Context, caseID string) ([]Flow, error) {
	flows := []Flow{}
	err := r.db.WithContext(ctx).
		Raw(selectFlow+` WHERE case_id = ? ORDER BY created_at, workflow_id`, caseID).
		Scan(&flows).Error
	if err != nil {
		return nil, err
	}
	return flows, nil
}

func (r *repository) MarkCompleted(ctx context.Context, workflowID string) (bool, error) {
	found := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var caseIDs []string
		if err := tx.Raw(`SELECT case_id FROM case_flows WHERE workflow_id = ?`, workflowID).Scan(&caseIDs).Error; err != nil {
			return err
		}
		if len(caseIDs) == 0 {
			return nil
		}
		found = true
		caseID := caseIDs[0]
		// Lock the case first, so two of its flows completing at once cannot each see
		// the other still running and both leave the case IN_PROGRESS, and a flow
		// starting at the same time reopens the case after this decides.
		if err := tx.Exec(`SELECT id FROM cases WHERE id = ? FOR UPDATE`, caseID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE case_flows SET status = ?, updated_at = now() WHERE workflow_id = ?`,
			StatusCompleted, workflowID).Error; err != nil {
			return err
		}
		return tx.Exec(`
			UPDATE cases SET state = ?, updated_at = now()
			WHERE id = ? AND state <> ?
			AND NOT EXISTS (SELECT 1 FROM case_flows WHERE case_id = ? AND status <> ?)
		`, CaseFinished, caseID, CaseFinished, caseID, StatusCompleted).Error
	})
	return found, err
}
