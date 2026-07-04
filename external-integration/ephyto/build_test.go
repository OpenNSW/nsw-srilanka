package ephyto

import (
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/hub"
)

// sampleUserform mirrors the shape the NPQS apply step
// (configs/npqs/1-apply/userinput_jsonform.json) produces once decoded into a
// map[string]any (JSON numbers arrive as float64).
func sampleUserform() map[string]any {
	return map[string]any{
		"certificate_type":         "export",
		"applicant_name":           "ABC Exports (Pvt) Ltd",
		"nppo_office_location":     "Office 1",
		"importing_country":        "Andorra - AD",
		"transit_countries":        "Angola, France",
		"exporter_name":            "ABC Exports (Pvt) Ltd",
		"exporter_address":         "123, Galle Road, Colombo 03, Sri Lanka",
		"consignee_name":           "Global Import Corp",
		"consignee_address":        "456 Main St, Andorra la Vella, Andorra",
		"consignee_country":        "Andorra - AD",
		"distinguishing_marks":     "EXP-NPQS-2026-A",
		"point_of_entry_port":      "Andorra la Vella",
		"seal_number":              "SL-CUSTOMS-93820",
		"transport_mode":           "sea",
		"import_permit_number":     "IP-ANDORRA-2026-948",
		"proposed_inspection_date": "2026-07-01",
		"additional_declaration":   "CODE: SAD1 - inspected and found free from quarantine pests.",
		"disinfestation_treatment": "Fumigated with Methyl Bromide at 48g/m3 for 24h at 21C.",
		"commodities": []any{
			map[string]any{
				"commodity_common_name":      "Fresh foliage (Monstera leaves)",
				"commodity_botanical_name":   "Monstera deliciosa",
				"commodity_description":      "Cut foliage leaves for decorative purposes",
				"commodity_plant_part":       "leaves",
				"commodity_condition":        "fresh",
				"commodity_intended_use":     "decorative",
				"quantity_net_weight":        float64(500),
				"quantity_net_weight_unit":   "kg",
				"quantity_gross_weight":      float64(550),
				"quantity_gross_weight_unit": "kg",
				"packages_count":             float64(25),
				"packages_description":       "Cardboard boxes",
				"origin_country":             "Sri Lanka",
			},
		},
	}
}

