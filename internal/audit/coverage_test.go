package audit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteAuditCoverage(t *testing.T) {
	// Path to app.go
	appPath := filepath.Join("..", "bootstrap", "app.go")
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		t.Skip("app.go not found, skipping coverage test")
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, appPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse app.go: %v", err)
	}

	// List of write routes that are explicitly exempt from middleware wrapping
	// because they are either audited internally (explicit front-end) or are public/unaudited.
	exemptRoutes := map[string]string{
		"POST /api/v1/consignments":                  "audited explicitly inside the handler (HandleCreateConsignment) for rich domain payload",
		"POST /api/v1/payments/{gatewayId}/webhook":  "public webhook, currently unaudited",
		"POST /api/v1/payments/{gatewayId}/validate": "public payment validation, currently unaudited",
		"PUT /api/v1/storage/{key}/content":          "local-dev mock storage endpoint, unaudited",
	}

	var missingCoverage []string

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Look for mux.Handle or mux.HandleFunc calls
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "mux" {
			return true
		}

		if sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc" {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		// First argument is the route pattern
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		pattern := strings.Trim(lit.Value, "`\"")
		parts := strings.SplitN(pattern, " ", 2)
		if len(parts) < 2 {
			return true // Not standard HTTP method + path pattern
		}

		method := parts[0]

		// Only check write methods (POST, PUT, PATCH, DELETE)
		if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
			return true
		}

		// Check if it's explicitly exempt
		if reason, exempt := exemptRoutes[pattern]; exempt {
			t.Logf("Route %q is exempt: %s", pattern, reason)
			return true
		}

		// Check if the handler expression (second argument) wraps it with audit
		handlerExpr := call.Args[1]
		hasAudit := hasAuditWrapper(handlerExpr)

		if !hasAudit {
			missingCoverage = append(missingCoverage, pattern)
		}

		return true
	})

	if len(missingCoverage) > 0 {
		t.Errorf("The following write routes are missing audit coverage (neither wrapped with audit middleware nor exempted):\n- %s\n\nTo resolve, either wrap them with an audit recorder wrapper (e.g. recorder.Wrap) or add them to the exemptRoutes map in this test if they are audited explicitly or are public.",
			strings.Join(missingCoverage, "\n- "))
	}
}

// hasAuditWrapper checks if the expression AST contains references to audit wrappers
func hasAuditWrapper(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		switch node := n.(type) {
		case *ast.Ident:
			if strings.HasSuffix(node.Name, "Wrap") {
				found = true
			}
		case *ast.SelectorExpr:
			if strings.HasSuffix(node.Sel.Name, "Wrap") {
				found = true
			}
		}
		return true
	})
	return found
}
