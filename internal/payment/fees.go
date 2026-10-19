package payment

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Fees are the per-deployment settings of the fees PAYMENT steps charge, by
// the id of the payment method that collects them. Each method lists its own
// in a "fees" list in its config in payment_methods.json. What a fee charges
// is the artifact's; what the gateway needs to collect it, such as the
// account it is paid into, differs per environment and is set here.
type Fees map[string][]Fee

// Fee is one fee's settings for the method that collects it.
type Fee struct {
	// TaskCode names the fee: the plugin_properties.task_code of the PAYMENT
	// steps that charge it.
	TaskCode string `json:"task_code"`

	// GatewayMetadata holds per-fee values the gateway needs, passed through
	// opaquely and stored on the transaction so the gateway can recover them
	// at callback time. Each gateway defines its own keys and vets them before
	// the checkout is persisted.
	GatewayMetadata map[string]string `json:"gateway_metadata"`
}

// methodsFile is the slice of payment_methods.json LoadFees reads: each
// method's id and the fees in its config. The rest of a method's config is its
// gateway's to read.
type methodsFile struct {
	Methods []struct {
		ID     string `json:"id"`
		Config struct {
			Fees []Fee `json:"fees"`
		} `json:"config"`
	} `json:"methods"`
}

// LoadFees reads each payment method's fees from the payment methods file at
// path. A method without a "fees" list in its config has none.
func LoadFees(path string) (Fees, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("payment fees: read %s: %w", path, err)
	}
	var file methodsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("payment fees: parse %s: %w", path, err)
	}
	fees := make(Fees, len(file.Methods))
	for _, method := range file.Methods {
		if len(method.Config.Fees) > 0 {
			fees[method.ID] = method.Config.Fees
		}
	}
	if err := fees.Validate(); err != nil {
		return nil, fmt.Errorf("payment fees: %s: %w", path, err)
	}
	return fees, nil
}

// Validate checks every fee names its task code, once per method.
func (f Fees) Validate() error {
	for method, fees := range f {
		seen := make(map[string]struct{}, len(fees))
		for i, fee := range fees {
			code := strings.TrimSpace(fee.TaskCode)
			if code == "" {
				return fmt.Errorf("method %q: fees[%d].task_code is required", method, i)
			}
			if _, dup := seen[code]; dup {
				return fmt.Errorf("method %q: task_code %q is declared more than once", method, code)
			}
			seen[code] = struct{}{}
		}
	}
	return nil
}

// GatewayMetadata returns the gateway metadata the method with id method
// collects the fee taskCode with, and whether that method lists the fee.
func (f Fees) GatewayMetadata(method, taskCode string) (map[string]string, bool) {
	taskCode = strings.TrimSpace(taskCode)
	for _, fee := range f[method] {
		if strings.TrimSpace(fee.TaskCode) == taskCode {
			return fee.GatewayMetadata, true
		}
	}
	return nil, false
}
