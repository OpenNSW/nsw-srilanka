package ephyto

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/hub"
	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/spscert"
)

// nowFunc is overridable in tests; production uses the wall clock for the
// certificate issue timestamp (the apply form does not capture it).
var nowFunc = time.Now

// BuildInput maps the NPQS phytosanitary application (the `userform` produced by
// the apply step, plus the workflow's certificate identifier) onto the friendly
// spscert.Input the ePhyto/SOAP layer consumes. It is the single mapping used by
// both the validate and submit plugins so the validated document and the
// submitted document are identical.
func BuildInput(inputs map[string]any) spscert.Input {
	uf := asMap(inputs["userform"])
	certID := asString(inputs["certificate_id"])

	certType := "851" // Phytosanitary Certificate
	if asString(uf["certificate_type"]) == "re-export" {
		certType = "657" // Phytosanitary Certificate for Re-Export
	}
	// The Hub destination is a connection code (e.g. "LK2"), chosen by the trader
	// on the submission form — not hardcoded here. In UAT the form offers only
	// the LK2 test connection, so envelopes route LK2 -> LK2, but the routing is
	// driven entirely by the selected value.
	hubDest := asString(inputs["hub_destination"])
	// The certificate content import country must be a valid ISO alpha-2 code
	// (the Hub validates ImportSPSCountry.ID as ISO alpha-2), so the connection
	// code "LK2" must not leak here. It is the ISO code the destination
	// connection represents — LK for Sri Lanka's LK2 test instance.
	importISO := isoFromHubConnection(hubDest)

	return spscert.Input{
		SOAP: spscert.SOAPInput{
			Operation: "ValidateAndDeliverEnvelope",
			// Envelope routing uses the Hub connection code selected by the trader,
			// not an ISO country code. NPQS only exchanges with its own IPPC Hub
			// test connection, so the same code is both sender and recipient.
			From:                  hubDest,
			To:                    hubDest,
			CertificateType:       certTypeCode(certType),
			CertificateStatus:     certStatusIssued,
			NPPOCertificateNumber: certID,
		},
		Certificate: spscert.CertInput{
			TypeCode:               certType,
			Number:                 certID,
			IssuingNPPO:            issuingNPPO,
			StatusCode:             strconv.Itoa(certStatusIssued),
			IssueDateTime:          nowFunc().Format(time.RFC3339),
			AuthorizedOfficer:      asString(uf["applicant_name"]),
			PlaceOfIssue:           officeName(asString(uf["nppo_office_location"])),
			CertifyingStatementIDs: certifyingStatementIDs(certType),
			DocDeclarations:        buildDocDeclarations(uf),
			Consignment:            buildConsignment(uf, importISO),
		},
	}
}

// BuildCertXML renders the SPSCertificate XML body for the given input. It also
// serves as the local validation gate: BuildCertificate rejects an incomplete
// document (missing certificate number, no items, a trade line without a
// description or origin), so a nil error means the document is structurally
// valid and ready to submit.
func BuildCertXML(in spscert.Input) (string, error) {
	cert, err := spscert.BuildCertificate(in)
	if err != nil {
		return "", err
	}
	body, err := xml.MarshalIndent(cert, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling certificate: %w", err)
	}
	return xml.Header + string(body), nil
}

// BuildDeliverSOAP renders the full ValidateAndDeliverEnvelope SOAP request.
func BuildDeliverSOAP(in spscert.Input, certXML string) string {
	return hub.BuildDeliverSOAP(toHubEnvelope(in.SOAP), certXML)
}

func toHubEnvelope(s spscert.SOAPInput) hub.Envelope {
	return hub.Envelope{
		Operation:             s.Operation,
		From:                  s.From,
		To:                    s.To,
		CertificateType:       s.CertificateType,
		CertificateStatus:     s.CertificateStatus,
		NPPOCertificateNumber: s.NPPOCertificateNumber,
		Forwardings:           s.Forwardings,
	}
}

