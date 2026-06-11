package consignment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateConsignmentDTO_Validate(t *testing.T) {
	tests := []struct {
		name    string
		dto     CreateConsignmentDTO
		wantErr bool
	}{
		{
			name: "valid import",
			dto: CreateConsignmentDTO{
				Flow:         FlowImport,
				ChaCompanyID: "cha1",
			},
			wantErr: false,
		},
		{
			name: "valid export",
			dto: CreateConsignmentDTO{
				Flow:         FlowExport,
				ChaCompanyID: "cha2",
			},
			wantErr: false,
		},
		{
			name: "missing chaCompanyId",
			dto: CreateConsignmentDTO{
				Flow: FlowImport,
			},
			wantErr: true,
		},
		{
			name: "invalid flow",
			dto: CreateConsignmentDTO{
				Flow:         "INVALID",
				ChaCompanyID: "cha1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dto.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
