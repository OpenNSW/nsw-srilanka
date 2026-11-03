package agency

import (
	"context"
	"errors"
	"net/http"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/taskflow/store"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks"
)

// FlowLister lists a case's flows, oldest first. *caseflow.Service satisfies it.
type FlowLister interface {
	Flows(ctx context.Context, caseID string) ([]caseflow.Flow, error)
}

// AdminCaseDetail is the response of GET /api/v1/admin/cases/{id}: the case, every task on
// it, and its flows, which an admin opens by workflow ID on the admin workflow routes.
type AdminCaseDetail struct {
	CaseDetail
	Flows []caseflow.View `json:"flows"`
}

// AdminCaseHandler serves the agency's admin view of a case. It checks no ownership of the
// case and lists every task on it, whatever its read roles: the route is gated on the admin
// read scope instead, as TNSW's admin consignment view is.
type AdminCaseHandler struct {
	repo      CaseRepository
	tasks     TaskLister
	flows     FlowLister
	artifacts *artifact.Set // resolves each task's render config for its title, and each flow's name
	audit     nswaudit.Auditor
}

// NewAdminCaseHandler creates an AdminCaseHandler. A nil auditor records nothing.
func NewAdminCaseHandler(repo CaseRepository, tasks TaskLister, flows FlowLister, artifacts *artifact.Set, auditor nswaudit.Auditor) *AdminCaseHandler {
	return &AdminCaseHandler{repo: repo, tasks: tasks, flows: flows, artifacts: artifacts, audit: auditor}
}

// HandleGetCase handles GET /api/v1/admin/cases/{id}.
func (h *AdminCaseHandler) HandleGetCase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if p, ok := authn.FromContext(ctx); !ok || p.Kind != authn.KindUser {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}
	id := r.PathValue("id")

	c, err := h.repo.GetAnyCase(ctx, id)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) {
			httputil.Error(w, r, http.StatusNotFound, errCaseNotFound)
			return
		}
		httputil.InternalServerError(w, r, "failed to get case", err, "caseId", id)
		return
	}
	flows, err := h.flows.Flows(ctx, id)
	if err != nil {
		httputil.InternalServerError(w, r, "failed to list case flows", err, "caseId", id)
		return
	}

	// Every task of the case: an empty Roles filter lists them whatever their read roles.
	nodes := make([]WorkflowNode, 0)
	for _, t := range h.tasks.GetTasks(ctx, store.TaskFilter{CaseID: id}) {
		if t.TaskType == "SYSTEM" {
			continue
		}
		nodes = append(nodes, toNode(t, tasks.Title(ctx, tasks.RenderConfigs(h.artifacts), t)))
	}
	views := make([]caseflow.View, 0, len(flows))
	for _, f := range flows {
		views = append(views, caseflow.ViewOf(ctx, h.artifacts, f))
	}

	if h.audit != nil {
		h.audit.Audit(ctx, nswaudit.ConsignmentEvent(nswaudit.ActionRead, id, false, map[string]any{"view": "admin"}))
	}
	httputil.JSON(w, http.StatusOK, AdminCaseDetail{
		CaseDetail: CaseDetail{CaseSummary: toSummary(*c), WorkflowNodes: nodes},
		Flows:      views,
	})
}
