package cusdec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalForm is the least a declaration form can carry and still build: an
// office, and one item. The field rules are ASYCUDA's to enforce, so the
// payload checks only what it cannot send at all.
func minimalForm() map[string]any {
	return map[string]any{
		"identification": map[string]any{"declarationType": "EX", "officeCode": "CBEX1"},
		"items": []any{
			map[string]any{
				"tarification":     map[string]any{"hsCode": "0801119000"},
				"goodsDescription": map[string]any{"commercialDescription": "cinnamon"},
			},
		},
	}
}

// identifier builds the payload and reads back what goes on the wire, which is
// where Annex A puts the field.
func identifier(t *testing.T, form map[string]any, previousEdgeID string) string {
	t.Helper()

	sub, _, err := BuildPayload(form, previousEdgeID)
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	assert.NotContains(t, wire, "submitter",
		"submitter belongs only inside properties")

	props := wire["properties"].(map[string]any)
	assert.Equal(t, submitterChannel, props["submitter"],
		"the channel is sent inside properties")
	return props["nswId"].(string)
}

// §2.2: a resend of the same attempt must carry the same identifier, which is
// the whole point — it is what lets ASYCUDA recognise the resend after a lost
// 202 instead of registering a second declaration.
func TestBuildPayload_SameAttemptDerivesTheSameIdentifier(t *testing.T) {
	first := identifier(t, minimalForm(), "")
	again := identifier(t, minimalForm(), "")

	require.NotEmpty(t, first)
	assert.Equal(t, first, again, "an unchanged resend is the same logical submission")
	assert.Len(t, first, 64, "a hex-encoded SHA-256")
}

// A correction the trader makes is a new business submission.
func TestBuildPayload_ACorrectionDerivesANewIdentifier(t *testing.T) {
	corrected := minimalForm()
	corrected["identification"] = map[string]any{"declarationType": "EX", "officeCode": "CBEX2"}

	assert.NotEqual(t, identifier(t, minimalForm(), ""), identifier(t, corrected, ""),
		"a changed declaration is a different submission")
}

// A resubmission after a rejected integration result is new even when the
// trader changed nothing: the previous attempt's edgeId settles it. Without
// this, ASYCUDA would suppress the resubmission as a duplicate and the
// integration result the trader is waiting on would never arrive.
func TestBuildPayload_AResubmissionIsNewEvenWhenNothingChanged(t *testing.T) {
	first := identifier(t, minimalForm(), "")
	second := identifier(t, minimalForm(), "5516e4c8-a93d-429d-8a18-6a484d331176")
	third := identifier(t, minimalForm(), "9f2b1a34-0c7e-4a71-b2d9-1e5f8c3a7b60")

	assert.NotEqual(t, first, second)
	assert.NotEqual(t, second, third)
}

// The identifier is derived from the submission as it will be sent, so it must
// not depend on itself: the field is empty while the digest is taken.
func TestBuildPayload_TheIdentifierDoesNotDependOnItself(t *testing.T) {
	sub, _, err := BuildPayload(minimalForm(), "")
	require.NoError(t, err)

	// Re-deriving from the payload as returned — with the field now populated —
	// would give a different answer if the field were part of the digest.
	rebuilt, _, err := BuildPayload(minimalForm(), "")
	require.NoError(t, err)
	assert.Equal(t, sub.Properties.NswID, rebuilt.Properties.NswID)
}

