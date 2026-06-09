// Package payment holds nsw-srilanka's country-specific payment gateway
// integrations (e.g. GovPay+) that plug into the generic core/payment framework.
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
)

type Config struct {
	BaseURL string
}

// -----------------------------------------------------------------------------
// GovPay+ wire types
//
// These mirror the GovPay+ GO API contract: GovPay+ posts the same request
// shape to both the presentment (validate) and update (webhook) endpoints, and
// expects a PresentmentResponse / UpdateResponse back. Field names, fallbacks
// and the presentmentData/paymentData object structures follow the GovPay+
// reference integration verbatim.
// -----------------------------------------------------------------------------

// refNoMaxLength is the maximum allowed length of the presentment refNo. Per the
// GovPay+ spec a data[].value is alphanumeric with a hard ceiling of 50; 20 is
// the value configured for this service at onboarding.
const refNoMaxLength = 20

// govPayParam is a single data[] item in a GovPay+ presentment/update request.
type govPayParam struct {
	Seq       string      `json:"seq"`
	ParamName string      `json:"paramName"`
	Value     interface{} `json:"value"`
}

// govPayRequest is the common request shape GovPay+ posts to both the
// presentment (validate) and update (webhook) endpoints.
type govPayRequest struct {
	TransactionID string
	SubInstID     string
	ServiceID     string
	ServiceName   string
	Data          []govPayParam
}

// ErrorResponse is the GovPay+ error envelope.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// PresentmentResponse is returned from the presentment (validate) call: it tells
// GovPay+ which fields to render to the payer.
type PresentmentResponse struct {
	TransactionID   string              `json:"transactionID"`
	SubInstID       string              `json:"subinstId"`
	ServiceID       string              `json:"serviceid"`
	ServiceName     string              `json:"serviceName"`
	Message         string              `json:"message"`
	PresentmentData []PresentmentObject `json:"presentmentData"`
}

// PresentmentObject is one renderable field in a PresentmentResponse.
type PresentmentObject struct {
	ObjType            string           `json:"objType"`
	Seq                string           `json:"seq"`
	ID                 string           `json:"id"`
	Placeholder        string           `json:"placeholder"`
	InitialValue       interface{}      `json:"initialValue"`
	DataType           string           `json:"datatype"`
	MaxLength          int              `json:"maxLength"`
	SelectionType      string           `json:"selectionType"`
	Mask               string           `json:"mask"`
	NotNull            string           `json:"notNull"`
	Enabled            string           `json:"enabled"`
	Returned           string           `json:"returned"`
	Rows               int              `json:"rows"`
	Cols               int              `json:"cols"`
	ReturnParam        string           `json:"returnedParam"`
	IsPaymentReference bool             `json:"isPaymentReference,omitempty"`
	IsPaymentAmount    bool             `json:"isPaymentAmount,omitempty"`
	ReturnValue        string           `json:"returnValue"`
	ObjData            []ComboItem      `json:"objData"`
	TableData          *TableDataObject `json:"tableData,omitempty"`
}

type ComboItem struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type TableDataObject struct {
	Header  []TableHeader `json:"header"`
	RowData []TableRow    `json:"rowData"`
}

type TableHeader struct {
	DataType string `json:"dataType,omitempty"`
	Value    string `json:"value"`
	Enabled  string `json:"enabled,omitempty"`
}

type TableRow struct {
	DataType string `json:"dataType"`
	Value    string `json:"value"`
	Enabled  string `json:"enabled"`
}

// UpdateResponse is returned from the update (webhook) call: it acknowledges the
// recorded payment and carries a receipt in paymentData.
type UpdateResponse struct {
	TransactionID string        `json:"transactionID"`
	SubInstID     string        `json:"subinstId"`
	ServiceID     string        `json:"serviceid"`
	ServiceName   string        `json:"serviceName"`
	Message       string        `json:"message"`
	PaymentData   []PaymentItem `json:"paymentData"`
}

// PaymentItem is one field in an UpdateResponse receipt.
type PaymentItem struct {
	ObjType       string           `json:"objType"`
	Seq           string           `json:"seq"`
	ID            string           `json:"id"`
	Placeholder   string           `json:"placeholder"`
	InitialValue  interface{}      `json:"initialValue"`
	DataType      string           `json:"datatype"`
	MaxLength     int              `json:"maxLength"`
	SelectionType string           `json:"selectionType"`
	Mask          string           `json:"mask"`
	NotNull       string           `json:"notNull"`
	Enabled       string           `json:"enabled"`
	Returned      string           `json:"returned"`
	Rows          int              `json:"rows"`
	Cols          int              `json:"cols"`
	ReturnParam   string           `json:"returnedParam"`
	ReturnValue   string           `json:"returnValue"`
	TableData     *TableDataObject `json:"tableData,omitempty"`
}

// GovPayGateway implements corepayment.PaymentGateway for the GovPay+ aggregator.
type GovPayGateway struct {
	cfg Config
}

// NewGovPayGateway satisfies corepayment.Factory: it constructs a fully
// configured GovPayGateway from its raw config.
func NewGovPayGateway(cfg json.RawMessage) (corepayment.PaymentGateway, error) {
	var config Config
	if err := json.Unmarshal(cfg, &config); err != nil {
		return nil, err
	}

	return &GovPayGateway{
		cfg: config,
	}, nil
}

