package bootstrap

// Agency mode wiring. Build calls into this file when config.yaml sets mode: agency,
// in place of TNSW's consignment stack. See internal/agency and docs/agency.md.

import (
	"fmt"
	"net/http"

	"github.com/OpenNSW/core/artifact"
	workflow "github.com/OpenNSW/core/workflow"
	"gorm.io/gorm"

	"github.com/OpenNSW/nsw-srilanka/internal/agency"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// newAgencyTaskGate builds the officer-only task authz gate. The catalog must map the
// officer role: it is the only role anyone can act in on an agency's tasks.
func newAgencyTaskGate(db *gorm.DB, roles map[string]string) (*agency.OfficerGate, error) {
	if err := catalog.RequireRoles(roles, agency.RoleOfficer); err != nil {
		return nil, fmt.Errorf("agency mode: %w", err)
	}
	return agency.NewOfficerGate(agency.NewRepository(db)), nil
}

// mountAgency registers the agency's own routes: inject, gated by a scope only the
// injecting client holds, and the officer case views. newAgencyTaskGate has already
// required the catalog to map the officer role.
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

	// ConsignmentRead is the read scope the portal's token already carries; the
	// handler additionally requires the officer token role.
	cases := agency.NewCaseHandler(agency.NewCaseRepository(db), tasks, artifactRegistry, roles[agency.RoleOfficer])
	mux.Handle("GET /api/v1/cases", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleListCases))))
	mux.Handle("GET /api/v1/cases/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleGetCase))))
}

// agencyCompletion is the parent runner's completion handler in agency mode: every
// parent workflow is an injected one, completed in the agency.
func agencyCompletion(db *gorm.DB) parentUpstreamService {
	return agency.NewCompletionHandler(agency.NewRepository(db))
}