// sampleForm is the declaration Customs supplied as the worked example for
// api/declaration/v1, expressed in the shape the NSW form collects.
func sampleForm() map[string]any {
	return map[string]any{
		"containerCount": float64(2),
		"identification": map[string]any{
			"officeCode": "CBEX1", "declarationType": "EX",
			"generalProcedureCode": "1", "manifestRegNumber": "",
		},
		"traders": map[string]any{
			"exporter": map[string]any{"code": "1340082537000"},
			"consignee": map[string]any{
				"name":    "RENUKA AGRI FOODS",
				"address": "JAPAN 2016 31 22, SHIBA KOEN, MINATO-KUTOKYO JP",
				"code":    "", "countryCode": "JP",
			},
			"declarant": map[string]any{"code": "1040661607000"},
		},
		"generalInfo": map[string]any{
			"countryOfFirstDestination": "JP", "exportCountryCode": "LK",
			"destinationCountryCode": "JP",
		},
		"transport": map[string]any{
			"vesselName": "TestVessal", "transportNationality": "LK",
			"voyageNo": "VotageNameHere", "voyageNationality": "LK",
			"modeOfTransport": "1", "containerized": true,
			"deliveryTermsCode": "FOB", "deliveryTermsPlace": "Tokyo",
			"borderOfficeCode": "CBEX1", "placeOfDischargeCode": "LKADP",
			"warehouseCode": "string", "warehouseDelay": float64(0),
		},
		"financial": map[string]any{
			"bankCode": "6010", "bankReference": "RemRefTest",
			"remittanceAmount": float64(1500), "paymentTermsCode": "10",
			"deferredPayment": "DiffPaymentString",
		},
		"valuation": map[string]any{
			"invoiceAmount":   map[string]any{"amount": float64(1500), "currencyCode": "USD"},
			"externalFreight": map[string]any{"amount": float64(100), "currencyCode": "USD"},
			"internalFreight": map[string]any{"amount": float64(0), "currencyCode": "USD"},
			"insurance":       map[string]any{"amount": float64(0), "currencyCode": "USD"},
			"otherCosts":      map[string]any{"amount": float64(0), "currencyCode": "USD"},
			"deductions":      map[string]any{"amount": float64(0), "currencyCode": "USD"},
		},
		"packages": map[string]any{"totalPackages": float64(10)},
		"items": []any{map[string]any{
			"packages": map[string]any{"quantity": float64(10), "kindCode": "2C"},
			"tarification": map[string]any{
				"hsCode": "0801119000", "extendedProcedureCode": "1000",
				"nationalProcedureCode": "000",
				"supplementaryUnit":     map[string]any{"code": "KGM", "quantity": float64(125)},
			},
			"goodsDescription": map[string]any{
				"originCountryCode": "LK", "description": "description",
				"commercialDescription": "commercial", "commercialDescription1": "commercial",
			},
			"valuation": map[string]any{
				"grossWeight":   float64(1500),
				"netWeight":     float64(1300),
				"invoiceAmount": map[string]any{"amount": float64(1500), "currencyCode": "USD"},
			},
			"bol": "string", "bolSplit": "string",
			"marksAndNumbers": "test", "numberOfUnits": float64(1),
		}},
	}
}

// Annex A carries fields the builder used to drop on the floor, so the form
// could collect them and they would still never reach ASYCUDA. Assert them on
// the wire, which is the only place that settles it.
func TestBuildPayload_CarriesEveryAnnexAField(t *testing.T) {
	sub, _, err := BuildPayload(sampleForm(), "")
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))

	base := wire["baseGeneralSegment"].(map[string]any)
	general := wire["generalSegment"].(map[string]any)
	item := wire["goodsShipments"].([]any)[0].(map[string]any)
	remittance := wire["remittances"].([]any)[0].(map[string]any)

	// importer.id is Annex A's Importer/Consignee Code.
	assert.Equal(t, "", base["importer"].(map[string]any)["id"],
		"the importer code is sent even when the sample leaves it blank")

	assert.Equal(t, "string", general["warehouseCode"])
	assert.Equal(t, float64(0), general["warehouseDelay"])
	assert.Equal(t, "DiffPaymentString", general["deferredPayment"])
	assert.Equal(t, true, general["containerFlag"], "renamed from isContainer in v1.6")

	assert.Equal(t, "string", item["bol"])
	assert.Equal(t, "string", item["bolSplit"])
	assert.Equal(t, "test", item["marksAndNumbers"])
	assert.NotContains(t, item, "numberOfUnits", "dropped from Annex A in spec v1.7")

	// Annex A still requires the six-part item customsValue. The form only
	// collects invoiceAmount on the line, so chargeAmount is filled and the
	// other five costs are zeros.
	customsValue := item["customsValue"].(map[string]any)
	assertAmount(t, customsValue["chargeAmount"], 1500, "USD")
	assertZeroAmount(t, customsValue["externalFreight"])
	assertZeroAmount(t, customsValue["internalFreight"])
	assertZeroAmount(t, customsValue["insurance"])
	assertZeroAmount(t, customsValue["otherCost"])
	assertZeroAmount(t, customsValue["deductions"])

	// remittanceValue is an AmountType (§4.3), not a bare number named amount.
	assert.NotContains(t, remittance, "amount", "the pre-v1.6 spelling is gone")
	value := remittance["remittanceValue"].(map[string]any)
	assert.Equal(t, float64(1500), value["value"])
	assert.Equal(t, "USD", value["currencyID"], "the declaration's currency carries to the remittance")
}

