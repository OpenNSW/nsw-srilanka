package payment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/OpenNSW/core/refid"
	"github.com/shopspring/decimal"
)

// Fees are the fees PAYMENT steps charge, read from the "fees" list of
// payment_methods.json. A step's artifact names its fee by task code; what it
// charges, and how the gateway and the payment reference are set up for it,
// are this deployment's to decide.
type Fees struct {
	Fees []Fee `json:"fees"`
}

// Fee is one fee a PAYMENT step charges.
type Fee struct {
	// TaskCode names the fee. A PAYMENT step's plugin_properties.task_code
	// selects it, and it is stored on the transaction.
	TaskCode string `json:"task_code"`

	// ServiceName is what the payer is shown they are paying for.
	ServiceName string `json:"service_name"`

	// Amount is charged unless the step is given an "amount" input, which
	// takes precedence. Optional for a fee whose steps always give one.
	Amount decimal.Decimal `json:"amount"`

	Currency string `json:"currency"`

	// GatewayMetadata holds per-fee values the selected gateway needs, passed
	// through opaquely and stored on the transaction so the gateway can
	// recover them at callback time. Each gateway defines its own keys and
	// vets them before the checkout is persisted.
	GatewayMetadata map[string]string `json:"gateway_metadata"`

	// Reference names the refid format of the fee's payment reference, the
	// reference the payer pays against. A fee without one takes the default
	// format; see RefGenerator.
	Reference *FeeReference `json:"reference,omitempty"`
}

// FeeReference names the refid format of a fee's payment reference and
// supplies the params it expects. Params maps a param to an absolute JSON
// Pointer into the step's inputs, as in REFID_GENERATOR, for values that vary
// per payment. Values gives a param its value as is, for codes fixed per fee.
// A param may be named in only one of the two.
type FeeReference struct {
	Issuer string            `json:"issuer"`
	IDType string            `json:"id_type"`
	Params map[string]string `json:"params,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

// LoadFees reads the fees from the payment methods file at path and checks
// them against the reference formats in formats. The file's gateway methods
// are core/payment's to read; a file without a "fees" list has no fees.
func LoadFees(path string, formats refid.Config) (Fees, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fees{}, fmt.Errorf("payment fees: read %s: %w", path, err)
	}
	var fees Fees
	if err := json.Unmarshal(data, &fees); err != nil {
		return Fees{}, fmt.Errorf("payment fees: parse %s: %w", path, err)
	}
	if err := fees.Validate(formats); err != nil {
		return Fees{}, fmt.Errorf("payment fees: %s: %w", path, err)
	}
	return fees, nil
}

// Validate checks every fee is complete and named once, and that each
// reference format a fee names is defined in formats, so a fee pointing at a
// missing format fails at startup rather than on the first checkout. It cannot
// check a reference's params against the step inputs they point into; a step
// fails on those when it runs.
func (f Fees) Validate(formats refid.Config) error {
	defined := make(map[[2]string]struct{})
	for _, issuer := range formats.Issuers {
		for _, format := range issuer.Formats {
			defined[[2]string{issuer.Issuer, format.IDType}] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(f.Fees))
	for i, fee := range f.Fees {
		code := strings.TrimSpace(fee.TaskCode)
		if code == "" {
			return fmt.Errorf("fees[%d].task_code is required", i)
		}
		if _, dup := seen[code]; dup {
			return fmt.Errorf("fees: task_code %q is declared more than once", code)
		}
		seen[code] = struct{}{}
		if err := fee.validate(defined); err != nil {
			return fmt.Errorf("fee %q: %w", code, err)
		}
	}
	return nil
}

func (f Fee) validate(formats map[[2]string]struct{}) error {
	if strings.TrimSpace(f.ServiceName) == "" {
		return errors.New("service_name is required")
	}
	if strings.TrimSpace(f.Currency) == "" {
		return errors.New("currency is required")
	}
	if f.Amount.IsNegative() {
		return fmt.Errorf("amount must be positive, got %s", f.Amount)
	}
	ref := f.Reference
	if ref == nil {
		return nil
	}
	if strings.TrimSpace(ref.Issuer) == "" || strings.TrimSpace(ref.IDType) == "" {
		return errors.New("reference needs an issuer and an id_type")
	}
	if _, ok := formats[[2]string{ref.Issuer, ref.IDType}]; !ok {
		return fmt.Errorf("reference format %s/%s is not defined in config.yaml refid", ref.Issuer, ref.IDType)
	}
	return nil
}

// Lookup returns the fee named taskCode.
func (f Fees) Lookup(taskCode string) (Fee, bool) {
	taskCode = strings.TrimSpace(taskCode)
	for _, fee := range f.Fees {
		if strings.TrimSpace(fee.TaskCode) == taskCode {
			return fee, true
		}
	}
	return Fee{}, false
}
