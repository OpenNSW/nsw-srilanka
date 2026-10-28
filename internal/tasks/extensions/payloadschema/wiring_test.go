package payloadschema

import (
	"testing"

	"github.com/OpenNSW/core/taskflow/extensions"
)

func TestRegister(t *testing.T) {
	forms := fakeForms{}
	tests := []struct {
		name        string
		nilReg      bool
		formOf      FormOf
		namespaceOf NamespaceOf
		fileOf      FileOf
		wantErr     bool
	}{
		{name: "nil registry", nilReg: true, formOf: forms.form, namespaceOf: namespace("form"), fileOf: noFiles, wantErr: true},
		{name: "no form lookup", namespaceOf: namespace("form"), fileOf: noFiles, wantErr: true},
		{name: "no namespace lookup", formOf: forms.form, fileOf: noFiles, wantErr: true},
		{name: "no file lookup", formOf: forms.form, namespaceOf: namespace("form"), wantErr: true},
		{name: "valid", formOf: forms.form, namespaceOf: namespace("form"), fileOf: noFiles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reg *extensions.Registry
			if !tt.nilReg {
				reg = extensions.NewRegistry()
			}
			if err := Register(reg, tt.formOf, tt.namespaceOf, tt.fileOf); (err != nil) != tt.wantErr {
				t.Fatalf("Register() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegister_Duplicate(t *testing.T) {
	forms := fakeForms{}
	reg := extensions.NewRegistry()
	if err := Register(reg, forms.form, namespace("form"), noFiles); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := Register(reg, forms.form, namespace("form"), noFiles); err == nil {
		t.Fatalf("second Register of %q: want error, got nil", ExtPayloadSchema)
	}
}
