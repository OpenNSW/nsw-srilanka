package hub

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ===========================================================================
// HTTP CLIENT  (mTLS over TLS 1.2/1.3)
//
// The Hub authenticates clients with an X.509 certificate (doc §4.4). The
// public key must already be uploaded against the connection profile in the
// Hub Admin Console; otherwise the handshake succeeds but the Hub rejects
// the call with "NPPO from client certificate not found".
//
// This file owns the transport only — endpoint URLs come from the caller
// (resolved via configuration). No URL is hardcoded here.
// ===========================================================================

// Client posts a Hub SOAP request and parses the response.
type Client struct {
	Endpoint string
	HTTP     *http.Client
}

// NewClient builds a Client that authenticates with a PEM cert + key pair.
func NewClient(endpoint, certPEM, keyPEM string, timeout time.Duration) (*Client, error) {
	cert, err := tls.LoadX509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}
	return &Client{
		Endpoint: endpoint,
		HTTP: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
					MinVersion:   tls.VersionTLS12,
				},
			},
		},
	}, nil
}

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

// Send posts the supplied SOAP envelope and parses the response.
func (c *Client) Send(ctx context.Context, soapXML string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, strings.NewReader(soapXML))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("posting to Hub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Hub response: %w", err)
	}

	out := &Response{HTTPStatus: resp.StatusCode, RawBody: string(body)}
	if err := parseResponse(body, out); err != nil {
		return out, err
	}
	return out, nil
}

// Deliver is a convenience wrapper around BuildDeliverSOAP + Send.
func (c *Client) Deliver(ctx context.Context, env Envelope, certXML string) (*Response, error) {
	return c.Send(ctx, BuildDeliverSOAP(env, certXML))
}

// GetEnvelopeTrackingInfo queries the delivery status of a single envelope
// (doc §6.6).
func (c *Client) GetEnvelopeTrackingInfo(ctx context.Context, trackingNumber string) (*Response, error) {
	return c.Send(ctx, BuildGetTrackingSOAP(trackingNumber))
}

// ===========================================================================
// Response parsing — keep namespace-agnostic; the Hub uses dynamic ns2/ns3
// prefixes so we match by local name.
// ===========================================================================

func parseResponse(body []byte, out *Response) error {
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
