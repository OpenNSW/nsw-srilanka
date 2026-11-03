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

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
)

const (
	errUnauthorized  = "unauthorized"
	errForbiddenRole = "catalog role required"
	errCaseNotFound  = "case not found"
	errInvalidPaging = "invalid pagination parameters"
)

// TaskLister is the narrow surface CaseHandler needs from the task store.
type TaskLister interface {
	GetAllTasks(ctx context.Context, caseID string) []store.TaskRecord
}

// CaseHandler serves the officer-facing case list and detail. Responses reuse the
// JSON field names of the consignment list/detail, so trader-app's existing
// ConsignmentDetailScreen renders a case without changes.
type CaseHandler struct {
	repo      CaseRepository
	tasks     TaskLister
	artifacts *artifact.Set // resolves each task's render config for its title
	// tokenRoles maps each token role the catalog maps a role to back to the roles'
	// logical names: two names may share one token role.
	tokenRoles map[string][]string
}

// NewCaseHandler creates a CaseHandler. roles is the agency catalog's Roles map: an
// officer holding the token role of any of its entries may use the case views, and sees
// the cases their OU owns in those roles.
func NewCaseHandler(repo CaseRepository, tasks TaskLister, artifacts *artifact.Set, roles map[string]string) *CaseHandler {
	tokenRoles := make(map[string][]string, len(roles))
	for name, tokenRole := range roles {
		// An empty token role would match a caller whose roles hold an empty string.
		if tokenRole != "" {
			tokenRoles[tokenRole] = append(tokenRoles[tokenRole], name)
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

// HandleListCases handles GET /api/v1/cases: the cases the officer's OU owns in a role
// their token holds (case_parties), as the task gate decides who may open their tasks.
func (h *CaseHandler) HandleListCases(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.requireCatalogRole(w, r)
	if !ok {
		return
	}
	offsetParam, limitParam, err := pagination.ParsePaginationParams(r)
	if err != nil {
		httputil.Error(w, r, http.StatusBadRequest, errInvalidPaging)
		return
	}
	offset, limit := pagination.ResolvePaginationParams(offsetParam, limitParam)

	cases, total, err := h.repo.ListCases(r.Context(), owner, offset, limit)
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

// HandleGetCase handles GET /api/v1/cases/{id}: the case plus the tasks of every flow
// on it, if the officer's OU owns it; otherwise 404, as for a case that does not exist.
func (h *CaseHandler) HandleGetCase(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.requireCatalogRole(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	c, err := h.repo.GetCase(ctx, owner, id)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) {
			httputil.Error(w, r, http.StatusNotFound, errCaseNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to get case", err, "caseId", id)
		return
	}
	// Every flow on the case files its tasks under the case ID.
	nodes := make([]WorkflowNode, 0)
	for _, t := range h.tasks.GetAllTasks(ctx, id) {
		if t.TaskType == "SYSTEM" {
			continue
		}
		nodes = append(nodes, toNode(t, h.title(r, t)))
	}
	httputil.JSON(w, http.StatusOK, CaseDetail{CaseSummary: toSummary(*c), WorkflowNodes: nodes})
}

// title names t's node: its render config's title, else its task template's id. A render
// config that does not resolve only costs its title, so it is logged.
func (h *CaseHandler) title(r *http.Request, t store.TaskRecord) string {
	rc, err := renderconfig.Load(r.Context(), h.artifacts, t)
	if err != nil {
		slog.WarnContext(r.Context(), "agency: render config unavailable for case view", "taskId", t.TaskID, "error", err)
		return t.TaskTemplateID
	}
	return taskDisplayName(t.TaskTemplateID, rc)
}

// requireCatalogRole returns the caller as an Owner: their OU and the catalog roles their
// token holds. It writes the error response and returns false unless the caller is a user
// holding the token role of any catalog role. A machine client is authenticated but never
// an officer, so it gets 403 rather than 401.
func (h *CaseHandler) requireCatalogRole(w http.ResponseWriter, r *http.Request) (Owner, bool) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return Owner{}, false
	}
	var roles []string
	if p.Kind == authn.KindUser {
		for _, tokenRole := range p.Roles {
			for _, name := range h.tokenRoles[tokenRole] {
				if !slices.Contains(roles, name) {
					roles = append(roles, name)
				}
			}
		}
	}
	if len(roles) == 0 {
		slog.WarnContext(r.Context(), "agency: case access denied, no catalog role held")
		httputil.Error(w, r, http.StatusForbidden, errForbiddenRole)
		return Owner{}, false
	}
	return Owner{OUHandle: p.OUHandle, Roles: roles}, true
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
func toNode(t store.TaskRecord, name string) WorkflowNode {
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
			Name: name,
			Type: t.TaskType,
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
