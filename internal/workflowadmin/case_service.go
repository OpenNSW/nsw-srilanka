package workflowadmin

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/taskflow/store"
	"gorm.io/gorm"

	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks"
)

// ErrCaseNotFound is returned when no cases row exists for an id.
var ErrCaseNotFound = errors.New("case not found")

// FlowLister lists a case's flows, oldest first. *caseflow.Service satisfies it.
type FlowLister interface {
	Flows(ctx context.Context, caseID string) ([]caseflow.Flow, error)
}

// CaseRow is the case itself, in the admin's view of it.
type CaseRow struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CaseNodeTemplate carries a task's display name and type, in the shape the trader/CHA
// and officer-facing views already use.
type CaseNodeTemplate struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CaseNode is one task of any flow on a case, for the admin view: the task store's own
// state, not a mode's business-facing label (see consignment.WorkflowNodeState and
// agency.toNode), since this is a diagnostic view, not the trader's or officer's own.
type CaseNode struct {
	ID                   string           `json:"id"`
	CreatedAt            string           `json:"createdAt"`
	UpdatedAt            string           `json:"updatedAt"`
	WorkflowNodeTemplate CaseNodeTemplate `json:"workflowNodeTemplate"`
	State                string           `json:"state"`
}

// CaseDetail is the response of GET /api/v1/admin/cases/{id}: any case — a consignment's
// or an agency case's, addressed by the one ID space they share — every task on it
// whatever its read roles, and its flows by workflow ID, which an admin opens on the
// workflow routes above.
type CaseDetail struct {
	CaseRow
	WorkflowNodes []CaseNode      `json:"workflowNodes"`
	Flows         []caseflow.View `json:"flows"`
}

// CaseService builds the admin's view of a case: the same `cases` row, task store, and
// caseflow that both a consignment and an agency case run on, addressed by the one ID
// space they share (a consignment's ID is its case's ID — see internal/consignment's
// cases JOIN). Mode-specific code — TNSW's trader/CHA company names on its own
// consignment view, an agency's ownership check on its own officer-facing case view —
// stays in internal/consignment and internal/agency; this is the part the admin
// consignment view and the admin case view need identically, so there is only one of it.
type CaseService struct {
	db        *gorm.DB
	taskStore TaskStore
	flows     FlowLister
	artifacts *artifact.Set
}

// NewCaseService creates a CaseService on the app's shared *gorm.DB.
func NewCaseService(db *gorm.DB, taskStore TaskStore, flows FlowLister, artifacts *artifact.Set) *CaseService {
	return &CaseService{db: db, taskStore: taskStore, flows: flows, artifacts: artifacts}
}

// GetCaseDetail returns id's case, with no ownership check: any caller holding the admin
// scope may fetch any case, TNSW's or an agency's.
func (s *CaseService) GetCaseDetail(ctx context.Context, id string) (*CaseDetail, error) {
	row, err := s.getCaseRow(ctx, id)
	if err != nil {
		return nil, err
	}

	rawTasks := s.taskStore.GetTasks(ctx, store.TaskFilter{CaseID: id})
	nodes := make([]CaseNode, 0, len(rawTasks))
	for _, t := range rawTasks {
		if t.TaskType == "SYSTEM" {
			continue
		}
		nodes = append(nodes, CaseNode{
			ID:        t.TaskID,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
			WorkflowNodeTemplate: CaseNodeTemplate{
				Name: tasks.Title(ctx, tasks.RenderConfigs(s.artifacts), t),
				Type: t.TaskType,
			},
			State: t.State,
		})
	}

	flows, err := s.flows.Flows(ctx, id)
	if err != nil {
		return nil, err
	}
	views := make([]caseflow.View, 0, len(flows))
	for _, f := range flows {
		views = append(views, caseflow.ViewOf(ctx, s.artifacts, f))
	}

	return &CaseDetail{CaseRow: row, WorkflowNodes: nodes, Flows: views}, nil
}

func (s *CaseService) getCaseRow(ctx context.Context, id string) (CaseRow, error) {
	var r struct {
		ID        string
		Name      sql.NullString
		State     string
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	result := s.db.WithContext(ctx).Raw(
		`SELECT id, name, state, created_at, updated_at FROM cases WHERE id = ?`, id,
	).Scan(&r)
	if result.Error != nil {
		return CaseRow{}, result.Error
	}
	if result.RowsAffected == 0 {
		return CaseRow{}, ErrCaseNotFound
	}
	return CaseRow{
		ID:        r.ID,
		Name:      r.Name.String,
		State:     r.State,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}, nil
}
