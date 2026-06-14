package govpay

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
}

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
