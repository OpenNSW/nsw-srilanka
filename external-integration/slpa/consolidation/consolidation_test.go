package consolidation

import (
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/OpenNSW/core/remote"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/cms"
)

// The sample answer from the CMS's own documentation, which is what the local
// stub replays while SLPA is still in testing.
// The two sides never carry the same number: a cap container is the real
// container the terminal pre-advised, a service-order container is the
// placeholder the order was priced against.
const fetched = `{
  "openapi": "3.0.3",
  "status": 1,
  "data": {
    "status": 1,
    "cap_containers": [
      {"sqid": "9876543210ZYXWVT", "cusdecserial": "CUSDEC-FCL-001", "container_no": "MSCU8492019",
       "container_size": "40", "con_status": "FCL", "so_container_sqid": null}
    ],
    "so_containers": [
      {"sqid": "zyxwvutsrqponmlk", "export_so_id": 1, "ContainerNumber": "DUMY0000001",
       "ContainerSize": "40", "Service": 1}
    ]
  }
}`

func body(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

// The lookup offers both sides; it does not consolidate anything. The pairing is
// the trader's to make, so every pre-advised container becomes a form line.
func TestFetch_OffersTheContainersForTheTraderToPair(t *testing.T) {
	ok, out := NewFetchInterpreter().Interpret(nil, body(t, fetched))

	require.True(t, ok)
	assert.Equal(t, OutcomeReady, out["outcome"])
	// One real container, one placeholder: the pairing is not a choice, so it is
	// pre-filled. Nothing was matched on the number — they differ.
	assert.Equal(t, []map[string]any{{
		"cap_container_no": "MSCU8492019",
		"cap_sqid":         "9876543210ZYXWVT",
		"so_container_no":  "DUMY0000001",
		"consolidate":      true,
	}}, out[RowsKey])

	// The service-order side travels with it, so the save step can turn the
	// container the trader picked into the sqid the CMS reads.
	assert.Equal(t, []map[string]any{{
		"sqid": "zyxwvutsrqponmlk", "container_no": "DUMY0000001", "size": "40",
	}}, out[SOContainersKey])
	assert.Equal(t, "DUMY0000001", out["available_so_containers"])

	// Nothing is consolidated yet: that only follows the trader's submission.
	assert.NotContains(t, out, ConsolidatedKey)
}

// With more than one container on either side the pairing is the trader's, and
// nothing is pre-filled: the numbers carry no relationship to match on, so a
// suggestion would be a guess they might submit unread.
func TestRows_LeavesAnAmbiguousPairingToTheTrader(t *testing.T) {
	rows := Rows(FetchResponse{
		CapContainers: []CapContainer{
			{Sqid: "cap-A", ContainerNo: "MSCU8492019"},
			{Sqid: "cap-B", ContainerNo: "TCLU1234567"},
		},
		SOContainers: []SOContainer{
			{Sqid: "so-1", ContainerNo: "DUMY0000001"},
			{Sqid: "so-2", ContainerNo: "DUMY0000002"},
		},
	})

	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Empty(t, row.SOContainerNo, "nothing to derive the pairing from")
		assert.False(t, row.Consolidate)
	}
	assert.Equal(t, "MSCU8492019", rows[0].CapContainerNo)
	assert.Equal(t, "TCLU1234567", rows[1].CapContainerNo)
}

// A pre-advised container whose number happens to equal a service-order number
// is still not matched: the equality means nothing, and treating it as a pairing
// would make behaviour depend on a coincidence.
func TestRows_DoesNotPairOnAMatchingNumber(t *testing.T) {
	rows := Rows(FetchResponse{
		CapContainers: []CapContainer{
			{Sqid: "cap-A", ContainerNo: "MSCU8492019"},
			{Sqid: "cap-B", ContainerNo: "DUMY0000001"},
		},
		SOContainers: []SOContainer{
			{Sqid: "so-1", ContainerNo: "DUMY0000001"},
			{Sqid: "so-2", ContainerNo: "DUMY0000002"},
		},
	})

	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Empty(t, row.SOContainerNo)
	}
}

// so_container_sqid carries the pairing once it is made, so an already
// consolidated container is not offered again.
func TestFetch_AlreadyConsolidatedIsNotOfferedAgain(t *testing.T) {
	const raw = `{"status": 1, "data": {"cap_containers": [
	  {"sqid": "cap-A", "container_no": "MSCU8492019", "so_container_sqid": "so-1"}],
	  "so_containers": [{"sqid": "so-1", "ContainerNumber": "DUMY0000001"}]}}`

	ok, out := NewFetchInterpreter().Interpret(nil, body(t, raw))

	assert.True(t, ok)
	assert.Equal(t, OutcomeDone, out["outcome"])
	assert.Empty(t, out[RowsKey])
	assert.Equal(t, []string{"MSCU8492019"}, out["already_consolidated"])
	// A pass is still issued for it, under the real container number.
	assert.Equal(t, []string{"MSCU8492019"}, out[ConsolidatedKey])
	assert.NotContains(t, out, "error")
}