func (g *GovPayGateway) GetFlowType() corepayment.InteractionType {
	return corepayment.FlowTypeInstruction
}

func (g *GovPayGateway) CreateSession(ctx context.Context, req corepayment.SessionRequest) (*corepayment.SessionResponse, error) {
	return &corepayment.SessionResponse{
		Type:         corepayment.FlowTypeInstruction,
		Instructions: "Please pay using your bank application. Enter the provided reference number in the bill payment section of your app.",
	}, nil
}

// ExtractReferenceNumber pulls the NSW reference number out of a presentment
// request. Per the GovPay+ contract the reference travels as the single data[]
// item named "refNo".
func (g *GovPayGateway) ExtractReferenceNumber(ctx context.Context, referenceData json.RawMessage) (string, error) {
	req, err := parseGovPayRequest(referenceData)
	if err != nil {
		return "", err
	}
	return validateRefNoOnly(req.Data)
}

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
// Request parsing & helpers
// -----------------------------------------------------------------------------

// parseGovPayRequest decodes the common GovPay+ request envelope, tolerating the
// field-name variants seen across GovPay+ environments.
func parseGovPayRequest(raw json.RawMessage) (govPayRequest, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return govPayRequest{}, fmt.Errorf("invalid json")
	}

	transactionID, ok, err := getStringField(payload, "transactionID", "transactionId")
	if err != nil {
		return govPayRequest{}, err
	}
	if !ok {
		return govPayRequest{}, fmt.Errorf("transactionID is missing in request")
	}

	subInstID, _, err := getStringField(payload, "subinstId", "suinstId")
	if err != nil {
		return govPayRequest{}, err
	}
	serviceID, _, err := getStringField(payload, "serviceid", "serviceId", "serviced")
	if err != nil {
		return govPayRequest{}, err
	}
	serviceName, _, err := getStringField(payload, "serviceName")
	if err != nil {
		return govPayRequest{}, err
	}

	var data []govPayParam
	if rawData, found := payload["data"]; found {
		if err := json.Unmarshal(rawData, &data); err != nil {
			return govPayRequest{}, fmt.Errorf("invalid data array")
		}
	}

	return govPayRequest{
		TransactionID: transactionID,
		SubInstID:     subInstID,
		ServiceID:     serviceID,
		ServiceName:   serviceName,
		Data:          data,
	}, nil
}

// validateRefNoOnly enforces that the request carries exactly one data item,
// named "refNo", whose value is a non-empty alphanumeric string no longer than
// refNoMaxLength. It returns the trimmed refNo on success.
func validateRefNoOnly(params []govPayParam) (string, error) {
	if len(params) != 1 {
		return "", fmt.Errorf("data must contain exactly one item: refNo")
	}
	param := params[0]
	if strings.TrimSpace(param.ParamName) != "refNo" {
		return "", fmt.Errorf("data must contain only refNo")
	}
	refNo, ok := param.Value.(string)
	if !ok {
		return "", fmt.Errorf("refNo must be a string")
	}
	refNo = strings.TrimSpace(refNo)
	if refNo == "" {
		return "", fmt.Errorf("refNo is required")
	}
	if len(refNo) > refNoMaxLength {
		return "", fmt.Errorf("refNo must not exceed %d characters", refNoMaxLength)
	}
	if !isAlphaNumeric(refNo) {
		return "", fmt.Errorf("refNo must be alphanumeric")
	}
	return refNo, nil
}

func isAlphaNumeric(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return s != ""
}

// indexParams keys the data[] items by their lower-cased, trimmed paramName for
// case-insensitive lookups.
func indexParams(params []govPayParam) map[string]govPayParam {
	out := make(map[string]govPayParam, len(params))
	for _, p := range params {
		out[strings.ToLower(strings.TrimSpace(p.ParamName))] = p
	}
	return out
}

// stringParam returns the string value of the first present param among keys.
func stringParam(params map[string]govPayParam, keys ...string) string {
	for _, key := range keys {
		p, ok := params[key]
		if !ok {
			continue
		}
		return paramValueString(p.Value)
	}
	return ""
}

func paramValueString(value interface{}) string {
	switch x := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

// paramDecimal converts a JSON-decoded data[] value (string or number) into a
// decimal.
func paramDecimal(value interface{}) (decimal.Decimal, error) {
	switch x := value.(type) {
	case string:
		return decimal.NewFromString(strings.TrimSpace(x))
	case float64:
		return decimal.NewFromFloat(x), nil
	case json.Number:
		return decimal.NewFromString(x.String())
	default:
		return decimal.Decimal{}, fmt.Errorf("unsupported numeric type %T", value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func getStringField(payload map[string]json.RawMessage, keys ...string) (string, bool, error) {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false, fmt.Errorf("%s must be string", key)
		}
		return value, true, nil
	}
	return "", false, nil
}

// jsonValidationResponse marshals v into a corepayment.ValidationResponse with
// the given HTTP status.
func jsonValidationResponse(status int, v interface{}) (*corepayment.ValidationResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &corepayment.ValidationResponse{Payload: body, HTTPStatus: status}, nil
}

// -----------------------------------------------------------------------------
// presentmentData / paymentData builders
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
	case float32, float64, int, int32, int64, uint, uint32, uint64:
		return "decimal"
	default:
		return "text"
	}
}

func boolToFlag(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
