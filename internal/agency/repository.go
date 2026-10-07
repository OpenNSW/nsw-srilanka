package agency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrWorkflowNotFound is returned when no agency_workflow row exists for a taskId.
var ErrWorkflowNotFound = errors.New("agency workflow not found")

// Status is this subsystem's own view of an injected workflow's lifecycle.
type Status string

const (
	// StatusStarting means the row is recorded but the engine start has not been
	// confirmed yet — either in flight or failed. A later inject retries the start.
	StatusStarting Status = "STARTING"
	// StatusStarted means the engine accepted the workflow; later injects are no-ops.
	StatusStarted Status = "STARTED"
	// StatusCompleted means the workflow finished. Later injects are no-ops too.
	StatusCompleted Status = "COMPLETED"
)

// Case states, matching the cases_state_check constraint.
const (
	CaseInProgress = "IN_PROGRESS"
	CaseFinished   = "FINISHED"
)

// Workflow is one row per injected taskId. TaskID doubles as the workflow instance ID
// passed to the engine, so it is also the RootWorkflowID of every task spawned under it.
// CaseID groups it with the other workflows working on the same case.
type Workflow struct {
	TaskID   string `gorm:"column:task_id" json:"taskId"`
	TaskCode string `gorm:"column:task_code" json:"taskCode"`
	CaseID   string `gorm:"column:case_id" json:"caseId"`
	Status   Status `gorm:"column:status" json:"status"`
	// CallbackToken is the token the injecting system expects the decision back on.
	// Not echoed in responses: only the workflow needs it.
	CallbackToken string          `gorm:"column:callback_token" json:"-"`
	Payload       json.RawMessage `gorm:"column:payload" json:"payload,omitempty"`
	CreatedAt     time.Time       `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt     time.Time       `gorm:"column:updated_at" json:"updatedAt"`
}

// Repository persists injected workflows.
type Repository interface {
	// Record inserts the row for taskID, and its case if new, on first sight and is a
	// no-op afterwards, so the first payload is the one the workflow was started with.
	Record(ctx context.Context, w Workflow) error
	MarkStarted(ctx context.Context, taskID string) error
	Get(ctx context.Context, taskID string) (*Workflow, error)
	// Exists reports whether taskID is an injected workflow — the check the task authz
	// gate uses to decide officer ownership.
	Exists(ctx context.Context, taskID string) (bool, error)
	// MarkCompleted marks taskID's workflow COMPLETED and, once every workflow of its
	// case is, the case FINISHED. found is false when taskID is not an injected
	// workflow. It is idempotent, so a retried completion is harmless.
	MarkCompleted(ctx context.Context, taskID string) (found bool, err error)
}

type repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository on the app's shared *gorm.DB.
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Record(ctx context.Context, w Workflow) error {
	// name is left unset (nothing supplies one yet) and state takes its IN_PROGRESS default.
	const insertCase = `
		INSERT INTO cases (id, created_at, updated_at)
		VALUES (?, now(), now())
		ON CONFLICT (id) DO NOTHING
	`
	const insertWorkflow = `
		INSERT INTO agency_workflow (task_id, task_code, case_id, status, callback_token, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, now(), now())
		ON CONFLICT (task_id) DO NOTHING
	`
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		caseResult := tx.Exec(insertCase, w.CaseID)
		if caseResult.Error != nil {
			return caseResult.Error
		}
		result := tx.Exec(insertWorkflow, w.TaskID, w.TaskCode, w.CaseID, StatusStarting, w.CallbackToken, w.Payload)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// A repeat taskId under a different consignmentId must not leave behind the
			// empty case created above. Nothing else can reference that row yet: it is
			// uncommitted, and a concurrent insert of the same id waits on it.
			if caseResult.RowsAffected == 0 {
				return nil
			}
			return tx.Exec(`DELETE FROM cases WHERE id = ?`, w.CaseID).Error
		}
		// A new workflow reopens a case that had finished.
		return tx.Exec(`UPDATE cases SET state = ?, updated_at = now() WHERE id = ? AND state <> ?`,
			CaseInProgress, w.CaseID, CaseInProgress).Error
	})
}

func (r *repository) MarkCompleted(ctx context.Context, taskID string) (bool, error) {
	found := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var caseIDs []string
		if err := tx.Raw(`SELECT case_id FROM agency_workflow WHERE task_id = ?`, taskID).Scan(&caseIDs).Error; err != nil {
			return err
		}
		if len(caseIDs) == 0 {
			return nil
		}
		found = true
		caseID := caseIDs[0]
		// Lock the case first, so two of its workflows completing at once cannot each
		// see the other still running and both leave the case IN_PROGRESS.
		if err := tx.Exec(`SELECT id FROM cases WHERE id = ? FOR UPDATE`, caseID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE agency_workflow SET status = ?, updated_at = now() WHERE task_id = ?`,
			StatusCompleted, taskID).Error; err != nil {
			return err
		}
		return tx.Exec(`
			UPDATE cases SET state = ?, updated_at = now()
			WHERE id = ? AND state <> ?
			AND NOT EXISTS (SELECT 1 FROM agency_workflow WHERE case_id = ? AND status <> ?)
		`, CaseFinished, caseID, CaseFinished, caseID, StatusCompleted).Error
	})
	return found, err
}

