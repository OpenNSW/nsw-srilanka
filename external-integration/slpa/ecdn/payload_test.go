package ecdn

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullForm is the ECDN form as the trader portal submits it.
func fullForm() map[string]any {
	var form map[string]any
	raw := `{
	  "cusdecNo": "CBEX12026E1050", "cusdecDate": "2026-08-19",
	  "cusdecOffice": "BIBE1", "cusdecSerLet": "E",
	  "cdnNumber": "CDN-2026-00481", "terminal": "CICT", "status": "FCL",
	  "shipperName": "JOTHI COCONUT EXPORTERS", "shipperAddress": "45A, KANDY ROAD, PILIMATHALAWA",
	  "consigneeName": "COCO WORLD SDN BHD", "consigneeAddress": "125, PUTRAJAYA, KUALA LUMPUR",
	  "packageNumber": 80, "packageType": "Bags",
	  "volumeCBM": 32.5, "weightKg": 16500.5,
	  "goodsDescription": "Refined Organic Coconut Oil",
	  "voyageNumber": "V092", "voyageDate": "2026-08-20",
	  "containers": [
	    {"containerType":"General","containerSize":"20","sealNumber":"894021",
	     "containerMark":"MSCU-849201-9","commodity":"DC"}
	  ]
	}`
	if err := json.Unmarshal([]byte(raw), &form); err != nil {
		panic(err)
	}
	return form
}

// referenceElements is the element order of the document SLPA's own generator
// produced (CBEX12026E1050_2026_08_20_12_26_07.xml). Their schema validates a
// sequence, so order is part of the contract and not a formatting detail: this
// is the test that would have caught the root element being wrong.
var referenceElements = []string{
	"CusDecNote",
	"CusDecNote/MAIN",
	"CusDecNote/MAIN/CusDecNo", "CusDecNote/MAIN/CusDecDate", "CusDecNote/MAIN/CusDecOffice",
	"CusDecNote/MAIN/CusDecSerial", "CusDecNote/MAIN/Terminal",
	"CusDecNote/MAIN/ShipperName", "CusDecNote/MAIN/ShipperAddress",
	"CusDecNote/MAIN/ConsigneeName", "CusDecNote/MAIN/ConsigneeAddress",
	"CusDecNote/MAIN/PackageNumber", "CusDecNote/MAIN/PackageType",
	"CusDecNote/MAIN/Volume", "CusDecNote/MAIN/Weight",
	"CusDecNote/MAIN/GoodsDescription", "CusDecNote/MAIN/CusDecSerLet", "CusDecNote/MAIN/Status",
	"CusDecNote/SUB",
	"CusDecNote/SUB/CdnNumber", "CusDecNote/SUB/VoyageNumber", "CusDecNote/SUB/VoyageDate",
	"CusDecNote/SUB/PortOfLoading", "CusDecNote/SUB/PortOfUnloading",
	"CusDecNote/SUB/VesselOPCode", "CusDecNote/SUB/ContOPCode", "CusDecNote/SUB/EXVessel",
	"CusDecNote/SUB/SLPANumber", "CusDecNote/SUB/ArrivalOfficer", "CusDecNote/SUB/ArrivalDate",
	"CusDecNote/SUB/DeclarantCode", "CusDecNote/SUB/MsgSeqNo", "CusDecNote/SUB/CdnDate",
	"CusDecNote/SUB/CdnYear", "CusDecNote/SUB/CdnSerial", "CusDecNote/SUB/BillNumber",
	"CusDecNote/SUB/DriverName", "CusDecNote/SUB/CleanerName",
	"CusDecNote/SUB/LorryNumber", "CusDecNote/SUB/TrailerNumber",
	"CusDecNote/CONTAINER",
	"CusDecNote/CONTAINER/container",
	"CusDecNote/CONTAINER/container/ContainerNumber", "CusDecNote/CONTAINER/container/ContainerType",
	"CusDecNote/CONTAINER/container/ContainerSize", "CusDecNote/CONTAINER/container/SealNumber",
	"CusDecNote/CONTAINER/container/ContainerMark", "CusDecNote/CONTAINER/container/Commodity",
}

// elementPaths walks the document in order, so both names and sequence are
// compared.
func elementPaths(t *testing.T, doc string) []string {
	t.Helper()

	var paths []string
	var stack []string
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			stack = append(stack, el.Name.Local)
			paths = append(paths, strings.Join(stack, "/"))
			require.Empty(t, el.Name.Space, "the reference document is not namespace qualified")
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	return paths
}

func TestBuildXML_MatchesTheReferenceStructure(t *testing.T) {
	doc, err := BuildXML(fullForm())
	require.NoError(t, err)

	assert.Equal(t, referenceElements, elementPaths(t, doc))
}

