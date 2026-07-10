// Package ephyto is the National Single Window's in-repo IPPC ePhyto Hub
// integration. The Hub is a registered remote service ("ippc_hub" in
// services.json — endpoint, timeout, mTLS client certificate), called by the
// generic SOAP-call taskflow plugin wired with the HubInterpreter below. The
// ePhyto/SOAP building blocks live in the sibling spscert and hub packages
// (ported from the OpenNSW external-integrations-sandbox `ippc` module); the
// friendly-JSON mapping and helpers live in build.go.
package ephyto

import (
	"errors"
	"fmt"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/hub"
)

// The NPQS IPPC ePhyto Hub integration is the generic SOAP-call plugin
// (NPQS_EPHYTO_HUB) wired with this interpreter, so the trader drives the
// whole flow through the standard task endpoint — no separate HTTP endpoint
// or auth. Transport (endpoint, timeout, mTLS client certificate) is the
// "ippc_hub" service in services.json. The subtask template's
// plugin_properties select the operation:
//
//	{ "operation": "submit" } — build the ePhyto from the application, validate
//	    it (BuildCertXML rejects an incomplete document, and the Hub's
//	    ValidateAndDeliverEnvelope validates on receipt), send it over mTLS, and
//	    record ephyto.submitted / ephyto.tracking_number / ephyto.error.
//	{ "operation": "poll" }   — query GetEnvelopeTrackingInfo for the tracking
//	    number once and set ephyto.delivered / ephyto.tracking_info.
//
// It is an auto plugin (the generic plugin returns nil to advance
// immediately). The workflow's gateways branch on the flags recorded here,
// and the trader-driven status-check USER_INPUT node re-enters poll on each
// "Check Status" click, so the poll only runs when the trader asks for it and
// never spins on its own.

// Hub operations selected via plugin_properties.operation.
const (
	OpSubmit = "submit"
	OpPoll   = "poll"
)

// buildError marks a failure to construct the envelope (an incomplete
// document, a missing selection) as opposed to a transport failure, so
// Interpret can surface the message to the trader verbatim.
type buildError struct{ msg string }

func (e *buildError) Error() string { return e.msg }

// HubInterpreter adapts the generic SOAP-call plugin to the IPPC ePhyto Hub:
// it builds the Deliver/GetTrackingInfo envelopes from the task inputs and
// interprets the Hub's responses into the flags the workflow gateways branch
// on.
type HubInterpreter struct{}

// NewHubInterpreter returns the IPPC ePhyto Hub interpreter.
func NewHubInterpreter() *HubInterpreter { return &HubInterpreter{} }

func (HubInterpreter) BuildEnvelope(operation string, inputs map[string]any) (string, error) {
	switch operation {
	case OpSubmit:
		in := BuildInput(inputs)
		if in.SOAP.To == "" {
			return "", &buildError{"Please select a destination IPPC Hub connection before submitting."}
		}
		certXML, err := BuildCertXML(in)
		if err != nil {
			return "", &buildError{err.Error()}
		}
		return BuildDeliverSOAP(in, certXML), nil
	case OpPoll:
		tracking, _ := inputs["tracking_number"].(string)
		if tracking == "" {
			return "", &buildError{"No IPPC Hub tracking number is available to poll."}
		}
		return hub.BuildGetTrackingSOAP(tracking), nil
	default:
		return "", &buildError{fmt.Sprintf("Unknown ePhyto Hub operation %q (want %q or %q).", operation, OpSubmit, OpPoll)}
	}
}

func (i HubInterpreter) Interpret(operation string, callErr error, resp *remote.RawResponse) (string, map[string]any) {
	if operation == OpPoll {
		return i.interpretPoll(callErr, resp)
	}
	return i.interpretSubmit(callErr, resp)
}

// interpretSubmit records the ValidateAndDeliverEnvelope outcome. A build
// failure keeps the record state unchanged (the call never happened); an
// attempted call always moves the state to EPHYTO_SUBMITTED and the recorded
// flags drive the gateways.
func (HubInterpreter) interpretSubmit(callErr error, resp *remote.RawResponse) (string, map[string]any) {
	const intro = "The phytosanitary certificate could not be submitted to the IPPC ePhyto Hub:"
	out := map[string]any{"submitted": false, "error": ""}

	var be *buildError
	if errors.As(callErr, &be) {
		out["error"] = intro + "\n\n- " + be.msg
		return "", out
	}

	hubResp, callErr := parseHubResponse(callErr, resp)
	submitted := callErr == nil && hubResp != nil && hubResp.Delivered()
	out["submitted"] = submitted
	if hubResp != nil {
		if hubResp.HubDeliveryNumber != "" {
			out["tracking_number"] = hubResp.HubDeliveryNumber
		}
		if hubResp.HUBTrackingInfo != "" {
			out["tracking_info"] = hubResp.HUBTrackingInfo
		}
	}
	if !submitted {
		out["error"] = DescribeFailure(intro, callErr, hubResp)
	}
	return "EPHYTO_SUBMITTED", out
}

// interpretPoll records whether a previously submitted envelope has been
// delivered to the importing NPPO. The trader triggers each poll by clicking
// "Check Status"; the workflow gateway loops back to the status-check screen
// while ephyto.delivered is false and ends the flow once it is true.
func (HubInterpreter) interpretPoll(callErr error, resp *remote.RawResponse) (string, map[string]any) {
	out := map[string]any{"delivered": false, "error": ""}

	var be *buildError
	if errors.As(callErr, &be) {
		out["error"] = be.msg
		return "", out
	}

	hubResp, callErr := parseHubResponse(callErr, resp)
	if hubResp != nil && hubResp.HUBTrackingInfo != "" {
		out["tracking_info"] = hubResp.HUBTrackingInfo
	}
	delivered := callErr == nil && hubResp != nil && IsDelivered(hubResp.HUBTrackingInfo)
	out["delivered"] = delivered
	if callErr != nil {
		out["error"] = "Could not fetch the delivery status from the IPPC ePhyto Hub. Please check again."
	}
	return "EPHYTO_POLLING", out
}

// parseHubResponse parses the raw response when the call produced one, folding
// a parse/fault error into callErr (an earlier transport error takes
// precedence).
func parseHubResponse(callErr error, resp *remote.RawResponse) (*hub.Response, error) {
	if resp == nil {
		return nil, callErr
	}
	hubResp, parseErr := hub.ParseResponse(resp.StatusCode, resp.Body)
	if callErr == nil {
		callErr = parseErr
	}
	return hubResp, callErr
}