func (r *repository) MarkStarted(ctx context.Context, taskID string) error {
	// Only STARTING moves forward: a fast workflow can already have completed by the
	// time its start is confirmed, and that COMPLETED must not be overwritten.
	const q = `
		UPDATE agency_workflow
		SET status = CASE WHEN status = ? THEN ? ELSE status END, updated_at = now()
		WHERE task_id = ?
	`
	result := r.db.WithContext(ctx).Exec(q, StatusStarting, StatusStarted, taskID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWorkflowNotFound
	}
	return nil
}

func (r *repository) Get(ctx context.Context, taskID string) (*Workflow, error) {
	const q = `
		SELECT task_id, task_code, case_id, status, COALESCE(callback_token, '') AS callback_token,
			payload, created_at, updated_at
		FROM agency_workflow
		WHERE task_id = ?
	`
	var w Workflow
	result := r.db.WithContext(ctx).Raw(q, taskID).Scan(&w)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrWorkflowNotFound
	}
	return &w, nil
}

func (r *repository) Exists(ctx context.Context, taskID string) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM agency_workflow WHERE task_id = ?)`
	var exists bool
	if err := r.db.WithContext(ctx).Raw(q, taskID).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}

// ErrCaseNotFound is returned when no cases row exists for an id.
var ErrCaseNotFound = errors.New("case not found")

// Case is one row of the generic cases table.
type Case struct {
	ID        string    `gorm:"column:id"`
	Name      *string   `gorm:"column:name"`
	State     string    `gorm:"column:state"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// CaseRepository reads cases and the workflows grouped under them.
type CaseRepository interface {
	// ListCases returns one page of cases, newest first, and the total count.
	ListCases(ctx context.Context, offset, limit int) ([]Case, int64, error)
	GetCase(ctx context.Context, id string) (*Case, error)
	// ListWorkflows returns every workflow in the case, oldest first.
	ListWorkflows(ctx context.Context, caseID string) ([]CaseWorkflow, error)
}

// CaseWorkflow identifies one workflow of a case: its instance id and the taskCode
// that selected its task_config.
type CaseWorkflow struct {
	TaskID   string `gorm:"column:task_id"`
	TaskCode string `gorm:"column:task_code"`
}

// NewCaseRepository creates a CaseRepository on the app's shared *gorm.DB.
func NewCaseRepository(db *gorm.DB) CaseRepository {
	return &repository{db: db}
}

func (r *repository) ListCases(ctx context.Context, offset, limit int) ([]Case, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM cases`).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	const q = `
		SELECT id, name, state, created_at, updated_at
		FROM cases
		ORDER BY created_at DESC
		OFFSET ? LIMIT ?
	`
	var cases []Case
	if err := r.db.WithContext(ctx).Raw(q, offset, limit).Scan(&cases).Error; err != nil {
		return nil, 0, err
	}
	return cases, total, nil
}

func (r *repository) GetCase(ctx context.Context, id string) (*Case, error) {
	const q = `SELECT id, name, state, created_at, updated_at FROM cases WHERE id = ?`
	var c Case
	result := r.db.WithContext(ctx).Raw(q, id).Scan(&c)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCaseNotFound
	}
	return &c, nil
}

func (r *repository) ListWorkflows(ctx context.Context, caseID string) ([]CaseWorkflow, error) {
	const q = `SELECT task_id, task_code FROM agency_workflow WHERE case_id = ? ORDER BY created_at`
	var workflows []CaseWorkflow
	if err := r.db.WithContext(ctx).Raw(q, caseID).Scan(&workflows).Error; err != nil {
		return nil, err
	}
	return workflows, nil
}
