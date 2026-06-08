package renderer

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"text/template"

	"github.com/OpenNSW/core/uiprojector"
	"github.com/OpenNSW/nsw/backend/internal/payments"
)

// ProjectorPayment defines the payment instructions projector type.
const ProjectorPayment uiprojector.ProjectorType = "PAYMENT"

// PaymentProjector dynamically templates payment instructions from the payment service.
type PaymentProjector struct {
	paymentService payments.PaymentService
	tmplCache      sync.Map // map[methodID string]*template.Template
}

// NewPaymentProjector creates a new PaymentProjector.
func NewPaymentProjector(paymentService payments.PaymentService) *PaymentProjector {
	return &PaymentProjector{
		paymentService: paymentService,
	}
}

// Type returns the projector type.
func (p *PaymentProjector) Type() uiprojector.ProjectorType {
	return ProjectorPayment
}

// Project resolves the selected payment method's instructions template and renders it.
// When the payment facts haven't been populated yet (entering PENDING_PAYMENT before
// CreateCheckoutSession completes), Project returns a placeholder markdown projection
// so the page still loads instead of 500ing.
func (p *PaymentProjector) Project(ctx context.Context, templateContent []byte, data any) (uiprojector.Projection, error) {
	if data == nil {
		return uiprojector.Projection{
			Type:    uiprojector.SectionTypeMarkdown,
			Content: "Preparing payment details...",
		}, nil
	}
	dataMap, ok := data.(map[string]any)
	if !ok {
		return uiprojector.Projection{}, fmt.Errorf("payment_projector: expected map data, got %T", data)
	}

	selectedMethod, _ := dataMap["selected_method"].(string)
	if selectedMethod == "" {
		selectedMethod = "lankapay"
	}

	method, err := p.paymentService.GetPaymentMethod(selectedMethod)
	if err != nil {
		return uiprojector.Projection{}, fmt.Errorf("payment_projector: get payment method %q: %w", selectedMethod, err)
	}

	var tmpl *template.Template
	if cached, ok := p.tmplCache.Load(selectedMethod); ok {
		tmpl = cached.(*template.Template)
	} else {
		parsed, err := template.New("instructions").Parse(method.Template)
		if err != nil {
			return uiprojector.Projection{}, fmt.Errorf("payment_projector: parse template: %w", err)
		}
		p.tmplCache.Store(selectedMethod, parsed)
		tmpl = parsed
	}

	orgName, _ := dataMap["org_name"].(string)
	tmplData := map[string]any{
		"ReferenceNumber":  dataMap["reference_number"],
		"Amount":           dataMap["amount"],
		"Currency":         dataMap["currency"],
		"CheckoutURL":      dataMap["checkout_url"],
		"ServiceName":      dataMap["service_name"],
		"ServiceType":      dataMap["service_type"],
		"OrganizationName": orgName,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
		return uiprojector.Projection{}, fmt.Errorf("payment_projector: execute template: %w", err)
	}

	if method.Type == "REDIRECT" {
		checkoutURL, _ := dataMap["checkout_url"].(string)
		return uiprojector.Projection{
			Type: uiprojector.SectionType("REDIRECT"),
			Content: map[string]any{
				"checkout_url": checkoutURL,
				"content":      buf.String(),
			},
		}, nil
	}

	return uiprojector.Projection{
		Type:    uiprojector.SectionTypeMarkdown,
		Content: buf.String(),
	}, nil
}
