package payloadschema

import (
	"testing"

	"github.com/OpenNSW/core/taskflow/extensions"
)

func TestRegister(t *testing.T) {
	artifacts := fakeArtifacts{}
	tests := []struct {
		name           string
		nilReg         bool
		stepTemplateOf StepTemplateOf
		formOf         FormOf
		wantErr        bool
	}{
		{name: "nil registry", nilReg: true, stepTemplateOf: artifacts.stepTemplate, formOf: artifacts.form, wantErr: true},
		{name: "no step template lookup", formOf: artifacts.form, wantErr: true},
		{name: "no form lookup", stepTemplateOf: artifacts.stepTemplate, wantErr: true},
		{name: "valid", stepTemplateOf: artifacts.stepTemplate, formOf: artifacts.form},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reg *extensions.Registry
			if !tt.nilReg {
				reg = extensions.NewRegistry()
			}
			if err := Register(reg, tt.stepTemplateOf, tt.formOf); (err != nil) != tt.wantErr {
				t.Fatalf("Register() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegister_Duplicate(t *testing.T) {
	artifacts := fakeArtifacts{}
	reg := extensions.NewRegistry()
	if err := Register(reg, artifacts.stepTemplate, artifacts.form); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := Register(reg, artifacts.stepTemplate, artifacts.form); err == nil {
		t.Fatalf("second Register of %q: want error, got nil", ExtPayloadSchema)
	}
}
