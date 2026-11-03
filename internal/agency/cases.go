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
	"github.com/OpenNSW/core/taskflow/renderconfig"
	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig"
	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig/taskconfigart"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

const (
	errUnauthorized  = "unauthorized"
	errForbiddenRole = "catalog role required"
	errCaseNotFound  = "case not found"
	errInvalidPaging = "invalid pagination parameters"
)

// TaskLister is the narrow surface CaseHandler needs from the task store: the tasks of a
// case whose task templates list, in read.roles, one of the catalog roles the officer
// holds. core's task store satisfies it. The roles must never be empty, or GetTasks would
// list every task of the case: requireCatalogRole admits no officer without one.
type TaskLister interface {
	GetTasks(ctx context.Context, filter store.TaskFilter) []store.TaskRecord
}

// CaseHandler serves the officer-facing case list and detail. Responses reuse the
// JSON field names of the consignment list/detail, so trader-app's existing
// ConsignmentDetailScreen renders a case without changes.
type CaseHandler struct {
	repo       CaseRepository
	tasks      TaskLister
	artifacts  *artifact.Set       // resolves each workflow's task_config for display
	tokenRoles map[string][]string // token role -> the catalog roles mapped to it
}

// NewCaseHandler creates a CaseHandler. roles is the agency catalog's Roles map, from
// catalog role to token role: an officer holding the token role of any of its entries may
// use the case views, and sees the tasks whose read.roles list one of those catalog roles.
func NewCaseHandler(repo CaseRepository, tasks TaskLister, artifacts *artifact.Set, roles map[string]string) *CaseHandler {
	tokenRoles := make(map[string][]string, len(roles))
	for role, tokenRole := range roles {
		// An empty token role would match a caller whose roles hold an empty string.
		if tokenRole != "" {
			tokenRoles[tokenRole] = append(tokenRoles[tokenRole], role)
		}
	}
	return &CaseHandler{repo: repo, tasks: tasks, artifacts: artifacts, tokenRoles: tokenRoles}
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

// HandleListCases handles GET /api/v1/cases. Officers see every case, whichever
// catalog role they hold: a case has no owner the way a consignment has a trader
// company.
func (h *CaseHandler) HandleListCases(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireCatalogRole(w, r); !ok {
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
// workflow grouped under it that the officer may read. A task is listed when its task
// template's read.roles name a catalog role the officer holds; one listing none is not.
func (h *CaseHandler) HandleGetCase(w http.ResponseWriter, r *http.Request) {
	roles, ok := h.requireCatalogRole(w, r)
	if !ok {
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
		meta := h.meta(r, wf)
		// Each injected workflow is a case of its own in the task store, keyed by its task ID.
		for _, t := range h.tasks.GetTasks(ctx, store.TaskFilter{CaseID: wf.TaskID, Roles: roles}) {
			if t.TaskType == "SYSTEM" {
				continue
			}
			nodes = append(nodes, toNode(t, meta, h.title(r, t, meta)))
		}
	}
	httputil.JSON(w, http.StatusOK, CaseDetail{CaseSummary: toSummary(*c), WorkflowNodes: nodes})
}

// meta returns the display metadata of wf's task_config, from the manifest version wf
// runs on. A config that does not load only costs the view its title and description,
// so it is logged and the view falls back to the task's own render title.
func (h *CaseHandler) meta(r *http.Request, wf CaseWorkflow) taskconfig.TaskMeta {
	reg, err := h.artifacts.Registry(r.Context(), wf.ManifestVersion)
	if err == nil {
		var cfg *taskconfig.TaskConfig
		if cfg, err = taskconfigart.Load(r.Context(), reg, wf.TaskCode); err == nil {
			return cfg.Meta
		}
	}
	slog.WarnContext(r.Context(), "agency: task config unavailable for case view", "taskCode", wf.TaskCode, "manifestVersion", wf.ManifestVersion, "error", err)
	return taskconfig.TaskMeta{}
}

// title names t's node: its task_config title, else its render config's, else its task
// template's id. A render config that does not resolve only costs its title, so it is logged.
func (h *CaseHandler) title(r *http.Request, t store.TaskRecord, meta taskconfig.TaskMeta) string {
	if meta.Title != "" {
		return meta.Title
	}
	rc, err := renderconfig.Load(r.Context(), h.artifacts, t)
	if err != nil {
		slog.WarnContext(r.Context(), "agency: render config unavailable for case view", "taskId", t.TaskID, "error", err)
		return t.TaskTemplateID
	}
	return taskDisplayName(t.TaskTemplateID, rc)
}

// requireCatalogRole returns the catalog roles the caller holds, sorted, or writes the
// error response and returns false unless the caller is a user holding the token role of
// any catalog role. A machine client is authenticated but never an officer, so it gets
// 403 rather than 401. An officer's roles come from their token, so they hold them in
// every case.
func (h *CaseHandler) requireCatalogRole(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return nil, false
	}
	var roles []string
	if p.Kind == authn.KindUser {
		for _, tokenRole := range p.Roles {
			roles = append(roles, h.tokenRoles[tokenRole]...)
		}
	}
	if len(roles) == 0 {
		slog.WarnContext(r.Context(), "agency: case access denied, no catalog role held")
		httputil.Error(w, r, http.StatusForbidden, errForbiddenRole)
		return nil, false
	}
	slices.Sort(roles)
	return slices.Compact(roles), true
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
// name is the node's title (see CaseHandler.title).
func toNode(t store.TaskRecord, meta taskconfig.TaskMeta, name string) WorkflowNode {
	state := "IN_PROGRESS"
	switch t.State {
	case "COMPLETED", "FAILED", "QUEUED_EXTERNALLY", store.StateCancelled, store.StateOnHold:
		state = t.State
	}
	return WorkflowNode{
		ID:        t.TaskID,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
		WorkflowNodeTemplate: WorkflowNodeTemplate{
			Name:        name,
			Description: meta.Description,
			Type:        t.TaskType,
		},
		State:     state,
		DependsOn: []string{},
	}
}

// taskDisplayName returns the title at the root of renderConfig, else the template id.
func taskDisplayName(templateID string, renderConfig json.RawMessage) string {
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
