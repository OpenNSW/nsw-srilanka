package boatnote

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
)

// fakeEdge answers each dispatch note retrieve from a canned body, keyed by the
// note's number. It records the bodies it was sent.
type fakeEdge struct {
	notes map[float64]string
	sent  []map[string]any
}

func (f *fakeEdge) Post(_ context.Context, path string, body any) (map[string]any, error) {
	raw, _ := json.Marshal(body)
	var sent map[string]any
	_ = json.Unmarshal(raw, &sent)
	sent["_path"] = path
	f.sent = append(f.sent, sent)

	if path != CDNRetrievePath {
		return nil, errors.New("unexpected path " + path)
	}
	n, _ := sent["cdnRef"].(map[string]any)["number"].(float64)
	answer, ok := f.notes[n]
	if !ok {
		return nil, errors.New("404 Not Found")
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(answer), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

const declaration = `{
	"eventType": "CUSDEC_RETRIEVED",
	"payload": {
		"cusdec": {
			"baseGeneralSegment": {
				"exporter": {"id": "900000000001", "name": "JOTHI COCONUT EXPORTERS", "address": "45A, KANDY ROAD"},
				"importer": {"id": "IMP882", "name": "COCO WORLD SDN BHD", "address": "125, PUTRAJAYA"}
			},
			"generalSegment": {
				"numberOfPackages": 160,
				"transportVesselName": "CMA CGM MARCO POLO",
				"transportVoyageName": "MARCO POLO",
				"placeOfDischarge": "DEHAM"
			},
			"goodsShipments": [
				{"commodity": {"commercialDescription": "Coconut Oil", "classification": "1513110000",
					"goodsMeasure": {"grossMassMeasure": {"value": 16500.5}, "netWeightMeasure": {"value": 16000}}}},
				{"commodity": {"commercialDescription": "Desiccated Coconut", "classification": "0801110000",
					"goodsMeasure": {"grossMassMeasure": {"value": 1000}, "netWeightMeasure": {"value": 950}}}}
			]
		},
		"cdnRefs": [
			{"office": "CBEX1", "year": "2026", "serial": "C", "number": 1, "status": "Registered"},
			{"office": "CBEX1", "year": "2026", "serial": "C", "number": 2, "status": "Cancelled"},
			{"office": "CBEX1", "year": "2026", "serial": "C", "number": 3, "status": "Registered"},
			{"office": "CBEX1", "year": "2026", "serial": "C", "number": 4.5, "status": "Registered"},
			{"office": "CBEX1", "year": "2026", "serial": "C", "number": 5, "status": "CANCELLED"}
		]
	}
}`

func note(container, seal, mark string, volume float64) string {
	raw, _ := json.Marshal(map[string]any{"eventType": "CDN_RETRIEVED", "payload": map[string]any{"cdn": map[string]any{
		"voyageNumber": "V092", "voyageDateAsString": "2026-06-25", "vessel": "CMA CGM MARCO POLO",
		"vesselOpCode": "OP-MSC", "contOpCode": "CO-882", "placeOfLoading": "LKCMB", "placeOfDischarge": "DEHAM",
		"containerNumber": container, "sealNo": seal, "containerMark": mark, "volume": volume,
	}}})
	return string(raw)
}

// retrievedInput is the prefill input for a declaration retrieve answer, read
// the way the CUSTOMS_CUSDEC_RETRIEVE step captures it.
func retrievedInput(t *testing.T, answer string) Input {
	t.Helper()
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(answer), &resp))
	return Input{
		Declaration:   document.RetrievedDeclaration(resp),
		CDNRefs:       document.RetrievedCDNRefs(resp),
		DeclarantCode: "D0012345",
		CompanyCode:   "900000000001",
		Fallback:      []map[string]any{{"cdn_number": "CBEX1/2026/C/1"}},
	}
}

