package authz

import (
	"testing"

	"github.com/OpenNSW/core/taskflow/extensions"
)

func TestRegister(t *testing.T) {
	tests := []struct {
		name    string
		nilReg  bool
		catalog Catalog
		wantErr bool
	}{
		{name: "nil registry", nilReg: true, catalog: testCatalog(), wantErr: true},
		{name: "empty catalog", catalog: Catalog{}, wantErr: true},
		{name: "roles only", catalog: Catalog{Roles: map[string]string{"trader": "Trader"}}, wantErr: false},
		{name: "clients only", catalog: Catalog{Clients: map[string]string{"fcau": "FCAU_TO_NSW"}}, wantErr: false},
		{name: "valid full", catalog: testCatalog(), wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reg *extensions.Registry
			if !tc.nilReg {
				reg = extensions.NewRegistry()
			}
			if err := Register(reg, tc.catalog); (err != nil) != tc.wantErr {
				t.Fatalf("Register() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestRegister_Duplicate(t *testing.T) {
	reg := extensions.NewRegistry()
	if err := Register(reg, testCatalog()); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := Register(reg, testCatalog()); err == nil {
		t.Fatalf("second Register of %q: want error, got nil", ExtAuthz)
	}
}
