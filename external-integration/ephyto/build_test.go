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
		"transport_mode":           "3",
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

func TestBuildInput_MapsAndBuildsValidSOAP(t *testing.T) { //nolint:gocyclo // exhaustive field-by-field assertions on the mapped certificate
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

// The Hub refuses an envelope whose certificate has no
// MainCarriageSPSTransportMovement ("element is mandatory"), so the certificate
// carries the mode of transport whatever the form said — including nothing.
func TestBuildInput_AlwaysCarriesTheTransportMovement(t *testing.T) {
	for name, tc := range map[string]struct{ formValue, want string }{
		"the code the form submitted": {"3", "3"},
		"another code":                {"8", "8"},
		"nothing chosen":              {"", modeCodeNotSpecified},
		// The form used to present labels; a task filled in before that changed
		// still holds one.
		"a legacy sea label": {"sea", "3"},
		"a legacy air label": {"air", "1"},
	} {
		t.Run(name, func(t *testing.T) {
			uf := sampleUserform()
			uf["transport_mode"] = tc.formValue

			in := BuildInput(map[string]any{"userform": uf, "certificate_id": "PC-1", "hub_destination": "LK2"})

			conveyances := in.Certificate.Consignment.MeansOfConveyance
			if len(conveyances) != 1 {
				t.Fatalf("conveyances = %d, want exactly one so the mandatory element is emitted", len(conveyances))
			}
			if conveyances[0].ModeCode != tc.want {
				t.Errorf("ModeCode = %q, want %q", conveyances[0].ModeCode, tc.want)
			}
		})
	}
}

// The same, seen on the wire: the element the Hub asked for is in the XML.
func TestBuildCertXML_EmitsTheTransportMovement(t *testing.T) {
	in := BuildInput(map[string]any{"userform": sampleUserform(), "certificate_id": "PC-1", "hub_destination": "LK2"})

	body, err := BuildCertXML(in)
	if err != nil {
		t.Fatalf("BuildCertXML: %v", err)
	}
	if !strings.Contains(body, "<ram:MainCarriageSPSTransportMovement>") {
		t.Error("certificate has no MainCarriageSPSTransportMovement; the Hub rejects that as a missing mandatory element")
	}
	if !strings.Contains(body, "<ram:ModeCode>3</ram:ModeCode>") {
		t.Error("the chosen mode of transport did not reach the certificate")
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

// The trader uploads documents at three points in the NPQS flow, and says at
// the ePhyto step which of them travel with the certificate. A yes lists the
// document; anything else leaves it out.
func TestBuildInput_ListsTheDocumentsTheTraderSaidYesTo(t *testing.T) {
	form := sampleUserform()
	form["attachments"] = []any{
		map[string]any{
			"attachment_file_url":    "https://nsw.gov.lk/storage/uploads/invoice-1092.pdf?sig=abc",
			"attachment_description": "Commercial invoice of foliage export",
			"file_type":              "Fumigation Certificate",
		},
		map[string]any{"attachment_file_url": "packing/list.pdf"},
	}

	in := BuildInput(map[string]any{
		"userform":       form,
		"certificate_id": "PC-2026-0001",

		"send_application_documents": true,
		"treatment_certificate_url":  "storage/certs/treatment-cert-1092.pdf",
		"send_treatment_certificate": true,
		"invoice_file_url":           "storage/docs/invoice.pdf",
		"send_invoice":               true,

		// Uploaded, but the trader said no.
		"packing_list_file_url": "storage/docs/packing.pdf",
		"send_packing_list":     false,
		// Said yes, but nothing was ever uploaded.
		"send_supervision_report": true,
	})

	got := in.Certificate.Attachments
	if len(got) != 4 {
		t.Fatalf("expected the two application attachments, the treatment certificate and the invoice, got %d: %+v", len(got), got)
	}

	// The trader's own type and description travel with the file, and a signed
	// URL's query is not part of the name a reader sees.
	if got[0].ID != "Fumigation Certificate" || got[0].Filename != "invoice-1092.pdf" {
		t.Errorf("first attachment = %+v", got[0])
	}
	if got[0].Information != "Commercial invoice of foliage export" {
		t.Errorf("description not carried: %q", got[0].Information)
	}
	if got[0].RelationshipTypeCode != "ZZZ" {
		t.Errorf("relationship = %q, want ZZZ (accompanying document)", got[0].RelationshipTypeCode)
	}

	// An attachment with no declared type is listed rather than dropped.
	if got[1].ID != "Supporting Document" || got[1].Filename != "list.pdf" {
		t.Errorf("untyped attachment = %+v", got[1])
	}

	// The later steps upload one file per field, so the field names the document.
	if got[2].ID != "Treatment Certificate" || got[2].Filename != "treatment-cert-1092.pdf" {
		t.Errorf("treatment certificate = %+v", got[2])
	}
	if got[3].ID != "Commercial Invoice" {
		t.Errorf("supporting document = %+v", got[3])
	}

	for _, a := range got {
		if a.ID == "Packing List" {
			t.Error("a document the trader said no to was listed")
		}
		if a.ID == "Treatment Supervision Report" {
			t.Error("a document that was never uploaded was listed")
		}
	}
}

// Saying no to everything sends nothing, and so does not being asked at all —
// a document travels only on an explicit yes.
func TestBuildInput_NothingSaidYesToListsNothing(t *testing.T) {
	form := sampleUserform()
	form["attachments"] = []any{map[string]any{"attachment_file_url": "storage/uploads/permit.pdf", "file_type": "Import Permit"}}

	for name, inputs := range map[string]map[string]any{
		"said no": {
			"userform": form, "certificate_id": "PC-2026-0002",
			"send_application_documents": false,
		},
		"never asked": {"userform": form, "certificate_id": "PC-2026-0002"},
	} {
		if got := BuildInput(inputs).Certificate.Attachments; len(got) != 0 {
			t.Errorf("%s: expected no attachments, got %+v", name, got)
		}
	}
}

// A form that stringifies its checkboxes still says yes.
func TestBuildInput_AcceptsAStringifiedYes(t *testing.T) {
	in := BuildInput(map[string]any{
		"userform":                   sampleUserform(),
		"certificate_id":             "PC-2026-0003",
		"treatment_certificate_url":  "storage/certs/fumigation.pdf",
		"send_treatment_certificate": "true",
	})
	if len(in.Certificate.Attachments) != 1 {
		t.Fatalf("expected the treatment certificate, got %+v", in.Certificate.Attachments)
	}
}

// The documents must reach the rendered certificate, not just the input struct.
func TestBuildCertXML_CarriesTheReferencedDocuments(t *testing.T) {
	form := sampleUserform()
	form["attachments"] = []any{map[string]any{
		"attachment_file_url":    "storage/uploads/phyto-permit.pdf",
		"attachment_description": "Import permit issued by the NPPO of destination",
		"file_type":              "Import Permit",
	}}

	xml, err := BuildCertXML(BuildInput(map[string]any{
		"userform":                   form,
		"certificate_id":             "PC-2026-0004",
		"send_application_documents": true,
	}))
	if err != nil {
		t.Fatalf("BuildCertXML: %v", err)
	}

	for _, want := range []string{
		"<ram:ReferenceSPSReferencedDocument>",
		"<ram:RelationshipTypeCode>ZZZ</ram:RelationshipTypeCode>",
		"<ram:ID>Import Permit</ram:ID>",
		`filename="phyto-permit.pdf"`,
		"Import permit issued by the NPPO of destination",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("certificate is missing %q", want)
		}
	}
}
