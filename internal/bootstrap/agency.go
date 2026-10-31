package bootstrap

// Agency mode wiring. Build calls into this file when config.yaml sets mode: agency,
// in place of TNSW's consignment stack. See internal/agency and docs/agency.md.

import (
	"net/http"

	"github.com/OpenNSW/core/artifact"
	workflow "github.com/OpenNSW/core/workflow"
	"gorm.io/gorm"

	"github.com/OpenNSW/nsw-srilanka/internal/agency"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// mountAgency registers the agency's own routes: inject, gated by a scope only the
// injecting client holds, and the officer case views.
func mountAgency(
	mux *http.ServeMux,
	db *gorm.DB,
	artifacts *artifact.Set,
	wm workflow.Manager,
	tasks agency.TaskLister,
	roles map[string]string,
	withAuth func(http.Handler) http.Handler,
	withScope func(string) func(http.Handler) http.Handler,
) {
	svc := agency.NewService(agency.NewRepository(db), artifacts, wm)
	mux.Handle("POST /api/v1/inject", withAuth(withScope(agency.ScopeWorkflowInject)(http.HandlerFunc(svc.HandleInject))))

	// ConsignmentRead is the read scope the portal's token already carries; the
	// handler additionally requires the token role of any catalog role.
	cases := agency.NewCaseHandler(agency.NewCaseRepository(db), tasks, artifacts, roles)
	mux.Handle("GET /api/v1/cases", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleListCases))))
	mux.Handle("GET /api/v1/cases/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleGetCase))))
}

// agencyCompletion is the parent runner's completion handler in agency mode: every
// parent workflow is an injected one, completed in the agency.
func agencyCompletion(db *gorm.DB) parentUpstreamService {
	return agency.NewCompletionHandler(agency.NewRepository(db))
}
