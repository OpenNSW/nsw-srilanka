package govpay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
)

// refNoMaxLength is the maximum length configured for refNo at this GovPay+
// service. The protocol allows data[].value up to 50 characters; this service
// uses the stricter 20-character limit.
const refNoMaxLength = 20

// parseGovPayRequest decodes the common GovPay+ request envelope, tolerating the
// field-name variants seen across GovPay+ environments.
func parseGovPayRequest(raw json.RawMessage) (govPayRequest, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return govPayRequest{}, fmt.Errorf("invalid json: %w", err)
	}

	request, err := parseGovPayRequestHeaders(payload)
	if err != nil {
		return govPayRequest{}, err
	}

	if rawData, found := payload["data"]; found {
		// UseNumber preserves unexpected numeric values exactly; decryptParams
		// will reject them because GovPay+ sends each encrypted field as a string.
		dec := json.NewDecoder(bytes.NewReader(rawData))
		dec.UseNumber()
		if err := dec.Decode(&request.Data); err != nil {
			return govPayRequest{}, fmt.Errorf("invalid data array: %w", err)
		}
	}
	return request, nil
}

func parseGovPayRequestHeaders(payload map[string]json.RawMessage) (govPayRequest, error) {
	transactionID, ok, err := getStringField(payload, "transactionID", "transactionId")
	if err != nil {
		return govPayRequest{}, err
	}
	if !ok {
		return govPayRequest{}, fmt.Errorf("transactionID is missing in request")
	}
	if err := validateANField("transactionID", transactionID, 20); err != nil {
		return govPayRequest{}, err
	}

	// Both identity fields are mandatory on the presentment and update calls
	// alike: they say which service the caller believes it is transacting
	// against, and a call that omits them cannot be checked against the service
	// the reference was registered under. Treated exactly like transactionID
	// above rather than defaulted to empty.
	subInstID, ok, err := getStringField(payload, "subinstId", "suinstId")
	if err != nil {
		return govPayRequest{}, err
	}
	if !ok {
		return govPayRequest{}, fmt.Errorf("subinstId is missing in request")
	}
	if err := validateNumericField("subinstId", subInstID, 3); err != nil {
		return govPayRequest{}, err
	}
	serviceID, ok, err := getStringField(payload, "serviceid", "serviceId", "serviced")
	if err != nil {
		return govPayRequest{}, err
	}
	if !ok {
		return govPayRequest{}, fmt.Errorf("serviceid is missing in request")
	}
	if err := validateNumericField("serviceid", serviceID, 3); err != nil {
		return govPayRequest{}, err
	}
	serviceName, ok, err := getStringField(payload, "serviceName")
	if err != nil {
		return govPayRequest{}, err
	}
	if !ok {
		return govPayRequest{}, fmt.Errorf("serviceName is missing in request")
	}
	if err := validateANSField("serviceName", serviceName, 50); err != nil {
		return govPayRequest{}, err
	}

	return govPayRequest{
		TransactionID: transactionID,
		SubInstID:     subInstID,
		ServiceID:     serviceID,
		ServiceName:   serviceName,
	}, nil
}

// validateGovPayData checks the plaintext data[] fields after decryption. GovPay+
// encrypts seq, paramName, and value separately on the wire, so their contract
// types and lengths cannot be checked while decoding the JSON envelope.
func validateGovPayData(params []govPayParam) error {
	for i, param := range params {
		prefix := fmt.Sprintf("data[%d]", i)
		if err := validateNumericField(prefix+".seq", param.Seq, 2); err != nil {
			return err
		}
		if err := validateANSField(prefix+".paramName", param.ParamName, 50); err != nil {
			return err
		}
		value, ok := param.Value.(string)
		if !ok {
			return fmt.Errorf("%s.value must be a string", prefix)
		}
		if err := validateANSField(prefix+".value", value, 50); err != nil {
			return err
		}
	}
	return nil
}

// validateRefNoOnly enforces that the request carries exactly one data item,
// named "refNo", whose value is non-empty, printable, and no longer than
// refNoMaxLength. GovPay+ defines data[].value as ans, which permits spaces and
// special characters. It returns the exact refNo value unchanged on success.
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
	return validateRefNoValue(refNo)
}

func validateRefNoValue(refNo string) (string, error) {
	if refNo == "" {
		return "", fmt.Errorf("refNo is required")
	}
	if utf8.RuneCountInString(refNo) > refNoMaxLength {
		return "", fmt.Errorf("refNo must not exceed %d characters", refNoMaxLength)
	}
	if !isANS(refNo) {
		return "", fmt.Errorf("refNo must contain printable characters")
	}
	return refNo, nil
}

func isANS(s string) bool {
	for _, r := range s {
		if r < ' ' || r > '~' {
			return false
		}
	}
	return s != ""
}

func validateANField(name, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%s must not exceed %d characters", name, maxLength)
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return fmt.Errorf("%s must contain only letters and numbers", name)
		}
	}
	return nil
}

func validateNumericField(name, value string, maxLength int) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%s must not exceed %d digits", name, maxLength)
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return fmt.Errorf("%s must contain only digits", name)
		}
	}
	return nil
}

func validateANSField(name, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%s must not exceed %d characters", name, maxLength)
	}
	if !isANS(value) {
		return fmt.Errorf("%s must contain printable characters", name)
	}
	return nil
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
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

// paramDecimal converts a JSON-decoded data[] value into a decimal. The data[]
// array is decoded with UseNumber (see parseGovPayRequest), so numeric values
// arrive as json.Number and quoted values as string.
func paramDecimal(value interface{}) (decimal.Decimal, error) {
	switch x := value.(type) {
	case string:
		return decimal.NewFromString(strings.TrimSpace(x))
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
		if !ok || bytes.Equal(raw, []byte("null")) {
			continue
		}
		// Accept either a JSON string or a JSON number: some GovPay+
		// environments send numeric fields (e.g. transactionID) unquoted,
		// which would otherwise fail unmarshalling into a string.
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			var number json.Number
			if err := json.Unmarshal(raw, &number); err != nil {
				return "", false, fmt.Errorf("%s must be string or number", key)
			}
			return number.String(), true, nil
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

func boolToFlag(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
