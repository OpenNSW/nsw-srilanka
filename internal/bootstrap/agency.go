package bootstrap

// Agency mode wiring. Build calls into this file when config.yaml sets mode: agency,
// in place of TNSW's consignment stack. See internal/agency and docs/agency.md.

import (
	"net/http"

	"github.com/OpenNSW/core/artifact"
	"gorm.io/gorm"

	"github.com/OpenNSW/nsw-srilanka/internal/agency"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// mountAgency registers the agency's own routes: the route the systems flows.clients
// lists start flows on, gated by a scope only they hold, and the officer case views. Its
// admin view of a case is not here — see workflowAdmin.HandleGetCase in Build, mounted
// unconditionally for both modes, since an agency case needs no mode-specific admin code.
func mountAgency(
	mux *http.ServeMux,
	db *gorm.DB,
	artifacts *artifact.Set,
	flows *caseflow.Service,
	clientFlows caseflow.ClientFlows,
	tasks agency.TaskLister,
	cat *catalog.Catalog,
	maxRequestBytes int64,
	withAuth func(http.Handler) http.Handler,
	withScope func(string) func(http.Handler) http.Handler,
) {
	start := caseflow.NewClientHandler(flows, clientFlows, cat.Clients, maxRequestBytes)
	mux.Handle("POST /api/v1/internal/cases/{caseId}/flows", withAuth(withScope(caseflow.ScopeFlowStart)(http.HandlerFunc(start.HandleStart))))

	// ConsignmentRead is the read scope the portal's token already carries; the
	// handler additionally requires the token role of any catalog role.
	cases := agency.NewCaseHandler(agency.NewCaseRepository(db), tasks, artifacts, cat.Roles)
	mux.Handle("GET /api/v1/cases", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleListCases))))
	mux.Handle("GET /api/v1/cases/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(cases.HandleGetCase))))
}
