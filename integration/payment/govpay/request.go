package govpay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corepayment "github.com/OpenNSW/core/payment"
	"github.com/shopspring/decimal"
)

// refNoMaxLength is the maximum allowed length of the presentment refNo. Per the
// GovPay+ spec a data[].value is alphanumeric with a hard ceiling of 50; 20 is
// the value configured for this service at onboarding.
const refNoMaxLength = 20

// parseGovPayRequest decodes the common GovPay+ request envelope, tolerating the
// field-name variants seen across GovPay+ environments.
func parseGovPayRequest(raw json.RawMessage) (govPayRequest, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return govPayRequest{}, fmt.Errorf("invalid json: %w", err)
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
		// Decode with UseNumber so numeric data[].value items land as
		// json.Number rather than float64; this preserves exact precision for
		// payment amounts (a float64 cannot represent every int64/decimal).
		dec := json.NewDecoder(bytes.NewReader(rawData))
		dec.UseNumber()
		if err := dec.Decode(&data); err != nil {
			return govPayRequest{}, fmt.Errorf("invalid data array: %w", err)
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
	refNo := paramValueString(param.Value)
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