func TestFetch_NothingPreAdvisedYet(t *testing.T) {
	ok, out := NewFetchInterpreter().Interpret(nil, body(t,
		`{"status": 1, "data": {"cap_containers": [], "so_containers": [{"sqid":"so-1","ContainerNumber":"DUMY0000001"}]}}`))

	require.False(t, ok)
	assert.Equal(t, OutcomeBlocked, out["outcome"])
	assert.Empty(t, out[RowsKey], "the form is always recorded, empty included")
	assert.Contains(t, out["error"], "Check Again")
}

func TestFetch_RefusalCarriesTheCMSsOwnReason(t *testing.T) {
	const raw = `{"status": 0, "error": {"code": "CUSDEC_NOT_FOUND",
	  "message": "Invalid Cusdec No or Export Service Order is not yet paid"}}`

	ok, out := NewFetchInterpreter().Interpret(nil, body(t, raw))

	require.False(t, ok)
	assert.Equal(t, OutcomeBlocked, out["outcome"])
	assert.Contains(t, out["error"], "Invalid Cusdec No or Export Service Order is not yet paid")
}

func TestFetch_QueryIsKeyedOnTheCusdecSerial(t *testing.T) {
	q := NewFetchInterpreter().BuildQuery(map[string]any{CusdecInput: " CUSDEC-FCL-001 "})
	assert.Equal(t, url.Values{"cusdecno": []string{"CUSDEC-FCL-001"}}, q)
	assert.Nil(t, NewFetchInterpreter().BuildQuery(map[string]any{}))
}

// This is a GET; the plugin must not be handed a body for it.
func TestFetch_SendsNoBody(t *testing.T) {
	assert.Nil(t, NewFetchInterpreter().BuildRequest(map[string]any{}))
}

// What is saved is what the trader ticked — resolved back to the sqids SLPA
// issued, since they work in container numbers and the CMS works in sqids.
func TestSave_SendsWhatTheTraderSelected(t *testing.T) {
	inputs := map[string]any{
		FormKey: map[string]any{RowsKey: []any{
			map[string]any{"cap_container_no": "MSCU8492019", "cap_sqid": "cap-A", "so_container_no": "DUMY0000001", "consolidate": true},
			map[string]any{"cap_container_no": "TCLU1234567", "cap_sqid": "cap-B", "so_container_no": "DUMY0000002", "consolidate": false},
		}},
		SOContainersKey: []any{
			map[string]any{"sqid": "so-1", "container_no": "DUMY0000001"},
			map[string]any{"sqid": "so-2", "container_no": "DUMY0000002"},
		},
	}

	raw, contentType, err := NewSaveInterpreter().BuildRequest(inputs).Encode()
	require.NoError(t, err)
	assert.Contains(t, contentType, "application/json")

	var sent SaveRequest
	require.NoError(t, json.Unmarshal(raw, &sent))
	assert.Equal(t, SaveRequest{Containers: []Pair{{ID: "cap-A", SOContainerID: "so-1"}}}, sent,
		"an unticked row is a container the trader declined")
	assert.NotContains(t, string(raw), "container_no", "the CMS reads only the two sqids")
}

// Whatever the trader picked is what is sent, looked up by the number they were
// shown — the numbers on the two sides are unrelated by design.
func TestResolve_SendsThePairingTheTraderChose(t *testing.T) {
	selection := Resolve(
		[]Row{{CapContainerNo: "MSCU8492019", CapSqid: "cap-A", SOContainerNo: " dumy0000002 ", Consolidate: true}},
		[]SOContainer{
			{Sqid: "so-1", ContainerNo: "DUMY0000001"},
			{Sqid: "so-2", ContainerNo: "DUMY0000002"},
		})

	assert.Equal(t, []Pair{{ID: "cap-A", SOContainerID: "so-2", ContainerNo: "MSCU8492019"}}, selection.Pairs)
	assert.Empty(t, selection.Unresolved)
}

