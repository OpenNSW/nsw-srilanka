package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCatalog() *Catalog {
	return &Catalog{
		Roles:   map[string]string{"trader": "Trader", "cha": "CHA"},
		Clients: map[string]string{"fcau": "FCAU_TO_NSW"},
	}
}

func TestCatalog_Validate(t *testing.T) {
	tests := []struct {
		name    string
		catalog *Catalog
		wantErr bool
	}{
		{name: "nil", catalog: nil, wantErr: true},
		{name: "empty", catalog: &Catalog{}, wantErr: true},
		{name: "role missing token role", catalog: &Catalog{Roles: map[string]string{"trader": ""}}, wantErr: true},
		{name: "client missing id", catalog: &Catalog{Clients: map[string]string{"fcau": ""}}, wantErr: true},
		{name: "valid roles only", catalog: &Catalog{Roles: map[string]string{"trader": "Trader"}}, wantErr: false},
		{name: "valid clients only", catalog: &Catalog{Clients: map[string]string{"fcau": "FCAU_TO_NSW"}}, wantErr: false},
		{name: "valid full", catalog: testCatalog(), wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.catalog.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	valid := write("ok.json", `{"roles":{"trader":"Trader"},"clients":{"fcau":"FCAU_TO_NSW"}}`)
	c, err := Load(valid)
	if err != nil || c == nil {
		t.Fatalf("Load(valid) = %v, %v", c, err)
	}
	if c.Roles["trader"] != "Trader" || c.Clients["fcau"] != "FCAU_TO_NSW" {
		t.Fatalf("Load(valid) parsed %+v", c)
	}

	// The catalog is meant to grow sections ahead of the code reading them, so an
	// unknown top-level key must not break existing consumers.
	forward := write("forward.json", `{"roles":{"trader":"Trader"},"clients":{"fcau":"FCAU_TO_NSW"},"agencies":{"customs":{"code":"CUSTOMS"}}}`)
	if c, err := Load(forward); err != nil || c.Roles["trader"] != "Trader" {
		t.Fatalf("Load(unknown section) = %v, %v; want the known sections to parse", c, err)
	}

	bad := write("bad.json", `{not json`)
	if _, err := Load(bad); err == nil {
		t.Fatal("Load(malformed) want error, got nil")
	}

	missing := filepath.Join(dir, "missing.json")
	err = func() error { _, e := Load(missing); return e }()
	if err == nil {
		t.Fatal("Load(missing) want error, got nil")
	}
	// The path is the only thing that makes a config failure actionable.
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("Load(missing) error %q does not name the path %q", err, missing)
	}

	invalid := write("invalid.json", `{"roles":{},"clients":{}}`)
	err = func() error { _, e := Load(invalid); return e }()
	if err == nil {
		t.Fatal("Load(empty catalog) want validation error, got nil")
	}
	if !strings.Contains(err.Error(), invalid) {
		t.Errorf("Load(invalid) error %q does not name the path %q", err, invalid)
	}
}

// RequireRoles is the shared precondition for callers that depend on specific
// logical names — internal/consignment and internal/tasks/authzgate both scope
// behavior by "trader"/"cha", and for them a missing name is a silent denial of
// every request in that role rather than a narrower catalog.
func TestRequireRoles(t *testing.T) {
	tests := []struct {
		name    string
		roles   map[string]string
		require []string
		wantErr string // substring expected in the error; "" means no error
	}{
		{name: "both present", roles: map[string]string{"trader": "Trader", "cha": "CHA"}, require: []string{"trader", "cha"}},
		{name: "extra roles ignored", roles: map[string]string{"trader": "Trader", "cha": "CHA", "fcau": "FCAU_TO_NSW"}, require: []string{"trader", "cha"}},
		{name: "requiring nothing always passes", roles: nil},
		{name: "missing cha", roles: map[string]string{"trader": "Trader"}, require: []string{"trader", "cha"}, wantErr: "cha"},
		{name: "missing trader", roles: map[string]string{"cha": "CHA"}, require: []string{"trader", "cha"}, wantErr: "trader"},
		{name: "missing both reported in order", roles: map[string]string{}, require: []string{"trader", "cha"}, wantErr: "trader, cha"},
		{name: "nil map", roles: nil, require: []string{"trader", "cha"}, wantErr: "trader, cha"},
		// An empty mapping can never match a real JWT role claim, so honoring it
		// would let a comparison against "" wrongly succeed.
		{name: "present but empty is missing", roles: map[string]string{"trader": "Trader", "cha": ""}, require: []string{"trader", "cha"}, wantErr: "cha"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireRoles(tc.roles, tc.require...)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("RequireRoles(%v, %v) = %v, want nil", tc.roles, tc.require, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("RequireRoles(%v, %v) = %v, want error containing %q", tc.roles, tc.require, err, tc.wantErr)
			}
		})
	}
}