func TestBuildInput_MapsAndBuildsValidSOAP(t *testing.T) {
	prev := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 7, 3, 10, 30, 0, 0, time.UTC) }
	defer func() { nowFunc = prev }()

	in := BuildInput(map[string]any{
		"userform":        sampleUserform(),
		"certificate_id":  "PC-2026-000123",
		"hub_destination": "LK2",
	})

	// Envelope routing uses the trader-selected Hub connection code (LK2 -> LK2);
	// the certificate content carries the ISO alpha-2 code it represents (LK).
	if in.SOAP.From != "LK2" || in.SOAP.To != "LK2" {
		t.Errorf("routing = %q -> %q, want LK2 -> LK2", in.SOAP.From, in.SOAP.To)
	}
	if in.SOAP.CertificateType != 851 {
		t.Errorf("CertificateType = %d, want 851 (export PC)", in.SOAP.CertificateType)
	}
	if in.SOAP.NPPOCertificateNumber != "PC-2026-000123" {
		t.Errorf("NPPOCertificateNumber = %q", in.SOAP.NPPOCertificateNumber)
	}

	c := in.Certificate
	if c.Number != "PC-2026-000123" || c.IssueDateTime != "2026-07-03T10:30:00Z" {
		t.Errorf("cert number/issue = %q / %q", c.Number, c.IssueDateTime)
	}
	// UAT pins the importer to LK2 (routing) / LK (ISO content), ignoring the
	// trader's selected importing country.
	if c.Consignment.ImportCountry != "LK" {
		t.Errorf("import country = %q, want LK", c.Consignment.ImportCountry)
	}
	if len(c.Consignment.Items) != 1 || len(c.Consignment.Items[0].TradeLines) != 1 {
		t.Fatalf("expected 1 item with 1 trade line, got %+v", c.Consignment.Items)
	}
	tl := c.Consignment.Items[0].TradeLines[0]
	if tl.ScientificName != "Monstera deliciosa" {
		t.Errorf("scientific name = %q", tl.ScientificName)
	}
	if tl.NetWeight == nil || tl.NetWeight.Value != "500" || tl.NetWeight.Unit != "KGM" {
		t.Errorf("net weight = %+v, want 500 KGM", tl.NetWeight)
	}
	if len(tl.Packages) != 1 || tl.Packages[0].Quantity != "25" {
		t.Errorf("packages = %+v, want quantity 25", tl.Packages)
	}
	if tl.OriginCountries[0] != "LK" {
		t.Errorf("origin = %v, want LK", tl.OriginCountries)
	}
	if len(tl.Treatments) != 1 || !strings.Contains(tl.Treatments[0].FullTreatment, "Methyl Bromide") {
		t.Errorf("treatment = %+v", tl.Treatments)
	}

	// The mapped input must build a valid SOAP envelope (validate + submit rely on it).
	certXML, err := BuildCertXML(in)
	if err != nil {
		t.Fatalf("BuildCertXML: %v", err)
	}
	soap := BuildDeliverSOAP(in, certXML)
	for _, want := range []string{"ValidateAndDeliverEnvelope", "<hub:To>LK2</hub:To>", "Monstera deliciosa"} {
		if !strings.Contains(soap, want) {
			t.Errorf("SOAP missing %q", want)
		}
	}
}

func TestBuildInput_ReExportSelectsType657(t *testing.T) {
	uf := sampleUserform()
	uf["certificate_type"] = "re-export"
	in := BuildInput(map[string]any{"userform": uf, "certificate_id": "PCR-1"})
	if in.SOAP.CertificateType != 657 || in.Certificate.TypeCode != "657" {
		t.Errorf("re-export type = %d / %q, want 657", in.SOAP.CertificateType, in.Certificate.TypeCode)
	}
	if len(in.Certificate.CertifyingStatementIDs) != 0 {
		t.Errorf("re-export should carry no document certifying statements, got %v", in.Certificate.CertifyingStatementIDs)
	}
}

func TestBuildCertXML_RejectsIncompleteDocument(t *testing.T) {
	// No certificate_id and no commodities => the validate gate must fail.
	in := BuildInput(map[string]any{"userform": map[string]any{"certificate_type": "export"}})
	if _, err := BuildCertXML(in); err == nil {
		t.Fatal("expected validation error for an incomplete document, got nil")
	}
}

func TestIsDelivered(t *testing.T) {
	for _, s := range []string{"Delivered", "delivered", "  DELIVERED  "} {
		if !IsDelivered(s) {
			t.Errorf("IsDelivered(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"PendingDelivery", "Sent", "", "Acknowledged"} {
		if IsDelivered(s) {
			t.Errorf("IsDelivered(%q) = true, want false", s)
		}
	}
}

func TestDescribeFailure_PrefersValidationResults(t *testing.T) {
	msg := DescribeFailure("Submission failed:", nil, &hub.Response{
		Validations: []hub.ValidationResult{
			{Level: "SEVERE", Area: "Consignment", Field: "ImportCountry", Msg: "Unknown country code"},
		},
	})
	if !strings.Contains(msg, "Unknown country code") || !strings.Contains(msg, "SEVERE") {
		t.Errorf("message = %q", msg)
	}

	transport := DescribeFailure("Submission failed:", context_err(), nil)
	if !strings.Contains(transport, "could not reach") {
		t.Errorf("transport message = %q", transport)
	}
}

func context_err() error { return &tempErr{} }

type tempErr struct{}

func (*tempErr) Error() string { return "dial tcp: timeout" }
