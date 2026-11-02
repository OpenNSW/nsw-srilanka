// Package scopes names the OAuth2 scopes this backend's routes require, in either
// mode.
//
// A route requires a logical scope, a Name, never a scope string: each deployment
// names the scope its IdP grants for it in catalog.scopes of config.yaml, and there
// is no default. Validate checks those entries when the config loads, and a Resolver
// gives the scope each route checks.
//
// Names are defined here (not in internal/authz) so they can be imported from the
// composition root, tests, and any future service-layer checks without coupling the
// generic authz package to this application.
package scopes

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
)

// Name is a logical scope: the key a deployment sets in catalog.scopes.
type Name string

const (
	// Consignment resource.
	ConsignmentRead Name = "consignmentRead"
	// ConsignmentWrite grants creating a consignment, which only traders do.
	ConsignmentWrite Name = "consignmentWrite"
	// ConsignmentAdminRead grants ops/admin visibility into any consignment's raw
	// workflow-engine state, unlike ConsignmentRead which is scoped to the caller's
	// own trader/CHA-owned consignments.
	ConsignmentAdminRead Name = "consignmentAdminRead"
	// ConsignmentAdminWrite grants ops/admin the ability to resolve a node parked
	// in AWAITING_ADMIN (RETRY/COMPLETE/ABORT) on any consignment's workflow.
	ConsignmentAdminWrite Name = "consignmentAdminWrite"

	// Consignment flow resource: the auxiliary flows within a consignment.
	// ConsignmentFlowWrite grants starting a configured flow on a consignment, which
	// its trader and its CHA both do.
	ConsignmentFlowWrite Name = "consignmentFlowWrite"

	// Task resource.
	TaskRead  Name = "taskRead"
	TaskWrite Name = "taskWrite"

	// Reference data (read-only).
	CompanyRead Name = "companyRead"
	CHARead     Name = "chaRead"

	// Profile resource (the caller's own user profile).
	ProfileRead Name = "profileRead"

	// Storage resource.
	StorageRead  Name = "storageRead"
	StorageWrite Name = "storageWrite"
	// StorageInternalWrite is the upload scope for machine clients, on the
	// internal upload route; StorageWrite is for users.
	StorageInternalWrite Name = "storageInternalWrite"

	// SLCE webhook resource.
	SLCEWebhooksWrite Name = "slceWebhooksWrite"

	// Payment webhook resource.
	PaymentWebhooksValidate Name = "paymentWebhooksValidate"
	PaymentWebhooksProcess  Name = "paymentWebhooksProcess"

	// WorkflowInject gates an agency's POST /api/v1/inject. An inject starts one
	// workflow (and creates or joins its case as a side effect), so the scope names
	// the workflow. It is an M2M permission, kept apart from the officer's task scopes.
	WorkflowInject Name = "workflowInject"
)

// names are the Names a deployment may set in catalog.scopes.
var names = []Name{
	ConsignmentRead, ConsignmentWrite, ConsignmentAdminRead, ConsignmentAdminWrite, ConsignmentFlowWrite,
	TaskRead, TaskWrite,
	CompanyRead, CHARead,
	ProfileRead,
	StorageRead, StorageWrite, StorageInternalWrite,
	SLCEWebhooksWrite,
	PaymentWebhooksValidate, PaymentWebhooksProcess,
	WorkflowInject,
}

// Validate reports an error unless every key of entries, catalog.scopes, is a Name
// and every value a single scope: not empty, and without whitespace, which
// separates scopes in a token. Messages are unprefixed so the config can qualify
// them with the section.
func Validate(entries map[string]string) error {
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		if !slices.Contains(names, Name(key)) {
			return fmt.Errorf("scopes[%q] is not a scope this backend's routes require", key)
		}
		scope := entries[key]
		if scope == "" || strings.IndexFunc(scope, unicode.IsSpace) >= 0 {
			return fmt.Errorf("scopes[%q] must map to one scope, with no whitespace", key)
		}
	}
	return nil
}

// Resolver gives the scope this deployment names for each Name.
type Resolver struct {
	entries map[string]string
}

// NewResolver returns the Resolver for entries, the catalog.scopes of a config
// Validate accepted.
func NewResolver(entries map[string]string) Resolver {
	return Resolver{entries: maps.Clone(entries)}
}

// Of returns the scope catalog.scopes names for name, and whether it names one.
func (r Resolver) Of(name Name) (string, bool) {
	scope, ok := r.entries[string(name)]
	return scope, ok
}