// IsDelivered reports whether a Hub tracking-info status means the envelope has
// reached the importing NPPO — the terminal success state the trader polls for.
// The Hub reports "PendingDelivery" until the importing NPPO downloads the
// envelope, then "Delivered"; the "delivered" match excludes "PendingDelivery".
func IsDelivered(trackingInfo string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(trackingInfo)), "delivered")
}

// DescribeFailure builds a trader-facing markdown message from a failed Hub
// call, preferring the Hub's validation results, then its error string, then a
// generic transport message.
func DescribeFailure(intro string, callErr error, resp *hub.Response) string {
	if resp != nil {
		if bullets := validationBullets(resp.Validations); len(bullets) > 0 {
			return intro + "\n\n" + strings.Join(bullets, "\n")
		}
		if resp.ErrorMessage != "" {
			return intro + "\n\n- " + resp.ErrorMessage
		}
	}
	if callErr != nil {
		return intro + "\n\n- We could not reach the IPPC ePhyto Hub. Please try again."
	}
	return intro
}

func validationBullets(items []hub.ValidationResult) []string {
	bullets := make([]string, 0, len(items))
	for _, v := range items {
		if v.Msg == "" {
			continue
		}
		msg := v.Msg
		if v.Level != "" {
			msg = fmt.Sprintf("**%s** — %s", v.Level, msg)
		}
		if loc := strings.Trim(strings.Join(nonEmpty(v.Area, v.Field), "."), "."); loc != "" {
			msg = fmt.Sprintf("%s _(%s)_", msg, loc)
		}
		bullets = append(bullets, "- "+msg)
	}
	return bullets
}

// ---------------------------------------------------------------------------
// Fixed values / code lists
// ---------------------------------------------------------------------------

const (
	// exportNPPOCode is Sri Lanka's ISO 3166-1 alpha-2 code, used inside the
	// SPSCertificate content (ExportSPSCountry / OriginSPSCountry, etc.). The Hub
	// validates these against ISO alpha-2, so a Hub connection code like "LK2"
	// must never leak here. Envelope routing (hub:From / hub:To) uses the
	// connection code the trader selects on the form, not this value.
	exportNPPOCode   = "LK"
	certStatusIssued = 70 // Hub CertificateStatus for an issued certificate
	issuingNPPO      = "National Plant Quarantine Service of Sri Lanka"
)

// isoFromHubConnection derives the ISO 3166-1 alpha-2 country code from a Hub
// connection identifier by taking its leading letters (e.g. "LK2" -> "LK").
// Hub connection codes are the ISO country code plus an instance suffix, so this
// yields the code the SPSCertificate content requires. Anything that does not
// resolve to a two-letter code falls back to the export NPPO country (LK).
func isoFromHubConnection(conn string) string {
	var letters []rune
	for _, r := range strings.TrimSpace(conn) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters = append(letters, r)
			continue
		}
		break
	}
	if len(letters) == 2 {
		return strings.ToUpper(string(letters))
	}
	return exportNPPOCode
}

func certTypeCode(typeCode string) int {
	if typeCode == "657" {
		return 657
	}
	return 851
}

func certifyingStatementIDs(typeCode string) []string {
	if typeCode == "657" {
		return nil
	}
	return []string{"1", "2"}
}

func officeName(office string) string {
	switch office {
	case "Office 1":
		return "National Plant Quarantine Service - Katunayake"
	case "Office 2":
		return "National Plant Quarantine Service - Head Office (Colombo)"
	case "Office 3":
		return "National Plant Quarantine Service - Port of Colombo"
	case "Office 4":
		return "National Plant Quarantine Service - Hambantota Port"
	case "Office 5":
		return "National Plant Quarantine Service - Mattala Airport"
	case "Office 6":
		return "National Plant Quarantine Service - Jaffna Airport"
	default:
		return office
	}
}

