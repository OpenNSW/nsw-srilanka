// Package scopes defines the NSW API OAuth2 scope constants.
//
// These values must stay in sync with the NSW_API resource server defined in
// idp/bootstrap/10-nsw-platform.yaml, and with the trader portal's
// portals/apps/trader-app/src/constants/scopes.ts. Each constant corresponds to a permission
// derived from a resource + action handle in the IdP, following the pattern
// "<resource>:<action>".
//
// Scope constants are defined here (not in internal/authz) so they can be
// imported from the composition root, tests, and any future service-layer
// checks without coupling the generic authz package to this application.
package scopes

const (
	// Consignment resource.
	ConsignmentRead = "consignment:read"
	// ConsignmentWrite grants creating a consignment, which only traders do.
	ConsignmentWrite = "consignment:write"
	// ConsignmentAdminRead grants ops/admin visibility into any consignment's raw
	// workflow-engine state, unlike ConsignmentRead which is scoped to the caller's
	// own trader/CHA-owned consignments.
	ConsignmentAdminRead = "consignment:adminread"
	// ConsignmentAdminWrite grants ops/admin the ability to resolve a node parked
	// in AWAITING_ADMIN (RETRY/COMPLETE/ABORT) on any consignment's workflow.
	ConsignmentAdminWrite = "consignment:adminwrite"

	// Consignment flow resource: the auxiliary flows within a consignment.
	// ConsignmentFlowWrite grants starting a configured flow on a consignment, which
	// its trader and its CHA both do.
	ConsignmentFlowWrite = "consignment:flow:write"

	// Task resource.
	TaskRead  = "task:read"
	TaskWrite = "task:write"

	// Reference data (read-only).
	HSCodeRead  = "hscode:read"
	CompanyRead = "company:read"
	CHARead     = "cha:read"

	// Profile resource (the caller's own user profile).
	ProfileRead = "profile:read"

	// Storage resource.
	StorageRead  = "storage:read"
	StorageWrite = "storage:write"
	// StorageInternalWrite is the upload scope for machine clients, on the
	// internal upload route; StorageWrite is for users.
	StorageInternalWrite = "storage:internalwrite"

	// SLCE webhook resource.
	SLCEWebhooksWrite = "slce-webhooks:write"

	// Payment webhook resource.
	PaymentWebhooksValidate = "payment-webhooks:validate"
	PaymentWebhooksProcess  = "payment-webhooks:process"
)
