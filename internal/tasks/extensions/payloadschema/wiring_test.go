package payloadschema

import (
	"testing"

	"github.com/OpenNSW/core/taskflow/extensions"
)

func TestRegister(t *testing.T) {
	forms := fakeForms{}
	tests := []struct {
		name    string
		nilReg  bool
		formOf  FormOf
		wantErr bool
	}{
		{name: "nil registry", nilReg: true, formOf: forms.form, wantErr: true},
		{name: "no form lookup", wantErr: true},
		{name: "valid", formOf: forms.form},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reg *extensions.Registry
			if !tt.nilReg {
				reg = extensions.NewRegistry()
			}
			if err := Register(reg, tt.formOf); (err != nil) != tt.wantErr {
				t.Fatalf("Register() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegister_Duplicate(t *testing.T) {
	forms := fakeForms{}
	reg := extensions.NewRegistry()
	if err := Register(reg, forms.form); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := Register(reg, forms.form); err == nil {
		t.Fatalf("second Register of %q: want error, got nil", ExtPayloadSchema)
	}
}