// The unused SUB fields are present but empty in the reference document. An XSD
// sequence treats a missing element and an empty one differently, so omitting
// them — which is what a Go struct does by default — would fail validation.
func TestBuildXML_EmitsEmptyElementsRatherThanOmittingThem(t *testing.T) {
	form := fullForm()
	for _, k := range []string{"voyageNumber", "portOfLoading", "portOfUnloading",
		"vesselOPCode", "contOPCode", "exVessel"} {
		delete(form, k)
	}

	doc, err := BuildXML(form)
	require.NoError(t, err)

	assert.Equal(t, referenceElements, elementPaths(t, doc),
		"an unset optional field must still be emitted as an empty element")
	for _, tag := range []string{"PortOfLoading", "SLPANumber", "DriverName", "TrailerNumber"} {
		assert.Contains(t, doc, "<"+tag+">"+"</"+tag+">", "%s must be present and empty", tag)
	}
}

// Their parser rejects a DOCTYPE outright ("Document types are not allowed"),
// even while their validator reports "no DTD found".
func TestBuildXML_CarriesNoDoctype(t *testing.T) {
	doc, err := BuildXML(fullForm())
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(doc, xml.Header))
	assert.NotContains(t, doc, "<!DOCTYPE")
}

// SLPA's generator writes the office code, declaration number and year run
// together; the trader is never asked for it.
func TestBuildXML_DerivesCusDecSerial(t *testing.T) {
	doc, err := BuildXML(fullForm())
	require.NoError(t, err)

	assert.Contains(t, doc, "<CusDecSerial>BIBE1CBEX12026E10502026</CusDecSerial>")
}

func TestBuildXML_ContainerNumber(t *testing.T) {
	// SLPA's generator puts a per-declaration key in ContainerNumber and the
	// physical container in ContainerMark.
	t.Run("synthesized when the trader gives none", func(t *testing.T) {
		doc, err := BuildXML(fullForm())
		require.NoError(t, err)
		assert.Contains(t, doc, "<ContainerNumber>CBEX12026E1050_2026-08-19_1</ContainerNumber>")
	})

	t.Run("the trader's own number is kept", func(t *testing.T) {
		form := fullForm()
		form["containers"].([]any)[0].(map[string]any)["containerNo"] = "MSCU8492019"

		doc, err := BuildXML(form)
		require.NoError(t, err)
		assert.Contains(t, doc, "<ContainerNumber>MSCU8492019</ContainerNumber>")
	})

	t.Run("numbering continues across containers", func(t *testing.T) {
		form := fullForm()
		first := form["containers"].([]any)[0]
		form["containers"] = []any{first, first}

		doc, err := BuildXML(form)
		require.NoError(t, err)
		assert.Contains(t, doc, "_1</ContainerNumber>")
		assert.Contains(t, doc, "_2</ContainerNumber>")
	})
}

// The reference document has "general" for the form's "General".
func TestBuildXML_LowercasesContainerType(t *testing.T) {
	doc, err := BuildXML(fullForm())
	require.NoError(t, err)
	assert.Contains(t, doc, "<ContainerType>general</ContainerType>")
}

func TestBuildXML_CargoValues(t *testing.T) {
	doc, err := BuildXML(fullForm())
	require.NoError(t, err)

	// Volume and Weight, not Volume_CBM/Weight_Kg.
	assert.Contains(t, doc, "<Volume>32.5</Volume>")
	assert.Contains(t, doc, "<Weight>16500.5</Weight>")
	assert.Contains(t, doc, "<PackageNumber>80</PackageNumber>")
}

// SLPA cannot raise a Service Order without a container, under either the FCL or
// the LCL rules, so this is worth stopping locally rather than round-tripping.
func TestBuildXML_RequiresAtLeastOneContainer(t *testing.T) {
	form := fullForm()
	form["containers"] = []any{}

	_, err := BuildXML(form)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one container")
}

func TestBuildXML_RejectsAnUnreadableForm(t *testing.T) {
	_, err := BuildXML(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be read")
}

// Text the trader typed must not be able to break the document.
func TestBuildXML_EscapesTraderText(t *testing.T) {
	form := fullForm()
	form["goodsDescription"] = `Coconut oil <grade "A"> & husk`

	doc, err := BuildXML(form)
	require.NoError(t, err)

	var out Declaration
	require.NoError(t, xml.Unmarshal([]byte(doc), &out))
	assert.Equal(t, `Coconut oil <grade "A"> & husk`, out.Main.GoodsDescription)
	assert.NotContains(t, doc, "<grade")
}

// JSONForms number widgets round-trip as strings in some browsers; a weight that
// arrives as "16500.5" must not silently become 0 in the declaration.
func TestBuildXML_AcceptsStringNumbers(t *testing.T) {
	form := fullForm()
	form["weightKg"] = "16500.5"
	form["packageNumber"] = "80"

	doc, err := BuildXML(form)
	require.NoError(t, err)
	assert.Contains(t, doc, "<Weight>16500.5</Weight>")
	assert.Contains(t, doc, "<PackageNumber>80</PackageNumber>")
}
