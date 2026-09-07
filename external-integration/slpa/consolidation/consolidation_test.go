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
	// Both sides travel with it, so the save step can turn the container the
	// trader picked into the sqid the CMS reads.
	assert.Equal(t, []string{"MSCU8492019"}, out["cap_container_numbers"],
		"the real containers a branch chooses from")

	assert.Equal(t, []map[string]any{{
		"sqid": "zyxwvutsrqponmlk", "container_no": "DUMY0000001", "size": "40",
	}}, out[SOContainersKey])
	assert.Equal(t, "DUMY0000001", out["available_so_containers"])
}

// Rows reports what is left to do, by both names, in a settled order. It models
// no pairing at all: the numbers carry no relationship to match on, and each
// branch resolves the one pairing it owns through PairOne.
func TestRows_ListsWhatIsLeftToConsolidate(t *testing.T) {
	rows := Rows(FetchResponse{
		CapContainers: []CapContainer{
			{Sqid: "cap-B", ContainerNo: "TCLU1234567"},
			{Sqid: "cap-A", ContainerNo: "MSCU8492019"},
		},
		SOContainers: []SOContainer{
			{Sqid: "so-1", ContainerNo: "DUMY0000001"},
			{Sqid: "so-2", ContainerNo: "DUMY0000002"},
		},
	})

	require.Len(t, rows, 2)
	assert.Equal(t, Row{CapContainerNo: "MSCU8492019", CapSqid: "cap-A"}, rows[0])
	assert.Equal(t, Row{CapContainerNo: "TCLU1234567", CapSqid: "cap-B"}, rows[1])
}

// A pre-advised container whose number happens to equal a service-order number
// is still not paired with it: the equality means nothing, and treating it as a
// pairing would make behaviour depend on a coincidence. The branch's own
// placeholder is what decides, so the coincidence is not what gets sent.
func TestSave_DoesNotPairOnACoincidentallyMatchingNumber(t *testing.T) {
	inputs := map[string]any{
		ChosenCapKey: "cap-B",
		BranchSOKey:  "DUMY0000002",
		CapContainersKey: []any{
			map[string]any{"sqid": "cap-A", "container_no": "MSCU8492019"},
			map[string]any{"sqid": "cap-B", "container_no": "DUMY0000001"},
		},
		SOContainersKey: []any{
			map[string]any{"sqid": "so-1", "container_no": "DUMY0000001"},
			map[string]any{"sqid": "so-2", "container_no": "DUMY0000002"},
		},
	}

	encoded, err := json.Marshal(NewSaveInterpreter().BuildRequest(inputs).(remote.JSONBody).V)
	require.NoError(t, err)

	var req SaveRequest
	require.NoError(t, json.Unmarshal(encoded, &req))
	require.Len(t, req.Containers, 1)
	assert.Equal(t, "cap-B", req.Containers[0].ID)
	assert.Equal(t, "so-2", req.Containers[0].SOContainerID,
		"the placeholder the branch owns, not the one whose number the container happens to share")
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
	assert.Empty(t, out["cap_container_numbers"], "a paired container is not offered again")
	assert.Equal(t, []string{"MSCU8492019"}, out["already_consolidated"])
	assert.NotContains(t, out, "error")
}

func TestFetch_NothingPreAdvisedYet(t *testing.T) {
	ok, out := NewFetchInterpreter().Interpret(nil, body(t,
		`{"status": 1, "data": {"cap_containers": [], "so_containers": [{"sqid":"so-1","ContainerNumber":"DUMY0000001"}]}}`))

	require.False(t, ok)
	assert.Equal(t, OutcomeBlocked, out["outcome"])
	assert.Empty(t, out["cap_container_numbers"], "nothing pre-advised, nothing to choose from")
	// The step has one button, so waiting is expressed by submitting an empty
	// choice, which loops the lookup. Naming an affordance the form does not
	// have leaves the trader looking for a button that is not there.
	assert.Contains(t, out["error"], "Submit without choosing a container")
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
		ChosenCapKey: "cap-A",
		BranchSOKey:  "TCLU9999999",
		CapContainersKey: []any{
			map[string]any{"sqid": "cap-A", "container_no": "MSCU8492019", "so_container_sqid": nil},
			map[string]any{"sqid": "cap-B", "container_no": "TCLU1234567", "so_container_sqid": nil},
		},
		SOContainersKey: []any{
			map[string]any{"sqid": "so-1", "container_no": "MSCU8492347", "size": "40"},
			map[string]any{"sqid": "so-2", "container_no": "TCLU9999999", "size": "40"},
		},
	}

	body := NewSaveInterpreter().BuildRequest(inputs)

	encoded, err := json.Marshal(body.(remote.JSONBody).V)
	require.NoError(t, err)

	var req SaveRequest
	require.NoError(t, json.Unmarshal(encoded, &req))
	require.Len(t, req.Containers, 1, "the branch consolidates exactly one container")
	assert.Equal(t, "cap-A", req.Containers[0].ID)
	assert.Equal(t, "so-2", req.Containers[0].SOContainerID,
		"the placeholder this branch owns, resolved to its sqid")
}

// A choice SLPA is no longer offering is not sent: the CMS would refuse a sqid
// we could not supply, and its reason would name neither half.
func TestSave_SendsNothingForAChoiceSLPADoesNotHold(t *testing.T) {
	inputs := map[string]any{
		ChosenCapKey:     "cap-gone",
		BranchSOKey:      "TCLU9999999",
		CapContainersKey: []any{},
		SOContainersKey:  []any{map[string]any{"sqid": "so-2", "container_no": "TCLU9999999"}},
	}

	encoded, err := json.Marshal(NewSaveInterpreter().BuildRequest(inputs).(remote.JSONBody).V)
	require.NoError(t, err)

	var req SaveRequest
	require.NoError(t, json.Unmarshal(encoded, &req))
	assert.Empty(t, req.Containers)
}
