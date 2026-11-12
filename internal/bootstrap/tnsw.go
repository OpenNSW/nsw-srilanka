package bootstrap

// TNSW mode routes. Build calls mountTNSW when config.yaml sets mode: tnsw (the
// default), in place of the agency's routes in agency.go.

import (
	"context"
	"net/http"

	"github.com/OpenNSW/core/authz"
	"github.com/OpenNSW/core/payment"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda"
	slpawebhook "github.com/OpenNSW/nsw-srilanka/external-integration/slpa/webhook"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
)

// tnswHandlers are the handlers behind TNSW's own routes.
type tnswHandlers struct {
	consignment *consignment.Router
	company     *company.Handler
	payment     *payment.HTTPHandler
	slce        *asycuda.Handler
	slpa        *slpawebhook.Handler
	files       *nswstorage.FileHandler
}

// mountTNSW registers TNSW's own routes: machine uploads, consignments and their admin views, the
// trader portal's company lookups, and the payment, customs and port callbacks.
// The routes both modes serve are registered in Build.
func mountTNSW(
	mux *http.ServeMux,
	h tnswHandlers,
	withAuth func(http.Handler) http.Handler,
	withScope func(string) func(http.Handler) http.Handler,
) {
	// Uploads by machine clients, such as an agency forwarding its officers'
	// files. Not served when this deployment has no storage of its own.
	if h.files.HasMachineUploads() {
		mux.Handle(nswstorage.InternalUploadRoute, withAuth(withScope(scopes.StorageInternalWrite)(http.HandlerFunc(h.files.InternalUpload))))
	}

	mux.Handle("GET /api/v1/companies", withAuth(withScope(scopes.CompanyRead)(http.HandlerFunc(h.company.HandleGetCompanies))))
	mux.Handle("POST /api/v1/consignments", withAuth(withScope(scopes.ConsignmentWrite)(http.HandlerFunc(h.consignment.HandleCreateConsignment))))
	mux.Handle("GET /api/v1/consignments/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(h.consignment.HandleGetConsignmentByID))))
	mux.Handle("POST /api/v1/consignments/{id}/flows", withAuth(withScope(scopes.ConsignmentFlowWrite)(http.HandlerFunc(h.consignment.HandleStartConsignmentFlow))))
	mux.Handle("GET /api/v1/consignments", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(h.consignment.HandleGetConsignments))))

	// The admin view of a consignment, behind the admin scope and with no ownership check: it
	// lists the consignment's flows, which the shared admin routes (see Build) open by
	// workflow ID.
	mux.Handle("GET /api/v1/admin/consignments/{id}", withAuth(withScope(scopes.ConsignmentAdminRead)(http.HandlerFunc(h.consignment.HandleAdminGetConsignmentByID))))

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

// canStartConsignmentFlows checks the scope required by the flow start route.
func canStartConsignmentFlows(ctx context.Context) bool {
	p, ok := authn.ScopePrincipalFromContext(ctx)
	return ok && authz.HasScope(p, scopes.ConsignmentFlowWrite)
}