// The boat note is filled from the declaration and the dispatch notes Customs
// holds against it: one container row per note that is not cancelled.
func TestPrefill_FillsTheBoatNoteFromCustoms(t *testing.T) {
	edge := &fakeEdge{notes: map[float64]string{
		1: note("MSCU8492019", "894077", "MSCU-849201-9", 32.5),
		3: note("MSCU1111111", "894100", "MSCU-111111-1", 30),
	}}

	got := Prefill(context.Background(), edge, retrievedInput(t, declaration))

	require.True(t, got.Retrieved)
	assert.Equal(t, []map[string]any{
		{"cdn_number": "CBEX1/2026/C/1", "container_number": "MSCU8492019", "customs_seal_no": "894077"},
		{"cdn_number": "CBEX1/2026/C/3", "container_number": "MSCU1111111", "customs_seal_no": "894100"},
	}, got.Form["containers"], "the cancelled note has no row")
	for field, want := range map[string]any{
		"shipper":                   "JOTHI COCONUT EXPORTERS, 45A, KANDY ROAD",
		"consignee":                 "COCO WORLD SDN BHD, 125, PUTRAJAYA",
		"exporters_registration_no": "900000000001",
		"vessel":                    "CMA CGM MARCO POLO",
		"voyage_no_date":            "V092 / 2026-06-25",
		"port_of_loading":           "LKCMB",
		"port_of_discharge":         "DEHAM",
		"vessel_op_code":            "OP-MSC",
		"cont_op_code":              "CO-882",
		"description_of_goods":      "Coconut Oil; Desiccated Coconut",
		"cccn_no":                   "1513110000; 0801110000",
		"number_of_packages":        "160",
		"gross_mass":                17500.5,
		"net_mass":                  16950.0,
		"cube_m3":                   62.5,
		"marks_nos_container_no":    "MSCU-849201-9, MSCU-111111-1",
	} {
		assert.Equal(t, want, got.Form[field], field)
	}
}

// Only the dispatch notes are retrieved, each by reference with the codes the
// declaration was made under: the declaration comes already retrieved.
func TestPrefill_RetrievesOnlyTheDispatchNotes(t *testing.T) {
	edge := &fakeEdge{notes: map[float64]string{}}

	Prefill(context.Background(), edge, retrievedInput(t, declaration))

	require.Len(t, edge.sent, 2, "the two notes that are not cancelled, and no declaration")
	for _, sent := range edge.sent {
		assert.Equal(t, CDNRetrievePath, sent["_path"])
		assert.Equal(t, "D0012345", sent["declarantCode"])
		assert.Equal(t, "900000000001", sent["companyCode"])
	}
	assert.Equal(t, map[string]any{"office": "CBEX1", "year": "2026", "serial": "C", "number": float64(1)}, edge.sent[0]["cdnRef"])
	assert.Equal(t, map[string]any{"office": "CBEX1", "year": "2026", "serial": "C", "number": float64(3)}, edge.sent[1]["cdnRef"])
}

// A dispatch note that cannot be retrieved keeps its row, with only its number,
// so the rows still match the notes Customs holds.
func TestPrefill_ANoteNotRetrievedKeepsItsRow(t *testing.T) {
	edge := &fakeEdge{notes: map[float64]string{1: note("MSCU8492019", "894077", "M", 0)}}

	got := Prefill(context.Background(), edge, retrievedInput(t, declaration))

	require.Len(t, got.Form["containers"], 2)
	assert.Equal(t, map[string]any{"cdn_number": "CBEX1/2026/C/3"}, got.Form["containers"].([]map[string]any)[1])
	assert.True(t, got.Retrieved, "one of the notes was retrieved")
}

// When no listed dispatch note can be retrieved, the rows carry only their
// numbers, and the form is not reported as built from what Customs holds.
func TestPrefill_NoNoteRetrievedIsNotRetrieved(t *testing.T) {
	got := Prefill(context.Background(), &fakeEdge{}, retrievedInput(t, declaration))

	require.Len(t, got.Form["containers"], 2)
	assert.False(t, got.Retrieved)
}

// Without a retrieved declaration (the retrieve failed, or answered without
// one), the boat note is as before: one row per dispatch note the consignment
// registered, and nothing else filled. Nothing is retrieved.
func TestPrefill_FallsBackWithoutTheDeclaration(t *testing.T) {
	edge := &fakeEdge{}
	in := retrievedInput(t, `{"payload": {}}`)

	got := Prefill(context.Background(), edge, in)

	assert.False(t, got.Retrieved)
	assert.Equal(t, map[string]any{"containers": in.Fallback}, got.Form)
	assert.Empty(t, edge.sent)
}

// A declaration that lists no dispatch notes still fills its own fields, with the
// consignment's rows.
func TestPrefill_NoNotesListedKeepsTheConsignmentRows(t *testing.T) {
	edge := &fakeEdge{}

	got := Prefill(context.Background(), edge,
		retrievedInput(t, `{"payload": {"cusdec": {"generalSegment": {"transportVesselName": "EVER GIVEN"}}, "cdnRefs": []}}`))

	assert.True(t, got.Retrieved)
	assert.Equal(t, "EVER GIVEN", got.Form["vessel"])
	assert.Equal(t, retrievedInput(t, declaration).Fallback, got.Form["containers"])
}
