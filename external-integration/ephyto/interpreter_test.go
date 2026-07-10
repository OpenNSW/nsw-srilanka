package ephyto

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/OpenNSW/core/remote"
)

func submitInputs() map[string]any {
	return map[string]any{
		"userform":        sampleUserform(),
		"certificate_id":  "LK-2026-000123",
		"hub_destination": "LK2",
	}
}

func TestBuildEnvelope_SubmitBuildsDeliverSOAP(t *testing.T) {
	env, err := HubInterpreter{}.BuildEnvelope(OpSubmit, submitInputs())
	if err != nil {
		t.Fatalf("BuildEnvelope(submit): %v", err)
	}
	if !strings.Contains(env, "ValidateAndDeliverEnvelope") {
		t.Errorf("envelope does not target ValidateAndDeliverEnvelope:\n%s", env)
	}
}

func TestBuildEnvelope_SubmitWithoutDestinationIsBuildError(t *testing.T) {
	inputs := submitInputs()
	delete(inputs, "hub_destination")

	_, err := HubInterpreter{}.BuildEnvelope(OpSubmit, inputs)
	if err == nil {
		t.Fatal("expected a build error for a missing destination")
	}

	state, out := HubInterpreter{}.Interpret(OpSubmit, err, nil)
	if state != "" {
		t.Errorf("state = %q, want unchanged (empty) on a build failure", state)
	}
	if out["submitted"] != false {
		t.Errorf("submitted = %v, want false", out["submitted"])
	}
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "could not be submitted") || !strings.Contains(msg, "destination IPPC Hub connection") {
		t.Errorf("error prose = %q, want intro + destination guidance", msg)
	}
}

func TestBuildEnvelope_PollWithoutTrackingIsBuildError(t *testing.T) {
	_, err := HubInterpreter{}.BuildEnvelope(OpPoll, map[string]any{})
	if err == nil {
		t.Fatal("expected a build error for a missing tracking number")
	}

	state, out := HubInterpreter{}.Interpret(OpPoll, err, nil)
	if state != "" {
		t.Errorf("state = %q, want unchanged (empty) on a build failure", state)
	}
	if got, _ := out["error"].(string); got != "No IPPC Hub tracking number is available to poll." {
		t.Errorf("error = %q", got)
	}
}

func TestBuildEnvelope_PollBuildsTrackingSOAP(t *testing.T) {
	env, err := HubInterpreter{}.BuildEnvelope(OpPoll, map[string]any{"tracking_number": "TRK-42"})
	if err != nil {
		t.Fatalf("BuildEnvelope(poll): %v", err)
	}
	if !strings.Contains(env, "GetEnvelopeTrackingInfo") || !strings.Contains(env, "TRK-42") {
		t.Errorf("envelope missing tracking query:\n%s", env)
	}
}

func TestInterpret_SubmitDelivered(t *testing.T) {
	resp := &remote.RawResponse{
		StatusCode: http.StatusOK,
		Body: []byte(`<Envelope><Body><Response>` +
			`<hubDeliveryNumber>HDN-001</hubDeliveryNumber>` +
			`<HUBTrackingInfo>PendingDelivery</HUBTrackingInfo>` +
			`</Response></Body></Envelope>`),
	}

	state, out := HubInterpreter{}.Interpret(OpSubmit, nil, resp)
	if state != "EPHYTO_SUBMITTED" {
		t.Errorf("state = %q, want EPHYTO_SUBMITTED", state)
	}
	if out["submitted"] != true {
		t.Errorf("submitted = %v, want true", out["submitted"])
	}
	if out["tracking_number"] != "HDN-001" {
		t.Errorf("tracking_number = %v", out["tracking_number"])
	}
	if got, _ := out["error"].(string); got != "" {
		t.Errorf("error = %q, want empty", got)
	}
}

func TestInterpret_SubmitSOAPFault(t *testing.T) {
	resp := &remote.RawResponse{
		StatusCode: http.StatusInternalServerError,
		Body: []byte(`<Envelope><Body><Fault>` +
			`<faultstring>NPPO from client certificate not found</faultstring>` +
			`</Fault></Body></Envelope>`),
	}

	state, out := HubInterpreter{}.Interpret(OpSubmit, nil, resp)
	if state != "EPHYTO_SUBMITTED" {
		t.Errorf("state = %q, want EPHYTO_SUBMITTED", state)
	}
	if out["submitted"] != false {
		t.Errorf("submitted = %v, want false", out["submitted"])
	}
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "NPPO from client certificate not found") {
		t.Errorf("error prose = %q, want the Hub fault surfaced", msg)
	}
}

func TestInterpret_SubmitTransportError(t *testing.T) {
	state, out := HubInterpreter{}.Interpret(OpSubmit, errors.New("dial tcp: timeout"), nil)
	if state != "EPHYTO_SUBMITTED" {
		t.Errorf("state = %q, want EPHYTO_SUBMITTED (call was attempted)", state)
	}
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "could not reach the IPPC ePhyto Hub") {
		t.Errorf("error prose = %q, want the unreachable-Hub message", msg)
	}
}

func TestInterpret_Poll(t *testing.T) {
	cases := []struct {
		name      string
		tracking  string
		delivered bool
	}{
		{"pending", "PendingDelivery", false},
		{"delivered", "Delivered", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &remote.RawResponse{
				StatusCode: http.StatusOK,
				Body:       []byte(`<Envelope><Body><HUBTrackingInfo>` + tc.tracking + `</HUBTrackingInfo></Body></Envelope>`),
			}
			state, out := HubInterpreter{}.Interpret(OpPoll, nil, resp)
			if state != "EPHYTO_POLLING" {
				t.Errorf("state = %q, want EPHYTO_POLLING", state)
			}
			if out["delivered"] != tc.delivered {
				t.Errorf("delivered = %v, want %v", out["delivered"], tc.delivered)
			}
			if out["tracking_info"] != tc.tracking {
				t.Errorf("tracking_info = %v, want %q", out["tracking_info"], tc.tracking)
			}
		})
	}
}

func TestInterpret_PollTransportError(t *testing.T) {
	state, out := HubInterpreter{}.Interpret(OpPoll, errors.New("dial tcp: timeout"), nil)
	if state != "EPHYTO_POLLING" {
		t.Errorf("state = %q, want EPHYTO_POLLING (call was attempted)", state)
	}
	if out["delivered"] != false {
		t.Errorf("delivered = %v, want false", out["delivered"])
	}
	if got, _ := out["error"].(string); !strings.Contains(got, "Could not fetch the delivery status") {
		t.Errorf("error = %q", got)
	}
}

func TestBuildEnvelope_UnknownOperation(t *testing.T) {
	_, err := HubInterpreter{}.BuildEnvelope("validate", nil)
	if err == nil {
		t.Fatal("expected an error for an unknown operation")
	}
	_, out := HubInterpreter{}.Interpret("validate", err, nil)
	if got, _ := out["error"].(string); !strings.Contains(got, `Unknown ePhyto Hub operation "validate"`) {
		t.Errorf("error = %q", got)
	}
}
