package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// requireAny stands in for authz's RequireScope: it records each scope required and
// admits every request.
func requireAny(required *[]string) func(string) func(http.Handler) http.Handler {
	return func(scope string) func(http.Handler) http.Handler {
		*required = append(*required, scope)
		return func(next http.Handler) http.Handler { return next }
	}
}

// A deployment must name the scope of every route its mode serves, and only those:
// the routes wired decide, so an agency need not name TNSW's scopes.
func TestScopeGates_TheWiredRoutesDecideTheScopesRequired(t *testing.T) {
	var required []string

	tnsw := newScopeGates(scopes.NewResolver(nil), requireAny(&required))
	mountTNSW(http.NewServeMux(), tnswHandlers{files: nswstorage.NewFileHandler(nil, refsStub{})}, stubAuth, tnsw.withScope)
	const wantTNSW = "catalog.scopes must name the scope for: chaRead, companyRead, consignmentAdminRead, " +
		"consignmentAdminWrite, consignmentFlowWrite, consignmentRead, consignmentWrite, paymentWebhooksProcess, " +
		"paymentWebhooksValidate, slceWebhooksWrite"
	if err := tnsw.err(); err == nil || err.Error() != wantTNSW {
		t.Errorf("TNSW routes: err = %v, want %q", err, wantTNSW)
	}

	agency := newScopeGates(scopes.NewResolver(nil), requireAny(&required))
	mountAgency(http.NewServeMux(), nil, nil, nil, nil, nil, stubAuth, agency.withScope)
	const wantAgency = "catalog.scopes must name the scope for: consignmentRead, workflowInject"
	if err := agency.err(); err == nil || err.Error() != wantAgency {
		t.Errorf("agency routes: err = %v, want %q", err, wantAgency)
	}

	if len(required) != 0 {
		t.Errorf("required %v, want no scope checked while catalog.scopes names none", required)
	}
}

// A route requires the scope catalog.scopes names for its logical scope.
func TestScopeGates_RequireTheNamedScope(t *testing.T) {
	var required []string
	gates := newScopeGates(scopes.NewResolver(map[string]string{"consignmentFlowWrite": "flows:start"}), requireAny(&required))

	gates.withScope(scopes.ConsignmentFlowWrite)

	if len(required) != 1 || required[0] != "flows:start" {
		t.Errorf("required %v, want [flows:start]", required)
	}
	if err := gates.err(); err != nil {
		t.Errorf("err = %v, want nil when catalog.scopes names every scope wired", err)
	}
}

// A route whose scope catalog.scopes leaves out admits nobody.
func TestScopeGates_AnUnsetScopeAdmitsNobody(t *testing.T) {
	var required []string
	gates := newScopeGates(scopes.NewResolver(nil), requireAny(&required))
	reached := false
	h := gates.withScope(scopes.TaskRead)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t1", nil))

	if rec.Code != http.StatusForbidden || reached {
		t.Errorf("status %d, reached %v; want 403 and the handler not reached", rec.Code, reached)
	}
}
