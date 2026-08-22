// Package readauthz is the read-side counterpart of the task-step authorization
// extension: it decides whether the caller may read a task at all, and resolves
// the per-role claims that shape what they see inside it.
//
// It exists as its own package rather than a task extension because core has no
// read hook — extensions only fire on CompleteTaskStep. Like the write-side
// evaluator it is pure: Layer 1 (internal/tasks/authzgate) resolves the caller's
// identity and a lazy ownership resolver into a taskauthz.Input, and this package
// matches that against the catalog and the task's render config. It touches no
// domain service and reads no configuration file.
//
// Eligibility — holding a role and owning the consignment in it — is decided by
// taskauthz, shared with the write path. What this package adds is that a read
// selects *content* rather than deciding one yes/no question, so it cannot simply
// take any match.
//
// One caller can be eligible for more than one role: a self-clearing company
// whose user holds both Trader and CHA is eligible for both on its own
// consignments. Since visibility rules only ever AND, two true claims would
// render two mutually contradictory sections at once ("here is the form" beside
// "waiting for your CHA"). So when a render config declares read.roles, that
// list is a precedence order: the caller acts as the first role in it they are
// eligible for, and exactly one claim is true. A config that declares no
// read.roles has expressed no precedence, so every eligible role's claim is
// reported — safe, because such a config has no per-role sections to disagree.
package readauthz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// ClaimPrefix namespaces ownership claims inside a render config's
// visibleWhen.requireClaim, leaving room for claims that are not about roles.
const ClaimPrefix = "role:"

// ErrDenied means the caller may not read this task. Handlers must translate it
// to a 404 (not a 403), so a denied read is indistinguishable from a task that
// does not exist and cannot be used to probe which task ids are real.
var ErrDenied = errors.New("task read authz: denied")

// ClaimKey returns the claim key a render config uses to gate a section on a
// catalog role. Keys are matched case-sensitively by uiprojector, so this is the
// single place the spelling is decided.
func ClaimKey(role string) string { return ClaimPrefix + role }

// Policy is this application's slice of a render.json blob: which roles may read
// the task. The blob is decoded independently by uiprojector (as a Blueprint) and
// by core's zoneview (for handles and state actions); each parser ignores the
// fields it does not own, so this third decode needs no library change.
type Policy struct {
	Read *ReadPolicy `json:"read"`
}

// ReadPolicy lists the logical catalog roles allowed to read the task, in
// precedence order: a caller eligible for several of them acts as the first one
// listed. An absent Read block, or an empty Roles list, means any role that owns
// the consignment may read it, with no precedence applied.
type ReadPolicy struct {
	Roles []string `json:"roles"`
}

// Evaluator resolves read claims and read decisions against the global catalog.
type Evaluator struct {
	catalog taskauthz.Catalog
}

// NewEvaluator builds an Evaluator. The catalog must define at least one role;
// with none, every claim would be false and every read denied.
func NewEvaluator(cat taskauthz.Catalog) (*Evaluator, error) {
	if len(cat.Roles) == 0 {
		return nil, errors.New("readauthz: catalog defines no roles")
	}
	return &Evaluator{catalog: cat}, nil
}

// Resolve decides whether the caller may read the task and, if so, returns the
// claims that shape their view of it. It returns ErrDenied when the caller may
// not read the task at all; any other error is a real failure, not a denial.
//
// The returned map always carries one entry per catalog role, denials included
// as false: uiprojector treats a claim a blueprint references but the caller
// never populated as a caller bug and fails the whole render, so a partially
// populated map would turn a policy decision into a 500.
func (e *Evaluator) Resolve(ctx context.Context, in taskauthz.Input, renderConfig json.RawMessage, rootWorkflowID string) (map[string]bool, error) {
	allowed, declared, err := e.allowedRoles(ctx, renderConfig)
	if err != nil {
		return nil, err
	}

	// Only the admitted roles can ever yield a true claim, so eligibility is
	// resolved for those alone: a caller holding none of them is denied without a
	// database lookup.
	eligible, err := e.catalog.Eligible(ctx, in, rootWorkflowID, allowed)
	if err != nil {
		return nil, err
	}

	claims := make(map[string]bool, len(e.catalog.Roles))
	for name := range e.catalog.Roles {
		claims[ClaimKey(name)] = false
	}

	if !declared {
		// No precedence was expressed, so report every role the caller is eligible
		// for and admit them if that is any role at all.
		admitted := false
		for _, name := range allowed {
			if eligible.Allows(name) {
				claims[ClaimKey(name)] = true
				admitted = true
			}
		}
		if !admitted {
			return nil, ErrDenied
		}
		return claims, nil
	}

	// read.roles is a precedence order: act as the first listed role the caller is
	// eligible for, so at most one claim is ever true and per-role sections cannot
	// contradict each other.
	for _, name := range allowed {
		if eligible.Allows(name) {
			claims[ClaimKey(name)] = true
			return claims, nil
		}
	}
	return nil, ErrDenied
}

// allowedRoles returns the logical role names the render config admits and
// whether it declared them. When it did, the order is the caller's precedence
// order; when it did not, every catalog role is admitted in no meaningful order.
//
// A name the catalog does not define cannot be owned by anyone, so it is dropped
// and logged — the render-config validator is the place that will turn it into a
// startup failure.
func (e *Evaluator) allowedRoles(ctx context.Context, renderConfig json.RawMessage) ([]string, bool, error) {
	var policy Policy
	if len(renderConfig) > 0 {
		if err := json.Unmarshal(renderConfig, &policy); err != nil {
			return nil, false, fmt.Errorf("readauthz: decode read policy: %w", err)
		}
	}
	if policy.Read == nil || len(policy.Read.Roles) == 0 {
		names := make([]string, 0, len(e.catalog.Roles))
		for name := range e.catalog.Roles {
			names = append(names, name)
		}
		return names, false, nil
	}

	names := make([]string, 0, len(policy.Read.Roles))
	for _, name := range policy.Read.Roles {
		if _, ok := e.catalog.Roles[name]; !ok {
			slog.WarnContext(ctx, "readauthz: render config allows a role the catalog does not define; ignoring it",
				"role", name)
			continue
		}
		names = append(names, name)
	}
	return names, true, nil
}
