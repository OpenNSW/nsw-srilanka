package bootstrap

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/OpenNSW/core/httputil"

	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
)

// scopeGates gives each route the check for the scope catalog.scopes names for its
// logical scope, and records the logical scopes catalog.scopes leaves out. The routes
// a mode wires decide which scopes it requires, so Build checks err once every
// route is wired and refuses to start while any is unset.
type scopeGates struct {
	scopes  scopes.Resolver
	require func(scope string) func(http.Handler) http.Handler
	unset   map[scopes.Name]bool
}

// newScopeGates returns the scopeGates for resolver, checking each named scope with
// require (an authz.Authorizer's RequireScope).
func newScopeGates(resolver scopes.Resolver, require func(scope string) func(http.Handler) http.Handler) *scopeGates {
	return &scopeGates{scopes: resolver, require: require, unset: map[scopes.Name]bool{}}
}

// withScope returns the middleware requiring the scope catalog.scopes names for
// name; compose it after withAuth so the authn context is already injected when the
// scope check runs. A route whose scope catalog.scopes leaves out admits nobody.
func (g *scopeGates) withScope(name scopes.Name) func(http.Handler) http.Handler {
	scope, ok := g.scopes.Of(name)
	if !ok {
		g.unset[name] = true
		return refuseAll
	}
	return g.require(scope)
}

// err reports the logical scopes the wired routes require that catalog.scopes does
// not name, or nil when it names them all.
func (g *scopeGates) err() error {
	if len(g.unset) == 0 {
		return nil
	}
	names := make([]string, 0, len(g.unset))
	for name := range g.unset {
		names = append(names, string(name))
	}
	slices.Sort(names)
	return fmt.Errorf("catalog.scopes must name the scope for: %s", strings.Join(names, ", "))
}

// refuseAll is the middleware of a route whose scope catalog.scopes does not name:
// no token carries a scope the deployment never named, so it admits nobody.
func refuseAll(http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httputil.Error(w, r, http.StatusForbidden, "forbidden")
	})
}
