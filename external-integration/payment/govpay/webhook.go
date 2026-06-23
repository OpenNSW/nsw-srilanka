package govpay

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
)

// ParseWebhook decodes a GovPay+ update (payment-completion) notification into a
// domain-neutral WebhookPayload plus the GovPay+ UpdateResponse acknowledgement
// (the paymentData receipt) to relay back once the notification is accepted. The
// reference, status, amount and currency travel as named data[] items so the
// service layer can verify them before marking the transaction paid; the
// acknowledgement echoes the request's protocol fields and submitted data[] and
// is independent of the settlement outcome.
func (g *GovPayGateway) ParseWebhook(ctx context.Context, body []byte, headers map[string][]string) (*corepayment.WebhookPayload, *corepayment.WebhookResponse, error) {
	req, err := parseGovPayRequest(body)
	if err != nil {
		return nil, nil, err
	}
	params := indexParams(req.Data)

	refNo := strings.TrimSpace(stringParam(params, "refno"))
	if refNo == "" {
		return nil, nil, fmt.Errorf("refNo is required in webhook payload")
	}

	// Status is required and is normalized against GovPay's vocabulary; an
	// absent or unknown status is rejected rather than assumed successful.
	status, err := mapGovPayStatus(stringParam(params, "status", "paymentstatus"))
	if err != nil {
		return nil, nil, err
	}

	payload := &corepayment.WebhookPayload{
		ReferenceNumber:      refNo,
		Status:               status,
		PaymentMethod:        stringParam(params, "paymentmethod"),
		GatewayTransactionID: firstNonEmpty(stringParam(params, "gatewaytransactionid"), req.TransactionID),
		Currency:             stringParam(params, "currency"),
		Timestamp:            stringParam(params, "timestamp"),
	}

	if p, ok := params["amount"]; ok {
		amount, err := paramDecimal(p.Value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid amount in webhook payload: %w", err)
		}
		payload.Amount = amount
	}

	// Build the GovPay+ UpdateResponse acknowledgement (paymentData receipt) from
	// the request itself — it echoes the submitted data[] and is the same
	// regardless of how the service settles the transaction.
	ack := UpdateResponse{
		TransactionID: req.TransactionID,
		SubInstID:     req.SubInstID,
		ServiceID:     req.ServiceID,
		ServiceName:   req.ServiceName,
		Message:       "Success",
		PaymentData:   buildPaymentData(req.Data, req.TransactionID),
	}
	ackBody, err := json.Marshal(ack)
	if err != nil {
		return nil, nil, err
	}

	return payload, &corepayment.WebhookResponse{Payload: ackBody, HTTPStatus: 200}, nil
}

// mapGovPayStatus normalizes GovPay's status vocabulary into the canonical
// corepayment.WebhookStatus. Unknown values are rejected rather than silently
// stored.
func mapGovPayStatus(raw string) (corepayment.WebhookStatus, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SUCCESS", "PAID", "COMPLETED":
		return corepayment.WebhookStatusSuccess, nil
	case "FAILED", "DECLINED", "REJECTED":
		return corepayment.WebhookStatusFailed, nil
	case "PENDING", "INITIATED":
		return corepayment.WebhookStatusPending, nil
	default:
		return "", fmt.Errorf("govpay status %q: %w", raw, corepayment.ErrUnsupportedWebhookStatus)
	}
}

// -----------------------------------------------------------------------------
// paymentData builders
// -----------------------------------------------------------------------------

// buildPaymentData echoes the submitted data[] back to GovPay+ and appends a
// receipt number and status, mirroring the GovPay+ update receipt.
func buildPaymentData(params []govPayParam, transactionID string) []PaymentItem {
	items := make([]PaymentItem, 0, len(params)+2)
	for i, param := range params {
		seq := strings.TrimSpace(param.Seq)
		if seq == "" {
			seq = strconv.Itoa(i + 1)
		}
		paramName := strings.TrimSpace(param.ParamName)
		if paramName == "" {
			paramName = fmt.Sprintf("param_%d", i+1)
		}

		items = append(items, newPaymentItem(i+1, seq, paramName, param.Value, valueDataType(param.Value)))
	}

	items = append(items, newPaymentItem(len(items)+1, strconv.Itoa(len(items)+1), "Receipt Number", fmt.Sprintf("REC-%s", transactionID), "text"))
	items = append(items, newPaymentItem(len(items)+1, strconv.Itoa(len(items)+1), "Status", "Payment recorded", "text"))

	return items
}

func newPaymentItem(idx int, seq, placeholder string, initialValue interface{}, dataType string) PaymentItem {
	return PaymentItem{
		ObjType:       "label",
		Seq:           seq,
		ID:            fmt.Sprintf("%03d%04d", idx, idx),
		Placeholder:   placeholder,
		InitialValue:  initialValue,
		DataType:      dataType,
		MaxLength:     50,
		SelectionType: "SINGLE",
		Mask:          "",
		NotNull:       "true",
		Enabled:       "false",
		Returned:      "false",
		Rows:          1,
		Cols:          1,
		ReturnParam:   "",
		ReturnValue:   "",
	}
}

func valueDataType(value interface{}) string {
	switch value.(type) {
	case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number, decimal.Decimal:
		return "decimal"
	default:
		return "text"
	}
}