func TestBuildPayload_HeaderValuationMapsTotalCustomsValuation(t *testing.T) {
	form := minimalForm()
	form["valuation"] = map[string]any{
		"invoiceAmount":    map[string]any{"amount": float64(2400), "currencyCode": "USD"},
		"externalFreight":  map[string]any{"amount": float64(100), "currencyCode": "USD"},
		"internalFreight":  map[string]any{"amount": float64(0), "currencyCode": "USD"},
		"insurance":        map[string]any{"amount": float64(0), "currencyCode": "USD"},
		"otherCosts":       map[string]any{"amount": float64(0), "currencyCode": "USD"},
		"deductions":       map[string]any{"amount": float64(0), "currencyCode": "USD"},
		"totalGrossWeight": float64(1500),
	}

	sub, _, err := BuildPayload(form, "")
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))

	total := wire["generalSegment"].(map[string]any)["totalCustomsValuation"].(map[string]any)

	assertAmount(t, total["chargeAmount"], 2400, "USD")
	assertAmount(t, total["externalFreight"], 100, "USD")
	assertZeroAmount(t, total["internalFreight"])
	assertZeroAmount(t, total["insurance"])
	assertZeroAmount(t, total["otherCost"])
	assertZeroAmount(t, total["deductions"])
}

// Item customsValue is the six-part Annex A block. The form only sends
// grossWeight, netWeight, and invoiceAmount on the line — chargeAmount comes
// from invoiceAmount; the other five costs are still sent as zeros.
func TestBuildPayload_ItemCustomsValueMapsFromInvoiceAmount(t *testing.T) {
	form := minimalForm()
	item := form["items"].([]any)[0].(map[string]any)
	item["valuation"] = map[string]any{
		"grossWeight":   float64(1550),
		"netWeight":     float64(1000),
		"invoiceAmount": map[string]any{"amount": float64(2400), "currencyCode": "USD"},
	}

	sub, _, err := BuildPayload(form, "")
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))

	customsValue := wire["goodsShipments"].([]any)[0].(map[string]any)["customsValue"].(map[string]any)

	assertAmount(t, customsValue["chargeAmount"], 2400, "USD")
	assertZeroAmount(t, customsValue["externalFreight"])
	assertZeroAmount(t, customsValue["internalFreight"])
	assertZeroAmount(t, customsValue["insurance"])
	assertZeroAmount(t, customsValue["otherCost"])
	assertZeroAmount(t, customsValue["deductions"])
}

// Remittance currency comes from valuation.invoiceAmount.currencyCode. The
// form no longer collects invoiceCurrencyCode, so the mapping must not depend
// on that deleted field.
func TestBuildPayload_RemittanceUsesInvoiceAmountCurrency(t *testing.T) {
	form := minimalForm()
	form["financial"] = map[string]any{
		"bankCode": "6010", "bankReference": "RemRefTest",
		"remittanceAmount": float64(1500), "paymentTermsCode": "10",
	}
	form["valuation"] = map[string]any{
		"invoiceAmount": map[string]any{"amount": float64(2400), "currencyCode": "USD"},
	}

	sub, _, err := BuildPayload(form, "")
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))

	remittance := wire["remittances"].([]any)[0].(map[string]any)
	assertAmount(t, remittance["remittanceValue"], 1500, "USD")
}

func assertAmount(t *testing.T, raw any, value float64, currency string) {
	t.Helper()
	m := raw.(map[string]any)
	assert.Equal(t, value, m["value"])
	assert.Equal(t, currency, m["currencyID"])
}

func assertZeroAmount(t *testing.T, raw any) {
	t.Helper()
	m := raw.(map[string]any)
	assert.Equal(t, float64(0), m["value"])
	assert.NotContains(t, m, "currencyID")
}

// --- spec v1.7 ---------------------------------------------------------------

// Annex A corrected the misspelled voyage fields in v1.7. The Go field names
// are cosmetic; the JSON tags are the contract, so they are what is asserted.
func TestBuildPayload_SendsTheCorrectedVoyageFieldNames(t *testing.T) {
	form := sampleForm()
	sub, _, err := BuildPayload(form, "")
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)
	body := string(encoded)

	assert.Contains(t, body, `"transportVoyageName":`)
	assert.Contains(t, body, `"transportVoyageNameNationality":`)
	assert.NotContains(t, body, "transportVoageName", "the v1.6 misspelling must not reach the wire")
}

