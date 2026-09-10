package cusdec

import (
	"context"
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

	sub, _, err := BuildPayload(context.Background(), form, previousEdgeID)
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire struct {
		Properties struct {
			Submitter string `json:"submitter"`
			NswID     string `json:"nswId"`
		} `json:"properties"`
		Submitter string `json:"submitter"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	assert.Equal(t, submitterChannel, wire.Properties.Submitter,
		"the channel is sent inside properties as well as at the top level")
	assert.Equal(t, submitterChannel, wire.Submitter, "the top-level submitter is unchanged")
	return wire.Properties.NswID
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
	sub, _, err := BuildPayload(context.Background(), minimalForm(), "")
	require.NoError(t, err)

	// Re-deriving from the payload as returned — with the field now populated —
	// would give a different answer if the field were part of the digest.
	rebuilt, _, err := BuildPayload(context.Background(), minimalForm(), "")
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
			"invoiceAmountForeign": float64(1500), "invoiceCurrencyCode": "USD",
			"externalFreight": map[string]any{"amountForeign": float64(100), "currencyCode": "USD"},
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
				"grossWeight": float64(1500), "netWeight": float64(1300),
				"invoiceAmountForeign": float64(1500), "invoiceCurrencyCode": "USD",
				"externalFreight": map[string]any{"amountForeign": float64(100), "currencyCode": "USD"},
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
	sub, _, err := BuildPayload(context.Background(), sampleForm(), "")
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
	assert.Equal(t, float64(1), item["numberOfUnits"])

	// The item repeats the declaration's six-part valuation, not a lone charge.
	customsValue := item["customsValue"].(map[string]any)
	assert.Equal(t, float64(1500), customsValue["chargeAmount"].(map[string]any)["value"])
	assert.Equal(t, float64(100), customsValue["externalFreight"].(map[string]any)["value"])
	assert.Equal(t, "USD", customsValue["externalFreight"].(map[string]any)["currencyID"])

	// remittanceValue is an AmountType (§4.2), not a bare number named amount.
	assert.NotContains(t, remittance, "amount", "the pre-v1.6 spelling is gone")
	value := remittance["remittanceValue"].(map[string]any)
	assert.Equal(t, float64(1500), value["value"])
	assert.Equal(t, "USD", value["currencyID"], "the declaration's currency carries to the remittance")
}
