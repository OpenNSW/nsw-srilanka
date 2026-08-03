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