func transportModeCode(mode string) string {
	switch mode {
	case "sea":
		return "1"
	case "air":
		return "4"
	default:
		return ""
	}
}

func weightUnit(unit string) string {
	switch unit {
	case "g":
		return "GRM"
	case "t":
		return "TNE"
	default:
		return "KGM"
	}
}

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

func buildDocDeclarations(uf map[string]any) *spscert.DocDeclarations {
	dd := &spscert.DocDeclarations{LanguageID: "en"}
	if v := asString(uf["additional_declaration"]); v != "" {
		dd.AdditionalDeclarations = []string{v}
	}
	if v := asString(uf["import_permit_number"]); v != "" {
		dd.ImportPermit = v
	}
	if v := asString(uf["proposed_inspection_date"]); v != "" {
		dd.DateOfInspection = v
	}
	if v := asString(uf["distinguishing_marks"]); v != "" {
		dd.DistinguishingMarks = []string{v}
	}
	if len(dd.AdditionalDeclarations) == 0 && dd.ImportPermit == "" &&
		dd.DateOfInspection == "" && len(dd.DistinguishingMarks) == 0 {
		return nil
	}
	return dd
}

func buildConsignment(uf map[string]any, importISO string) spscert.ConsignmentInput {
	c := spscert.ConsignmentInput{
		ExportCountry: exportNPPOCode,
		ImportCountry: importISO,
		Consignor: spscert.PartyInput{
			Name:         asString(uf["exporter_name"]),
			AddressLines: addressLines(asString(uf["exporter_address"])),
		},
		Consignee: spscert.PartyInput{
			Name:         asString(uf["consignee_name"]),
			AddressLines: addressLines(asString(uf["consignee_address"])),
		},
		Seal: asString(uf["seal_number"]),
	}
	for _, name := range splitList(asString(uf["transit_countries"])) {
		// Only include a transit country we can resolve to a valid ISO alpha-2
		// code; an unrecognised free-text name is omitted rather than sent as an
		// invalid code the Hub would reject (transit countries are optional).
		if code := isoCode(name); code != "" {
			c.TransitCountries = append(c.TransitCountries, code)
		}
	}
	if poe := asString(uf["point_of_entry_port"]); poe != "" {
		c.PointOfEntry = &spscert.PointOfEntry{Name: poe}
	}
	if mode := transportModeCode(asString(uf["transport_mode"])); mode != "" {
		c.MeansOfConveyance = []spscert.Conveyance{{ModeCode: mode}}
	}

	treatment := asString(uf["disinfestation_treatment"])
	for i, raw := range asSlice(uf["commodities"]) {
		c.Items = append(c.Items, spscert.ItemInput{
			TradeLines: []spscert.TradeLineInput{buildTradeLine(asMap(raw), i+1, treatment)},
		})
	}
	return c
}

func buildTradeLine(com map[string]any, seq int, treatment string) spscert.TradeLineInput {
	description := asString(com["commodity_description"])
	if description == "" {
		description = asString(com["commodity_common_name"])
	}

	tl := spscert.TradeLineInput{
		Sequence:        seq,
		Description:     description,
		DescriptionLang: "en",
		LanguageID:      "en",
		ScientificName:  asString(com["commodity_botanical_name"]),
		OriginCountries: []string{originISO(asString(com["origin_country"]))},
		VegetablePart:   asString(com["commodity_plant_part"]),
		Condition:       asString(com["commodity_condition"]),
		IntendedUse:     asString(com["commodity_intended_use"]),
	}
	if cn := asString(com["commodity_common_name"]); cn != "" {
		tl.CommonNames = []string{cn}
	}

	// Packaging: a structured package OR the free-text fallback (OPTND), never
	// both — the mapping requires exactly one.
	freeText := asString(com["packages_free_text"])
	if asBool(com["packages_free_text_enabled"]) && freeText != "" {
		tl.PackageDescription = freeText
	} else if count := numStr(com["packages_count"]); count != "" {
		tl.Packages = []spscert.PackageInput{{
			LevelCode: "1",
			TypeCode:  "PK", // generic package; the form has no IPPC package code
			Quantity:  count,
		}}
	} else if pd := asString(com["packages_description"]); pd != "" {
		tl.PackageDescription = pd
	}

	if v := numStr(com["quantity_net_weight"]); v != "" {
		tl.NetWeight = &spscert.MeasureInput{Value: v, Unit: weightUnit(asString(com["quantity_net_weight_unit"]))}
	}
	if v := numStr(com["quantity_gross_weight"]); v != "" {
		tl.GrossWeight = &spscert.MeasureInput{Value: v, Unit: weightUnit(asString(com["quantity_gross_weight_unit"]))}
	}

	if treatment != "" {
		tl.Treatments = []spscert.Treatment{{FullTreatment: treatment, LanguageID: "en"}}
	}
	return tl
}