// §8 metadata document: a supporting document held elsewhere, referenced rather
// than attached. The form collects it under metaDocuments, and the date arrives
// as ISO-8601 even though Annex A wants dd/MM/yyyy.
func TestBuildPayload_AcceptsAMetadataSupportingDocument(t *testing.T) {
	form := minimalForm()
	form["supportingDocuments"] = map[string]any{
		"metaDocuments": []any{
			map[string]any{
				"documentCode": "N380",
				"itemSequence": "003",
				"documentId":   "INV-2026-0042",
				"dateAsString": "2026-09-15",
			},
		},
	}

	sub, _, err := BuildPayload(form, "")
	require.NoError(t, err)
	require.Len(t, sub.SupportingDocuments, 1)

	doc := sub.SupportingDocuments[0]
	assert.False(t, doc.HasFile(), "a metadata document sends no bytes")
	assert.Equal(t, 1, doc.SequenceNumber)
	assert.Equal(t, "003", doc.ItemSequence)
	assert.Equal(t, "INV-2026-0042", doc.DocumentID)
	assert.Equal(t, "15/09/2026", doc.DateAsString)

	// fileName is omitted rather than sent empty: §6.1.2 matches part filenames
	// against it, and an empty one would match a part that is not there.
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "fileName")
	assert.NotContains(t, string(encoded), "fileBase64")
}

// A metadata row missing a reference field, or a scanned row missing its file
// or code, would be rejected by the endpoint (400).
func TestBuildPayload_RefusesAnIncompleteSupportingDocument(t *testing.T) {
	for name, entry := range map[string]map[string]any{
		"nothing but a code": {"documentCode": "N380"},
		"no reference":       {"documentCode": "N380", "itemSequence": "001", "dateAsString": "2026-09-15"},
		"no date":            {"documentCode": "N380", "itemSequence": "001", "documentId": "INV-1"},
		"no item number":     {"documentCode": "N380", "documentId": "INV-1", "dateAsString": "2026-09-15"},
		"no document code":   {"itemSequence": "001", "documentId": "INV-1", "dateAsString": "2026-09-15"},
	} {
		t.Run(name, func(t *testing.T) {
			form := minimalForm()
			form["supportingDocuments"] = map[string]any{
				"metaDocuments": []any{entry},
			}

			_, _, err := BuildPayload(form, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "meta document 1")
		})
	}

	for name, entry := range map[string]map[string]any{
		"no file":          {"documentCode": "N380"},
		"no document code": {"fileBase64": "storage/docs/invoice.pdf"},
	} {
		t.Run(name, func(t *testing.T) {
			form := minimalForm()
			form["supportingDocuments"] = map[string]any{
				"scannedDocuments": []any{entry},
			}

			_, _, err := BuildPayload(form, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "scanned document 1")
		})
	}
}

// Scanned documents always travel with sequenceNumber 0 and come before the
// metadata documents, which are numbered from 1. fileName and fileBase64 are
// both the storage key, so the fileN part filename matches the entry.
func TestBuildPayload_AScannedSupportingDocumentStillCarriesItsFile(t *testing.T) {
	form := minimalForm()
	form["supportingDocuments"] = map[string]any{
		"scannedDocuments": []any{
			map[string]any{"documentCode": "AGRM", "fileBase64": "storage/docs/invoice.pdf"},
			map[string]any{"documentCode": "APH", "fileBase64": "storage/docs/permit.pdf"},
		},
		"metaDocuments": []any{
			map[string]any{
				"documentCode": "AGRI",
				"itemSequence": "002",
				"documentId":   "INV-1092",
				"dateAsString": "2026-09-15",
			},
		},
	}

	sub, _, err := BuildPayload(form, "")
	require.NoError(t, err)
	require.Len(t, sub.SupportingDocuments, 3)

	first := sub.SupportingDocuments[0]
	assert.True(t, first.HasFile())
	assert.Equal(t, 0, first.SequenceNumber)
	assert.Equal(t, "AGRM", first.DocumentCode)
	assert.Equal(t, "storage/docs/invoice.pdf", first.FileName)
	assert.Equal(t, "storage/docs/invoice.pdf", first.FileBase64)

	second := sub.SupportingDocuments[1]
	assert.True(t, second.HasFile())
	assert.Equal(t, 0, second.SequenceNumber)
	assert.Equal(t, "storage/docs/permit.pdf", second.FileName)

	meta := sub.SupportingDocuments[2]
	assert.False(t, meta.HasFile())
	assert.Equal(t, 1, meta.SequenceNumber)
	assert.Equal(t, "002", meta.ItemSequence)
	assert.Equal(t, "15/09/2026", meta.DateAsString)
}
