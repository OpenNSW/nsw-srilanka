package govpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"
)

// HandleValidateReference formats the GovPay+ presentment response. When the
// reference is payable it returns the fields to render (presentmentData);
// otherwise it returns the GovPay+ error envelope with the matching HTTP status
// (404 for an unknown/foreign reference, 409 for one already settled or expired).
func (g *GovPayGateway) HandleValidateReference(ctx context.Context, tx *corepayment.ValidationTransaction, isPayable bool, reqData json.RawMessage) (*corepayment.ValidationResponse, error) {
	req, err := parseGovPayRequest(reqData)
	if err != nil {
		return nil, err
	}

	if tx == nil {
		return jsonValidationResponse(404, ErrorResponse{
			Error:   "invalid_reference",
			Message: "invalid reference number",
		})
	}

	// The reference exists, but check it belongs to the service this call
	// claims to be for before saying anything else about it.
	if err := identityFromMetadata(tx.Metadata).verify(req.SubInstID, req.ServiceID); err != nil {
		// A fee that never declared its ids is our own misconfiguration, not a
		// bad request. Saying so plainly — and loudly — keeps it from being
		// chased as a mystery "unknown reference".
		if errors.Is(err, ErrIdentityNotConfigured) {
			slog.ErrorContext(ctx, "govpay presentment rejected: fee is missing its govpay identity",
				"reference", tx.ReferenceNumber, "error", err)
			return jsonValidationResponse(500, ErrorResponse{
				Error:   "configuration_error",
				Message: "this service is not configured for GovPay payments",
			})
		}

		// A genuine mismatch gets exactly what an unknown reference gets, so a
		// caller aimed at the wrong service learns nothing about whether this
		// reference exists or is payable.
		slog.WarnContext(ctx, "govpay presentment rejected: identity mismatch",
			"reference", tx.ReferenceNumber,
			"gotSubInstId", req.SubInstID, "gotServiceId", req.ServiceID,
			"error", err)
		return jsonValidationResponse(404, ErrorResponse{
			Error:   "invalid_reference",
			Message: "invalid reference number",
		})
	}

	if !isPayable {
		return jsonValidationResponse(409, ErrorResponse{
			Error:   "not_payable",
			Message: "payment already completed, expired, or otherwise not payable for this reference number",
		})
	}

	resp := PresentmentResponse{
		TransactionID:   req.TransactionID,
		SubInstID:       req.SubInstID,
		ServiceID:       req.ServiceID,
		ServiceName:     req.ServiceName,
		Message:         "Success",
		PresentmentData: buildPresentmentData(tx),
	}
	return jsonValidationResponse(200, resp)
}

// -----------------------------------------------------------------------------
// presentmentData builders
// -----------------------------------------------------------------------------

// buildPresentmentData returns the fields GovPay+ should display for a payable
// transaction. The reference number is presented (read-only) and echoed back in
// the update request (returned=true), as is the amount to be paid.
func buildPresentmentData(tx *corepayment.ValidationTransaction) []PresentmentObject {
	objects := []PresentmentObject{
		newPresentmentObject(1, "label", "Reference Number", tx.ReferenceNumber, "text", refNoMaxLength, false, true, "refNo", true, false),
		newPresentmentObject(2, "textBox", "Amount To Be Paid", tx.Amount.String(), "decimal", 13, false, true, "amount", false, true),
	}
	if strings.TrimSpace(tx.Currency) != "" {
		objects = append(objects, newPresentmentObject(len(objects)+1, "label", "Currency", tx.Currency, "text", 8, false, true, "currency", false, false))
	}
	return objects
}

// newPresentmentObject builds a single presentment object with the common
// GovPay+ defaults, varying only the fields a caller cares about.
func newPresentmentObject(seq int, objType, placeholder string, initialValue interface{}, dataType string, maxLength int, enabled, returned bool, returnParam string, isPaymentReference, isPaymentAmount bool) PresentmentObject {
	return PresentmentObject{
		ObjType:            objType,
		Seq:                strconv.Itoa(seq),
		ID:                 fmt.Sprintf("%03d%04d", seq, seq),
		Placeholder:        placeholder,
		InitialValue:       initialValue,
		DataType:           dataType,
		MaxLength:          maxLength,
		SelectionType:      "SINGLE",
		Mask:               "",
		NotNull:            "true",
		Enabled:            boolToFlag(enabled),
		Returned:           boolToFlag(returned),
		Rows:               1,
		Cols:               1,
		ReturnParam:        returnParam,
		IsPaymentReference: isPaymentReference,
		IsPaymentAmount:    isPaymentAmount,
		ReturnValue:        "",
		ObjData:            []ComboItem{},
	}
}
