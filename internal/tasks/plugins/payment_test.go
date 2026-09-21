package plugins

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestResolvePaymentAmount(t *testing.T) {
	configured := decimal.NewFromFloat(12500.00)

	tests := []struct {
		name       string
		input      any
		configured decimal.Decimal
		want       string
		wantErr    bool
	}{
		{
			name:       "workflow-supplied float64 overrides configured amount",
			input:      4500.50,
			configured: configured,
			want:       "4500.5",
		},
		{
			name:       "workflow-supplied string amount is accepted",
			input:      "4500.50",
			configured: configured,
			want:       "4500.5",
		},
		{
			name:       "no input falls back to configured amount",
			input:      nil,
			configured: configured,
			want:       "12500",
		},
		{
			name:       "no input and no configured amount is an error",
			input:      nil,
			configured: decimal.Decimal{},
			wantErr:    true,
		},
		{
			name:       "zero workflow-supplied amount is rejected even with a valid configured fallback",
			input:      0.0,
			configured: configured,
			wantErr:    true,
		},
		{
			name:       "negative workflow-supplied amount is rejected",
			input:      -100.0,
			configured: configured,
			wantErr:    true,
		},
		{
			name:       "unparseable string amount is rejected",
			input:      "not-a-number",
			configured: configured,
			wantErr:    true,
		},
		{
			name:       "decimal.Decimal is never a valid input type",
			input:      decimal.NewFromFloat(4500.50),
			configured: configured,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePaymentAmount(tt.input, tt.configured)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolvePaymentAmount(%v, %v) = %v, want error", tt.input, tt.configured, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePaymentAmount(%v, %v) unexpected error: %v", tt.input, tt.configured, err)
			}
			if got.String() != tt.want {
				t.Fatalf("resolvePaymentAmount(%v, %v) = %s, want %s", tt.input, tt.configured, got.String(), tt.want)
			}
		})
	}
}
