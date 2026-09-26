package govpay

import "errors"

// -----------------------------------------------------------------------------
// GovPay+ wire types
//
// These mirror the GovPay+ GO API contract: GovPay+ posts the same request
// shape to both the presentment (validate) and update (webhook) endpoints, and
// expects a PresentmentResponse / UpdateResponse back. Field names, fallbacks
// and the presentmentData/paymentData object structures follow the GovPay+
// reference integration verbatim.
// -----------------------------------------------------------------------------

// Config holds the GovPay+ gateway configuration.
type Config struct {
	BaseURL string

	// WebhookClientID is the OAuth2 client GovPay+ authenticates its callbacks
	// as. Both callback routes already require a token carrying the payment
	// webhook scopes, but any machine client holding those scopes could
	// otherwise post against any gateway — so the gateway pins the caller to its
	// own client. Required: without it a callback cannot be judged either way.
	WebhookClientID string `json:"webhook_client_id"`

	// PrivateKey / PrivateKeyFile locate this GO's RSA private key, the half
	// of the pair whose public key GovPay+ holds. GovPay+ encrypts a fresh
	// transaction key to it on every call (spec §3), so without it no call can
	// be read. PrivateKey (inline PEM) takes precedence over PrivateKeyFile.
	PrivateKey     string `json:"private_key"`
	PrivateKeyFile string `json:"private_key_file"`
}

// MethodID is the payment method this gateway is registered under. The
// identity requirements below apply to GovPay+ only; other methods are
// unaffected.
const MethodID = "govpay"

// ErrIdentityMismatch reports a GovPay+ call whose sub-institution or service
// id is not the one the reference number was registered under.
var ErrIdentityMismatch = errors.New("govpay identity mismatch")

// ErrWebhookClientNotConfigured reports a deployment that has not named the
// OAuth2 client GovPay+ calls back as. It is an operational fault, not evidence
// about the caller, so it must never be reported as a verification failure.
var ErrWebhookClientNotConfigured = errors.New("govpay webhook client not configured")

// ErrIdentityNotConfigured reports a GovPay+ fee whose artifact did not declare
// both ids. They are mandatory for GovPay+, so a transaction missing them
// cannot be settled — a callback would have nothing trustworthy to be checked
// against.
var ErrIdentityNotConfigured = errors.New("govpay identity not configured")

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
//
// Every field is a string because every field is encrypted on the wire
// (spec §3.1.7) and AES-CBC output is base64 text. Numeric and boolean fields
// therefore carry their string form ("50", "true") before encryption, and the
// response uses the `returnedValue` field name (spec §3.4).
type PresentmentObject struct {
	ObjType            string           `json:"objType"`
	Seq                string           `json:"seq"`
	ID                 string           `json:"id"`
	Placeholder        string           `json:"placeholder"`
	InitialValue       string           `json:"initialValue"`
	DataType           string           `json:"datatype"`
	MaxLength          string           `json:"maxLength"`
	SelectionType      string           `json:"selectionType"`
	Mask               string           `json:"mask"`
	NotNull            string           `json:"notNull"`
	Enabled            string           `json:"enabled"`
	Returned           string           `json:"returned"`
	Rows               string           `json:"rows"`
	Cols               string           `json:"cols"`
	ReturnParam        string           `json:"returnedParam"`
	IsPaymentReference string           `json:"isPaymentReference,omitempty"`
	IsPaymentAmount    string           `json:"isPaymentAmount,omitempty"`
	ReturnValue        string           `json:"returnedValue"`
	ObjData            []ComboItem      `json:"objData,omitempty"`
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

// PaymentItem is one field in an UpdateResponse receipt. It is all-string for
// the same reason as PresentmentObject: every field is encrypted on the wire.
type PaymentItem struct {
	ObjType       string           `json:"objType"`
	Seq           string           `json:"seq"`
	ID            string           `json:"id"`
	Placeholder   string           `json:"placeholder"`
	InitialValue  string           `json:"initialValue"`
	DataType      string           `json:"datatype"`
	MaxLength     string           `json:"maxLength"`
	SelectionType string           `json:"selectionType"`
	Mask          string           `json:"mask"`
	NotNull       string           `json:"notNull"`
	Enabled       string           `json:"enabled"`
	Returned      string           `json:"returned"`
	Rows          string           `json:"rows"`
	Cols          string           `json:"cols"`
	ReturnParam   string           `json:"returnedParam"`
	ReturnValue   string           `json:"returnedValue"`
	TableData     *TableDataObject `json:"tableData,omitempty"`
}
