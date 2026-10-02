package bootstrap

// Agency hooks. Everything the agency deployment adds to Build is reached from the three
// calls in app.go into this file, so the branch touches app.go by three lines and merges
// from main stay conflict-free. See internal/agency and docs/agency.md.

import (
	"net/http"

	"github.com/OpenNSW/core/artifact"
	workflow "github.com/OpenNSW/core/workflow"
	"gorm.io/gorm"

	"github.com/OpenNSW/nsw-srilanka/internal/agency"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/authzgate"
)

// newTaskAuthzGate builds the task authz gate and, when the catalog defines the
// officer role, extends it with officer ownership of injected workflows.
func newTaskAuthzGate(db *gorm.DB, roles map[string]string, ownership authzgate.OwnershipResolver, company authzgate.CompanyResolver) (agency.TaskGate, error) {
	gate, err := authzgate.NewMiddleware(ownership, company, roles)
	if err != nil {
		return nil, err
	}
	return agency.WrapTaskGate(gate, agency.NewRepository(db), roles), nil
}

// mountAgency registers the agency routes. Inject is gated by a scope only an agency
// deployment's IDP issues. The officer case views are mounted only when the catalog
// defines the officer role, so a TNSW deployment serves none of them.
func mountAgency(
	mux *http.ServeMux,
	db *gorm.DB,
	artifactRegistry *artifact.Registry,
	wm workflow.Manager,
	tasks agency.TaskLister,
	roles map[string]string,
	withAuth func(http.Handler) http.Handler,
	withScope func(string) func(http.Handler) http.Handler,
) {
	svc := agency.NewService(agency.NewRepository(db), artifactRegistry, wm)
	mux.Handle("POST /api/v1/inject", withAuth(withScope(agency.ScopeWorkflowInject)(http.HandlerFunc(svc.HandleInject))))

	officerRole, ok := roles[agency.RoleOfficer]
	if !ok {
		return
	}
	// ConsignmentRead is the read scope the portal's token already carries; the
	// handler additionally requires the officer token role.
	cases := agency.NewCaseHandler(agency.NewCaseRepository(db), tasks, artifactRegistry, officerRole)
	mux.Handle("GET /api/v1/cases", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleListCases))))
	mux.Handle("GET /api/v1/cases/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleGetCase))))
}

// agencyCompletion routes the parent runner's completions: injected workflows complete
// in the agency (workflow COMPLETED, case FINISHED once all its workflows are), every
// other workflow goes to upstream as before.
func agencyCompletion(db *gorm.DB, upstream agency.CompletionHandler) agency.CompletionHandler {
	return agency.NewCompletionRouter(agency.NewRepository(db), upstream)
}