// A container the CMS does not hold cannot be turned into a sqid, so it is
// reported rather than dropped where nobody would see it.
func TestResolve_ReportsWhatItCannotResolve(t *testing.T) {
	selection := Resolve(
		[]Row{{CapContainerNo: "MSCU8492019", CapSqid: "cap-A", SOContainerNo: "DUMY9999999", Consolidate: true}},
		[]SOContainer{{Sqid: "so-1", ContainerNo: "DUMY0000001"}})

	assert.Empty(t, selection.Pairs)
	assert.Equal(t, []string{"MSCU8492019"}, selection.Unresolved)
}

func TestSave_ReadsTheEnvelopeStatus(t *testing.T) {
	saved, out := NewSaveInterpreter().Interpret(nil, body(t,
		`{"openapi": "3.0.3", "status": 1, "message": "FCL Container consolidation saved successfully."}`))
	require.True(t, saved)
	assert.Equal(t, "FCL Container consolidation saved successfully.", out["message"])

	refused, out := NewSaveInterpreter().Interpret(nil, body(t, `{"status": 0, "message": "nope"}`))
	assert.False(t, refused)
	assert.Contains(t, out["error"], "did not save")

	// A body with no status at all: reporting an unsaved consolidation as done
	// is the worse failure, so this is not read as a save.
	unknown, _ := NewSaveInterpreter().Interpret(nil, body(t, `{"message": "hmm"}`))
	assert.False(t, unknown)
}

func TestSave_UnreachableCMSSaysSo(t *testing.T) {
	saved, out := NewSaveInterpreter().Interpret(errors.New("dial tcp: timeout"), nil)
	require.False(t, saved)
	assert.Contains(t, out["error"], "could not get a usable answer")
}

func TestHeaders_PresentTheClientKey(t *testing.T) {
	for name, i := range map[string]interface {
		BuildHeaders(map[string]any) map[string]string
	}{"fetch": NewFetchInterpreter(), "save": NewSaveInterpreter()} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, map[string]string{cms.ClientKeyHeader: "agztNvLSUA"},
				i.BuildHeaders(map[string]any{cms.ClientKeyInput: " agztNvLSUA "}))
			assert.Nil(t, i.BuildHeaders(map[string]any{}), "no key invents no identity")
		})
	}
}

// The lookup records its own shape, not the CMS's — "container_no" where the
// CMS sends "ContainerNumber" — so a branch's pairing must be resolved by
// reading those fields, not by decoding them with the CMS's tags. Doing the
// latter left every placeholder nameless and reported a correct pairing as one
// SLPA did not hold.
func TestSave_PairsTheBranchesChoiceFromWhatTheLookupRecorded(t *testing.T) {
	inputs := map[string]any{
		ChosenCapKey: "cap-e778-mscu8492019",
		BranchSOKey:  "TCLU9999999",
		CapContainersKey: []any{
			map[string]any{"sqid": "cap-e778-mscu8492019", "container_no": "MSCU8492019", "so_container_sqid": nil},
			map[string]any{"sqid": "cap-e778-tclu1234567", "container_no": "TCLU1234567", "so_container_sqid": nil},
		},
		SOContainersKey: []any{
			map[string]any{"sqid": "so-e778-mscu8492347", "container_no": "MSCU8492347", "size": "40"},
			map[string]any{"sqid": "so-e778-tclu9999999", "container_no": "TCLU9999999", "size": "40"},
		},
	}

	body := NewSaveInterpreter().BuildRequest(inputs)

	encoded, err := json.Marshal(body.(remote.JSONBody).V)
	require.NoError(t, err)

	var req SaveRequest
	require.NoError(t, json.Unmarshal(encoded, &req))
	require.Len(t, req.Containers, 1, "the branch consolidates exactly one container")
	assert.Equal(t, "cap-e778-mscu8492019", req.Containers[0].ID)
	assert.Equal(t, "so-e778-tclu9999999", req.Containers[0].SOContainerID,
		"the placeholder this branch owns, resolved to its sqid")
}

// A choice SLPA is no longer offering is not sent: the CMS would refuse a sqid
// we could not supply, and its reason would name neither half.
func TestSave_SendsNothingForAChoiceSLPADoesNotHold(t *testing.T) {
	inputs := map[string]any{
		ChosenCapKey:     "cap-e778-gone",
		BranchSOKey:      "TCLU9999999",
		CapContainersKey: []any{},
		SOContainersKey:  []any{map[string]any{"sqid": "so-e778-tclu9999999", "container_no": "TCLU9999999"}},
	}

	encoded, err := json.Marshal(NewSaveInterpreter().BuildRequest(inputs).(remote.JSONBody).V)
	require.NoError(t, err)

	var req SaveRequest
	require.NoError(t, json.Unmarshal(encoded, &req))
	assert.Empty(t, req.Containers)
}
