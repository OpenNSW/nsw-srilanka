package bootstrap

// TNSW mode routes. Build calls mountTNSW when config.yaml sets mode: tnsw (the
// default), in place of the agency's routes in agency.go.

import (
	"net/http"

	"github.com/OpenNSW/core/payment"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda"
	slpawebhook "github.com/OpenNSW/nsw-srilanka/external-integration/slpa/webhook"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// tnswHandlers are the handlers behind TNSW's own routes.
type tnswHandlers struct {
	consignment *consignment.Router
	cha         *cha.Handler
	company     *company.Handler
	payment     *payment.HTTPHandler
	slce        *asycuda.Handler
	slpa        *slpawebhook.Handler
}

// mountTNSW registers TNSW's own routes: consignments and their admin views, the
// trader portal's CHA/company lookups, and the payment, customs and port callbacks.
// The routes both modes serve are registered in Build.
func mountTNSW(
	mux *http.ServeMux,
	h tnswHandlers,
	withAuth func(http.Handler) http.Handler,
	withScope func(string) func(http.Handler) http.Handler,
) {
	mux.Handle("GET /api/v1/chas", withAuth(withScope(scopes.CHARead)(http.HandlerFunc(h.cha.HandleGetCHAs))))
	mux.Handle("GET /api/v1/companies", withAuth(withScope(scopes.CompanyRead)(http.HandlerFunc(h.company.HandleGetCompanies))))
	mux.Handle("POST /api/v1/consignments", withAuth(withScope(scopes.ConsignmentWrite)(http.HandlerFunc(h.consignment.HandleCreateConsignment))))
	mux.Handle("GET /api/v1/consignments/{id}/agency", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(h.consignment.HandleGetConsignmentAgency))))
	mux.Handle("GET /api/v1/consignments/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(h.consignment.HandleGetConsignmentByID))))
	mux.Handle("GET /api/v1/consignments", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(h.consignment.HandleGetConsignments))))

	// Ops/admin views of consignment data — gated behind the dedicated admin scope, not the
	// trader/CHA-facing consignment read scope, and with no per-consignment ownership check.
	// Kept together (and as their own handlers, not scope branches on the routes above) so this
	// distinct trust boundary — a small admin group that can read any consignment — stays easy
	// to audit as a group rather than spread through the general consignment API.
	mux.Handle("GET /api/v1/admin/consignments/{id}/engine-status", withAuth(withScope(scopes.ConsignmentAdminRead)(http.HandlerFunc(h.consignment.HandleGetConsignmentEngineStatus))))
	mux.Handle("GET /api/v1/admin/consignments/{id}", withAuth(withScope(scopes.ConsignmentAdminRead)(http.HandlerFunc(h.consignment.HandleAdminGetConsignmentByID))))
	// A TASK node's independent per-task ("micro") workflow — separate ID space and
	// workflow.Manager from the consignment/child-workflow route above (see
	// EngineNodeDTO.TaskWorkflowID).
	mux.Handle("GET /api/v1/admin/task/{id}/engine-status", withAuth(withScope(scopes.ConsignmentAdminRead)(http.HandlerFunc(h.consignment.HandleGetTaskWorkflowEngineStatus))))
	// Resolving a parked node can mutate workflow data (GlobalVariablesPatch) or force it down a path the
	// interpreter never chose (Complete/Abort), so this sits behind ConsignmentAdminWrite, a
	// stricter scope than the read-only admin views above.
	mux.Handle("POST /api/v1/admin/consignments/{id}/steps/{stepId}/resolve", withAuth(withScope(scopes.ConsignmentAdminWrite)(http.HandlerFunc(h.consignment.HandleResolveAdminIntervention))))
	// Same, for a node inside a task workflow, which lives in its own ID space on the task workflow
	// manager (mirrors the two engine-status routes above).
	mux.Handle("POST /api/v1/admin/task/{id}/steps/{stepId}/resolve", withAuth(withScope(scopes.ConsignmentAdminWrite)(http.HandlerFunc(h.consignment.HandleResolveTaskWorkflowAdminIntervention))))

	// Payment webhook endpoints. Requires valid JWT issued from nsw-srilanka's IDP with the appropriate scope. The gatewayId path param is used to resolve the correct payment gateway configuration for the webhook.
	// Authenticating the caller as the gateway itself is the gateway's own job:
	// core/payment calls PaymentGateway.VerifyWebhook before any reference lookup
	// or settlement, so each gateway checks the scheme it actually uses.
	mux.Handle("POST /api/v1/payments/{gatewayId}/webhook", withAuth(withScope(scopes.PaymentWebhooksProcess)(http.HandlerFunc(h.payment.HandleWebhook))))
	mux.Handle("POST /api/v1/payments/{gatewayId}/validate", withAuth(withScope(scopes.PaymentWebhooksValidate)(http.HandlerFunc(h.payment.HandleValidateReference))))

	// SLCE Webhook Endpoint (single central route handling all ASYCUDA/SLCE events).
	mux.Handle("POST /webhooks/slce", withAuth(withScope(scopes.SLCEWebhooksWrite)(http.HandlerFunc(h.slce.HandleWebhook))))

	// SLPA Webhook Endpoint. Authenticated by the HMAC signature on the request
	// itself — see slpa.VerifySignature — so no token middleware here.
	mux.Handle("POST /webhooks/slpa", http.HandlerFunc(h.slpa.HandleWebhook))
}
