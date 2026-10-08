package agency

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/pagination"
	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig"
	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig/taskconfigart"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

const (
	errUnauthorized     = "unauthorized"
	errForbiddenOfficer = "officer role required"
	errCaseNotFound     = "case not found"
	errInvalidPaging    = "invalid pagination parameters"
)

// TaskLister is the narrow surface CaseHandler needs from the task store.
type TaskLister interface {
	GetAllTasks(ctx context.Context, rootWorkflowID string) []store.TaskRecord
}

// CaseHandler serves the officer-facing case list and detail. Responses reuse the
// JSON field names of the consignment list/detail, so trader-app's existing
// ConsignmentDetailScreen renders a case without changes.
type CaseHandler struct {
	repo        CaseRepository
	tasks       TaskLister
	registry    *artifact.Registry // resolves each workflow's task_config for display
	officerRole string             // the token role the catalog maps RoleOfficer to
}

// NewCaseHandler creates a CaseHandler. officerRole is the token role an officer holds.
func NewCaseHandler(repo CaseRepository, tasks TaskLister, registry *artifact.Registry, officerRole string) *CaseHandler {
	return &CaseHandler{repo: repo, tasks: tasks, registry: registry, officerRole: officerRole}
}

// CaseSummary is one row of GET /api/v1/cases.
type CaseSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CaseDetail is the response of GET /api/v1/cases/{id}.
type CaseDetail struct {
	CaseSummary
	WorkflowNodes []WorkflowNode `json:"workflowNodes"`
}

// WorkflowNode is one task of any workflow in the case, in the consignment detail's
// workflowNodes shape.
type WorkflowNode struct {
	// ID is the task's ID (one run of a TASK node), not the node's ID in the workflow definition.
	ID                   string               `json:"id"`
	CreatedAt            string               `json:"createdAt"`
	UpdatedAt            string               `json:"updatedAt"`
	WorkflowNodeTemplate WorkflowNodeTemplate `json:"workflowNodeTemplate"`
	State                string               `json:"state"`
	DependsOn            []string             `json:"depends_on"`
}

// WorkflowNodeTemplate carries a task's display name and type.
type WorkflowNodeTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// HandleListCases handles GET /api/v1/cases. Officers see every case: a case has no
// owner the way a consignment has a trader company.
func (h *CaseHandler) HandleListCases(w http.ResponseWriter, r *http.Request) {
	if !h.requireOfficer(w, r) {
		return
	}
	offsetParam, limitParam, err := pagination.ParsePaginationParams(r)
	if err != nil {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidPaging)
		return
	}
	offset, limit := pagination.ResolvePaginationParams(offsetParam, limitParam)

	cases, total, err := h.repo.ListCases(r.Context(), offset, limit)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to list cases", err)
		return
	}
	items := make([]CaseSummary, len(cases))
	for i, c := range cases {
		items[i] = toSummary(c)
	}
	httputil.JSON(w, http.StatusOK, pagination.NewPageResult(items, total, offset, limit))
}

// HandleGetCase handles GET /api/v1/cases/{id}: the case plus the tasks of every
// workflow grouped under it.
func (h *CaseHandler) HandleGetCase(w http.ResponseWriter, r *http.Request) {
	if !h.requireOfficer(w, r) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	c, err := h.repo.GetCase(ctx, id)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) {
			httputil.Error(w, r, http.StatusNotFound, errCaseNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to get case", err, "caseId", id)
		return
	}
	workflows, err := h.repo.ListWorkflows(ctx, id)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to list case workflows", err, "caseId", id)
		return
	}

	nodes := make([]WorkflowNode, 0)
	for _, wf := range workflows {
		meta := h.meta(r, wf.TaskCode)
		for _, t := range h.tasks.GetAllTasks(ctx, wf.TaskID) {
			if t.TaskType == "SYSTEM" {
				continue
			}
			nodes = append(nodes, toNode(t, meta))
		}
	}
	httputil.JSON(w, http.StatusOK, CaseDetail{CaseSummary: toSummary(*c), WorkflowNodes: nodes})
}

// meta returns the display metadata of taskCode's task_config. A config that no
// longer loads only costs the view its title and description, so it is logged and
// the view falls back to the task's own render title.
func (h *CaseHandler) meta(r *http.Request, taskCode string) taskconfig.TaskMeta {
	cfg, err := taskconfigart.Load(r.Context(), h.registry, taskCode)
	if err != nil {
		slog.WarnContext(r.Context(), "agency: task config unavailable for case view", "taskCode", taskCode, "error", err)
		return taskconfig.TaskMeta{}
	}
	return cfg.Meta
}

// requireOfficer writes the error response and returns false unless the caller is a
// user holding the officer token role. A machine client is authenticated but never an
// officer, so it gets 403 rather than 401.
func (h *CaseHandler) requireOfficer(w http.ResponseWriter, r *http.Request) bool {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return false
	}
	if p.Kind != authn.KindUser || !slices.Contains(p.Roles, h.officerRole) {
		slog.WarnContext(r.Context(), "agency: case access denied, officer role missing")
		httputil.Error(w, r, http.StatusForbidden, errForbiddenOfficer)
		return false
	}
	return true
}

func toSummary(c Case) CaseSummary {
	s := CaseSummary{
		ID:        c.ID,
		State:     c.State,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
	}
	if c.Name != nil {
		s.Name = *c.Name
	}
	return s
}

// toNode maps a task record to the node states ActionListView groups by. Mirrors
// consignment.Service.buildNodeDTOsFromTaskRecords, copied rather than exported so the
// agency can diverge (e.g. officer-facing state labels) without touching TNSW code.
// The task_config's meta names the node when it has a title.
func toNode(t store.TaskRecord, meta taskconfig.TaskMeta) WorkflowNode {
	state := "IN_PROGRESS"
	switch t.State {
	case "COMPLETED", "FAILED", "QUEUED_EXTERNALLY":
		state = t.State
	}
	return WorkflowNode{
		ID:        t.TaskID,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
		WorkflowNodeTemplate: WorkflowNodeTemplate{
			Name:        taskDisplayName(meta.Title, t.ActiveTaskTemplateID, t.RenderConfig),
			Description: meta.Description,
			Type:        t.TaskType,
		},
		State:     state,
		DependsOn: []string{},
	}
}

// taskDisplayName returns the task_config title, else the render config's title, else
// the template id.
func taskDisplayName(configTitle, templateID string, renderConfig json.RawMessage) string {
	if configTitle != "" {
		return configTitle
	}
	if len(renderConfig) > 0 {
		var rc struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(renderConfig, &rc); err == nil && rc.Title != "" {
			return rc.Title
		}
	}
	return templateID
}
