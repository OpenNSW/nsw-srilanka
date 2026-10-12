package caseflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrFlowNotFound is returned when no case_flows row exists for a workflow ID.
var ErrFlowNotFound = errors.New("case flow not found")

// Status is this package's view of a flow's lifecycle, matching the
// case_flows_status_check constraint.
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
	WorkflowID string `json:"workflowId"`
	CaseID     string `json:"caseId"`
	TemplateID string `json:"templateId"`
	Status     Status `json:"status"`
	// Payload is the variables the flow was first started with. Not echoed in
	// responses: it may hold business data.
	Payload   json.RawMessage `json:"-"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// Repository persists flows and keeps each case's state in step with them.
type Repository interface {
	// EnsureCase inserts the cases row for caseID on first sight and is a no-op
	// afterwards, so a domain row that references the case (a consignment) can be
	// created after it.
	EnsureCase(ctx context.Context, caseID string) error
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
	db *sql.DB
}

// NewRepository creates a Repository on db, the app's shared connection pool.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

// withTx runs fn in a transaction, committing if it returns nil and rolling back
// otherwise.
func (r *repository) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // a no-op once committed
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// The state takes its IN_PROGRESS default; name is left for the domain to set.
const insertCase = `
	INSERT INTO cases (id, created_at, updated_at)
	VALUES ($1, now(), now())
	ON CONFLICT (id) DO NOTHING
`

func (r *repository) EnsureCase(ctx context.Context, caseID string) error {
	_, err := r.db.ExecContext(ctx, insertCase, caseID)
	return err
}

func (r *repository) Record(ctx context.Context, f Flow) error {
	const insertFlow = `
		INSERT INTO case_flows (workflow_id, case_id, template_id, status, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now())
		ON CONFLICT (workflow_id) DO NOTHING
	`
	// A nil payload is stored as NULL, not as an empty jsonb value.
	var payload any
	if len(f.Payload) > 0 {
		payload = []byte(f.Payload)
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		caseResult, err := tx.ExecContext(ctx, insertCase, f.CaseID)
		if err != nil {
			return err
		}
		// The flow row goes in before the case is reopened below, and that order is what keeps
		// this safe against MarkCompleted finishing the case at the same moment. The insert's
		// foreign key check on case_flows.case_id takes a key-share lock on the case row, which
		// conflicts with MarkCompleted's FOR UPDATE, so one of the two waits for the other.
		// The reopen UPDATE alone would not: while the case reads IN_PROGRESS it matches no row
		// and takes no lock. Dropping that foreign key, or reopening before the insert, would
		// let the case be marked FINISHED with this flow still pending.
		result, err := tx.ExecContext(ctx, insertFlow, f.WorkflowID, f.CaseID, f.TemplateID, StatusStarting, payload)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			// A repeat workflow ID under a different case must not leave behind the
			// empty case created above. Nothing else can reference that row yet: it is
			// uncommitted, and a concurrent insert of the same id waits on it.
			caseInserted, err := caseResult.RowsAffected()
			if err != nil {
				return err
			}
			if caseInserted == 0 {
				return nil
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM cases WHERE id = $1`, f.CaseID)
			return err
		}
		// A new flow reopens a case that had finished.
		_, err = tx.ExecContext(ctx, `UPDATE cases SET state = $1, updated_at = now() WHERE id = $2 AND state <> $1`,
			CaseInProgress, f.CaseID)
		return err
	})
}

func (r *repository) MarkStarted(ctx context.Context, workflowID string) error {
	// Only STARTING moves forward: a fast workflow can already have completed by the
	// time its start is confirmed, and that COMPLETED must not be overwritten.
	const q = `
		UPDATE case_flows
		SET status = CASE WHEN status = $1 THEN $2 ELSE status END, updated_at = now()
		WHERE workflow_id = $3
	`
	result, err := r.db.ExecContext(ctx, q, StatusStarting, StatusStarted, workflowID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrFlowNotFound
	}
	return nil
}

const selectFlow = `
	SELECT workflow_id, case_id, template_id, status, payload, created_at, updated_at
	FROM case_flows
`

// rowScanner is what scanFlow reads from: a *sql.Row or *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanFlow(row rowScanner) (Flow, error) {
	var f Flow
	var payload []byte
	if err := row.Scan(&f.WorkflowID, &f.CaseID, &f.TemplateID, &f.Status, &payload, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return Flow{}, err
	}
	if payload != nil {
		f.Payload = json.RawMessage(payload)
	}
	return f, nil
}

func (r *repository) Get(ctx context.Context, workflowID string) (*Flow, error) {
	f, err := scanFlow(r.db.QueryRowContext(ctx, selectFlow+` WHERE workflow_id = $1`, workflowID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFlowNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *repository) ListByCase(ctx context.Context, caseID string) ([]Flow, error) {
	rows, err := r.db.QueryContext(ctx, selectFlow+` WHERE case_id = $1 ORDER BY created_at, workflow_id`, caseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	flows := []Flow{}
	for rows.Next() {
		f, err := scanFlow(rows)
		if err != nil {
			return nil, err
		}
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

func (r *repository) MarkCompleted(ctx context.Context, workflowID string) (bool, error) {
	found := false
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var caseID string
		err := tx.QueryRowContext(ctx, `SELECT case_id FROM case_flows WHERE workflow_id = $1`, workflowID).Scan(&caseID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		// Lock the case first, so two of its flows completing at once cannot each see
		// the other still running and both leave the case IN_PROGRESS, and a flow
		// starting at the same time reopens the case after this decides.
		if _, err := tx.ExecContext(ctx, `SELECT id FROM cases WHERE id = $1 FOR UPDATE`, caseID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE case_flows SET status = $1, updated_at = now() WHERE workflow_id = $2`,
			StatusCompleted, workflowID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE cases SET state = $1, updated_at = now()
			WHERE id = $2 AND state <> $1
			AND NOT EXISTS (SELECT 1 FROM case_flows WHERE case_id = $2 AND status <> $3)
		`, CaseFinished, caseID, StatusCompleted)
		return err
	})
	return found, err
}
