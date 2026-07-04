// Package hub builds and posts IPPC Hub SOAP messages. It is intentionally
// decoupled from the spscert package: callers convert their JSON input into
// the small DTOs defined here before invoking the builders. Ported from the
// OpenNSW external-integrations-sandbox `ippc/internal/hub` package.
package hub

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// IPPC Hub SOAP namespaces and the default operation.
const (
	nsSOAPEnv = "http://schemas.xmlsoap.org/soap/envelope/"
	nsEPH     = "http://ephyto.ippc.int/"
	nsHUB     = "http://ephyto.ippc.int/HUB.Entities"

	defaultOperation = "ValidateAndDeliverEnvelope"
)

// Envelope is the routing portion of the Hub SOAP request (doc §5.1.1). It's
// a thin DTO so the hub package doesn't have to import the spscert JSON model.
type Envelope struct {
	Operation             string
	From                  string
	To                    string
	CertificateType       int
	CertificateStatus     int
	NPPOCertificateNumber string
	Forwardings           []string
}

// BuildDeliverSOAP wraps the certificate XML inside the configured operation
// (default ValidateAndDeliverEnvelope, doc §6.13).
func BuildDeliverSOAP(env Envelope, certXML string) string {
	op := env.Operation
	if op == "" {
		op = defaultOperation
	}

	var fwd strings.Builder
	if len(env.Forwardings) > 0 {
		fwd.WriteString("        <hub:Forwardings>\n")
		for _, code := range env.Forwardings {
			fwd.WriteString("          <hub:EnvelopeForwarding>\n")
			fwd.WriteString("            <hub:Code>" + esc(code) + "</hub:Code>\n")
			fwd.WriteString("          </hub:EnvelopeForwarding>\n")
		}
		fwd.WriteString("        </hub:Forwardings>\n")
	}

	var b strings.Builder
	b.WriteString(`<soapenv:Envelope xmlns:soapenv="` + nsSOAPEnv + `" xmlns:eph="` + nsEPH + `" xmlns:hub="` + nsHUB + `">` + "\n")
	b.WriteString("  <soapenv:Header/>\n")
	b.WriteString("  <soapenv:Body>\n")
	b.WriteString("    <eph:" + op + ">\n")
	b.WriteString("      <eph:envelope>\n")
	b.WriteString("        <hub:From>" + esc(env.From) + "</hub:From>\n")
	b.WriteString("        <hub:To>" + esc(env.To) + "</hub:To>\n")
	fmt.Fprintf(&b, "        <hub:CertificateType>%d</hub:CertificateType>\n", env.CertificateType)
	fmt.Fprintf(&b, "        <hub:CertificateStatus>%d</hub:CertificateStatus>\n", env.CertificateStatus)
	b.WriteString("        <hub:NPPOCertificateNumber>" + esc(env.NPPOCertificateNumber) + "</hub:NPPOCertificateNumber>\n")
	b.WriteString(fwd.String())
	b.WriteString("        <hub:Content><![CDATA[" + certXML + "]]></hub:Content>\n")
	b.WriteString("      </eph:envelope>\n")
	b.WriteString("    </eph:" + op + ">\n")
	b.WriteString("  </soapenv:Body>\n")
	b.WriteString("</soapenv:Envelope>\n")
	return b.String()
}

// BuildGetTrackingSOAP wraps GetEnvelopeTrackingInfo (doc §6.6).
func BuildGetTrackingSOAP(trackingNumber string) string {
	return buildSimpleSOAP("GetEnvelopeTrackingInfo", "hubTrackingNumber", trackingNumber)
}

func buildSimpleSOAP(op, paramName, paramValue string) string {
	var b strings.Builder
	b.WriteString(`<soapenv:Envelope xmlns:soapenv="` + nsSOAPEnv + `" xmlns:eph="` + nsEPH + `">` + "\n")
	b.WriteString("  <soapenv:Header/>\n")
	b.WriteString("  <soapenv:Body>\n")
	b.WriteString("    <eph:" + op + ">\n")
	b.WriteString("      <eph:" + paramName + ">" + esc(paramValue) + "</eph:" + paramName + ">\n")
	b.WriteString("    </eph:" + op + ">\n")
	b.WriteString("  </soapenv:Body>\n")
	b.WriteString("</soapenv:Envelope>\n")
	return b.String()
}

func esc(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