// ---------------------------------------------------------------------------
// Small conversion / parsing helpers (JSON decodes into map[string]any).
// ---------------------------------------------------------------------------

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return ""
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// numStr renders a JSON number (float64) as a plain string without a trailing
// ".0", so 500 stays "500" and 550.5 stays "550.5". Strings pass through trimmed.
func numStr(v any) string {
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case string:
		return strings.TrimSpace(t)
	default:
		return ""
	}
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// addressLines splits a free-text address into up to five lines (the schema
// caps ram:SpecifiedSPSAddress at LineOne..LineFive), preferring newline then
// comma separators.
func addressLines(addr string) []string {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	sep := "\n"
	if !strings.Contains(addr, "\n") {
		sep = ","
	}
	var lines []string
	for _, part := range strings.Split(addr, sep) {
		if p := strings.TrimSpace(part); p != "" {
			lines = append(lines, p)
		}
	}
	if len(lines) > 5 {
		lines[4] = strings.Join(lines[4:], ", ")
		lines = lines[:5]
	}
	return lines
}

// splitList splits a comma/semicolon/newline separated free-text list.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	var out []string
	for _, f := range fields {
		if t := strings.TrimSpace(f); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// isoCode resolves a free-text country field to an ISO 3166-1 alpha-2 code —
// the form the IPPC Hub validates (ExportSPSCountry.ID, TransitSPSCountry.ID,
// OriginSPSCountry.ID, etc. must all be valid alpha-2). The apply form renders
// countries as "Name - XX" (e.g. "Andorra - AD"), so a trailing two-letter
// token wins; a bare two-letter value is accepted; and full country names are
// looked up in countryNameToISO. An unresolvable value returns "" (empty) —
// never a fabricated code like "ANGOLA" — so callers can skip an optional
// country or fall back to a sensible default rather than send the Hub garbage.
func isoCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// "Name - XX" (the apply form's format): a trailing 2-letter token wins.
	if i := strings.LastIndex(s, "-"); i != -1 {
		if tail := strings.TrimSpace(s[i+1:]); len(tail) == 2 && isAlpha(tail) {
			return strings.ToUpper(tail)
		}
	}
	// Already a bare alpha-2 code.
	if len(s) == 2 && isAlpha(s) {
		return strings.ToUpper(s)
	}
	// Full country name (or common alias).
	if code, ok := countryNameToISO[strings.ToLower(s)]; ok {
		return code
	}
	return ""
}

// originISO resolves a commodity's country of origin, defaulting to the export
// country (Sri Lanka) when the field is blank or unrecognised — NPQS export
// certificates are for goods of Sri Lankan origin.
func originISO(s string) string {
	if code := isoCode(s); code != "" {
		return code
	}
	return exportNPPOCode
}

var countryNameToISO = map[string]string{
	"sri lanka": "LK",
	"india":     "IN",
	"maldives":  "MV",
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
