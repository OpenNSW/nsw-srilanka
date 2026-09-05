package cdn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullForm is one container's worth of trader input, in the shape the
// customs-cdn--user-form produces.
func fullForm() map[string]any {
	var form map[string]any
	raw := `{
	  "officeCode": "CBEX1",
	  "cusdecRef": "CBEX1/2026/E/1047",
	  "shipper":   {"id": "EXP001", "name": "ACME Exports", "address": "18 Galle Road, Colombo 03", "countryCode": "LK"},
	  "consignee": {"id": "IMP882", "name": "Hamburg Fine Foods", "address": "Industriestrasse 104", "countryCode": "DE"},
	  "transport": {
	    "voyageNumber": "V092", "voyageDate": "2026-06-25", "vessel": "CMA CGM MARCO POLO",
	    "vesselOpCode": "OP-MSC", "contOpCode": "CO-882",
	    "lorryRegNo": "WP-LY-4920", "trailerRegNo": "TR-8841", "driverName": "K. A. Sunil Perera",
	    "locationOfGoods": "CFS1", "placeOfLoading": "LKCMB", "placeOfDischarge": "DEHAM"
	  },
	  "goods": {
	    "description": "Refined Organic Coconut Oil", "packageNumber": 80, "packageType": "DR",
	    "grossWeight": 16500.5, "volume": 32.5, "bol": "BL-9382019-COL", "tempRequired": 4
	  },
	  "container": {"number": "MSCU8492019", "type": "20GP", "sealNo": "894021", "mark": "MSCU-849201-9"}
	}`
	if err := json.Unmarshal([]byte(raw), &form); err != nil {
		panic(err)
	}
	return form
}

func TestBuildPayload_MapsAnnexBFields(t *testing.T) {
	sub, err := BuildPayload(fullForm(), "6d40eae3-3173-4247-8236-2c587ec50a74")
	require.NoError(t, err)

	assert.Equal(t, "CBEX1", sub.OfficeCode)
	assert.Equal(t, Party{ID: "EXP001", Name: "ACME Exports", Address: "18 Galle Road, Colombo 03", CountryCode: "LK"}, sub.Shipper)
	assert.Equal(t, Party{ID: "IMP882", Name: "Hamburg Fine Foods", Address: "Industriestrasse 104", CountryCode: "DE"}, sub.Consignee)

	assert.Equal(t, "V092", sub.VoyageNumber)
	assert.Equal(t, "2026-06-25", sub.VoyageDateAsString)
	assert.Equal(t, "CMA CGM MARCO POLO", sub.Vessel)
	assert.Equal(t, "WP-LY-4920", sub.LorryRegNo)
	assert.Equal(t, "TR-8841", sub.TrailerRegNo)
	assert.Equal(t, "LKCMB", sub.PlaceOfLoading)
	assert.Equal(t, "DEHAM", sub.PlaceOfDischarge)

	assert.Equal(t, 80, sub.PackageNumber)
	assert.Equal(t, "DR", sub.PackageType)
	assert.Equal(t, 16500.5, sub.GrossWeight)
	assert.Equal(t, 32.5, sub.Volume)

	assert.Equal(t, "MSCU8492019", sub.ContainerNumber)
	assert.Equal(t, "20GP", sub.ContainerType)
	assert.Equal(t, "894021", sub.SealNo)
	assert.Equal(t, "MSCU-849201-9", sub.ContainerMark)

	// §4.1: the display reference is split back into the four canonical parts.
	require.Len(t, sub.CusDecRefs, 1)
	assert.Equal(t, DocumentReference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 1047}, sub.CusDecRefs[0])
}

// The unused optional cost/volume lines must not appear on the wire as bare
// zeroes, and the mandatory ones must, so CIG sees exactly what was declared.
func TestBuildPayload_OmitsEmptyOptionalFields(t *testing.T) {
	form := fullForm()
	goods := form["goods"].(map[string]any)
	delete(goods, "volume")
	delete(goods, "bol")
	delete(goods, "tempRequired")

	sub, err := BuildPayload(form, "")
	require.NoError(t, err)

	body, err := json.Marshal(sub)
	require.NoError(t, err)

	assert.NotContains(t, string(body), `"volume"`)
	assert.NotContains(t, string(body), `"bol"`)
	assert.NotContains(t, string(body), `"tempRequired"`)
	assert.Contains(t, string(body), `"grossWeight":16500.5`)
}

func TestBuildPayload_RejectsUnusableForms(t *testing.T) {
	t.Run("empty form", func(t *testing.T) {
		_, err := BuildPayload(map[string]any{}, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not be read")
	})

	t.Run("no declaration reference", func(t *testing.T) {
		form := fullForm()
		delete(form, "cusdecRef")
		_, err := BuildPayload(form, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not linked to a registered customs declaration")
	})

	t.Run("unparseable declaration reference", func(t *testing.T) {
		form := fullForm()
		form["cusdecRef"] = "CBEX1-2026-E-1047"
		_, err := BuildPayload(form, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "office/year/serial/number")
	})
}

// A CDN may dispatch cargo for several declarations (§7.1), so the builder
// accepts a list as well as the single-reference form.
func TestBuildPayload_MultipleDeclarationReferences(t *testing.T) {
	form := fullForm()
	delete(form, "cusdecRef")
	form["cusDecRefs"] = []any{
		"CBEX1/2026/E/1047",
		map[string]any{"office": "CBEX1", "year": "2026", "serial": "E", "number": float64(1048)},
	}

	sub, err := BuildPayload(form, "")
	require.NoError(t, err)
	require.Len(t, sub.CusDecRefs, 2)
	assert.Equal(t, 1047, sub.CusDecRefs[0].Number)
	assert.Equal(t, 1048, sub.CusDecRefs[1].Number)
}

func TestParseDocumentReference(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		ref, err := ParseDocumentReference(" CBEX1/2026/E/1047 ")
		require.NoError(t, err)
		assert.Equal(t, DocumentReference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 1047}, ref)
	})

	for name, input := range map[string]string{
		"too few parts":      "CBEX1/2026/E",
		"non-numeric number": "CBEX1/2026/E/ABC",
		"empty office":       "/2026/E/1047",
		"zero number":        "CBEX1/2026/E/0",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDocumentReference(input)
			assert.Error(t, err)
		})
	}
}

// JSONForms number widgets round-trip as strings in some browsers; a weight
// that arrived as "16500.5" must not silently become 0 on the wire.
func TestBuildPayload_AcceptsStringNumbers(t *testing.T) {
	form := fullForm()
	goods := form["goods"].(map[string]any)
	goods["grossWeight"] = "16500.5"
	goods["packageNumber"] = "80"

	sub, err := BuildPayload(form, "")
	require.NoError(t, err)
	assert.Equal(t, 16500.5, sub.GrossWeight)
	assert.Equal(t, 80, sub.PackageNumber)
}
