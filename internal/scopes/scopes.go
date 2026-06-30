// Package scopes defines the NSW API OAuth2 scope constants.
//
// These values must stay in sync with the NSW_API resource server defined in
// idp/resources/shared/resource-servers.json (nsw:* namespace), seeded by
// idp/sample-resources.sh. Each constant corresponds to
// a permission derived from a resource + action handle in the IdP, following
// the pattern "nsw:<resource>:<action>".
//
// Scope constants are defined here (not in internal/authz) so they can be
// imported from the composition root, tests, and any future service-layer
// checks without coupling the generic authz package to this application.
package scopes

const (
	// Consignment resource.
	ConsignmentRead  = "nsw:consignment:read"
	ConsignmentWrite = "nsw:consignment:write"

	// Task resource.
	TaskRead  = "nsw:task:read"
	TaskWrite = "nsw:task:write"

	// Reference data (read-only).
	HSCodeRead  = "nsw:hscode:read"
	CompanyRead = "nsw:company:read"
	CHARead     = "nsw:cha:read"

	// Storage resource.
	StorageRead   = "nsw:storage:read"
	StorageWrite  = "nsw:storage:write"
	StorageDelete = "nsw:storage:delete"
)
