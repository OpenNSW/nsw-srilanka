package hub

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// ===========================================================================
// HUB RESPONSE PARSING
//
// Transport (mTLS, timeouts, endpoint) lives in core/remote — the Hub is a
// registered service in services.json and the generic SOAP-call plugin posts
// the envelopes. This package owns what is Hub-specific: building the SOAP
// envelopes (soap.go) and parsing the Hub's responses (this file).
//
// Keep parsing namespace-agnostic; the Hub uses dynamic ns2/ns3 prefixes so
// we match by local name.
// ===========================================================================

// Response is the parsed outcome of a Hub call.
type Response struct {
	HubDeliveryNumber string
	HUBTrackingInfo   string
	ErrorMessage      string
	Validations       []ValidationResult
	HTTPStatus        int
	RawBody           string
}

// Delivered reports whether the Hub accepted the envelope into its queue.
func (r *Response) Delivered() bool {
	return r.HubDeliveryNumber != "" && r.HUBTrackingInfo != "FailedDelivery"
}

// ValidationResult mirrors a single <ns3:ValidatePhytoXMLResult> row
// (doc §6.8). Level is SEVERE / WARNING / INFO.
type ValidationResult struct {
	Area  string `xml:"area" json:"area"`
	Field string `xml:"field" json:"field"`
	Level string `xml:"level" json:"level"`
	Msg   string `xml:"msg" json:"msg"`
}

// ParseResponse parses a Hub HTTP response body. The Response is returned even
// alongside an error, carrying whatever fields could be extracted.
//
// A non-2xx status (403 from an mTLS/authorisation problem, 502 from the
// gateway, ...) often carries a non-XML body, so the error surfaces the HTTP
// status rather than a cryptic "XML syntax error on line 1" — preferring a
// parsed SOAP fault message, then the parse error, then a bare status.
func ParseResponse(status int, body []byte) (*Response, error) {
	out := &Response{HTTPStatus: status, RawBody: string(body)}
	parseErr := parseBody(body, out)

	if status < 200 || status >= 300 {
		switch {
		case out.ErrorMessage != "":
			return out, fmt.Errorf("SOAP fault (HTTP %d): %s", status, out.ErrorMessage)
		case parseErr != nil:
			return out, fmt.Errorf("HTTP error %d: %w", status, parseErr)
		default:
			return out, fmt.Errorf("unexpected HTTP status %d", status)
		}
	}
	if parseErr != nil {
		return out, parseErr
	}
	return out, nil
}

func parseBody(body []byte, out *Response) error {
	var fault struct {
		XMLName     xml.Name `xml:"Envelope"`
		FaultString string   `xml:"Body>Fault>faultstring"`
	}
	if err := xml.Unmarshal(body, &fault); err == nil && fault.FaultString != "" {
		out.ErrorMessage = fault.FaultString
		return fmt.Errorf("SOAP fault: %s", fault.FaultString)
	}

	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("parsing Hub response: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "hubDeliveryNumber":
			if err := dec.DecodeElement(&out.HubDeliveryNumber, &start); err != nil {
				return err
			}
		case "HUBTrackingInfo":
			if err := dec.DecodeElement(&out.HUBTrackingInfo, &start); err != nil {
				return err
			}
		case "hubDeliveryErrorMessage":
			if err := dec.DecodeElement(&out.ErrorMessage, &start); err != nil {
				return err
			}
		case "ValidatePhytoXMLResult", "ValidationResult":
			var v ValidationResult
			if err := dec.DecodeElement(&v, &start); err != nil {
				return err
			}
			out.Validations = append(out.Validations, v)
		}
	}
	return nil
}
