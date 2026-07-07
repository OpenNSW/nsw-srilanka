package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto"
	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto/hub"
)

// The NPQS IPPC ePhyto Hub integration is a single in-process taskflow plugin
// (NPQS_EPHYTO_HUB) so the trader drives the whole flow through the standard
// task endpoint — no separate HTTP endpoint or auth. The subtask template's
// plugin_properties select the operation:
//
//	{ "operation": "submit" } — build the ePhyto from the application, validate
//	    it (BuildCertXML rejects an incomplete document, and the Hub's
//	    ValidateAndDeliverEnvelope validates on receipt), send it over mTLS, and
//	    record ephyto.submitted / ephyto.tracking_number / ephyto.error.
//	{ "operation": "poll" }   — query GetEnvelopeTrackingInfo for the tracking
//	    number once and set ephyto.delivered / ephyto.tracking_info.
//
// It is an auto plugin (returns nil to advance immediately). The workflow's
// gateways branch on the flags it records, and the trader-driven status-check
// USER_INPUT node re-enters poll on each "Check Status" click, so the poll only
// runs when the trader asks for it and never spins on its own.

// ePhyto Hub operations selected via plugin_properties.operation.
const (
	ephytoOpSubmit = "submit"
	ephytoOpPoll   = "poll"
)

// EphytoHubPlugin performs the SOAP/mTLS IPPC Hub calls (submit or poll). The
// Hub client (and its underlying mTLS connection pool) is built once, lazily, on
// first use and reused across executions — rebuilding it per call would bypass
// connection pooling and leak sockets/file descriptors.
type EphytoHubPlugin struct {
	cfg        *ephyto.Config
	clientOnce sync.Once
	client     *hub.Client
	clientErr  error
}

// NewEphytoHubPlugin returns a Hub plugin bound to the Hub config.
func NewEphytoHubPlugin(cfg *ephyto.Config) *EphytoHubPlugin {
	return &EphytoHubPlugin{cfg: cfg}
}

// getClient lazily builds the mTLS Hub client on first use and caches it (and
// any construction error) so every submit/poll reuses the same client.
func (p *EphytoHubPlugin) getClient() (*hub.Client, error) {
	p.clientOnce.Do(func() {
		p.client, p.clientErr = p.cfg.NewHubClient()
	})
	return p.client, p.clientErr
}

type ephytoHubConfig struct {
	Operation string `json:"operation"`
}

func (p *EphytoHubPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg ephytoHubConfig
	if len(configRaw) > 0 && string(configRaw) != "null" {
		if err := json.Unmarshal(configRaw, &cfg); err != nil {
			return fmt.Errorf("npqs_ephyto_hub: invalid config: %w", err)
		}
	}

	switch cfg.Operation {
	case ephytoOpSubmit:
		return p.submit(ctx)
	case ephytoOpPoll:
		return p.poll(ctx)
	default:
		return fmt.Errorf("npqs_ephyto_hub: unknown operation %q (want %q or %q)", cfg.Operation, ephytoOpSubmit, ephytoOpPoll)
	}
}

// submit sends the ePhyto to the IPPC Hub via ValidateAndDeliverEnvelope (SOAP
// over mTLS) and records the delivery outcome.
func (p *EphytoHubPlugin) submit(ctx pluginContext) error {
	const intro = "The phytosanitary certificate could not be submitted to the IPPC ePhyto Hub:"
	out := map[string]any{"submitted": false, "error": ""}

	in := ephyto.BuildInput(ctx.Inputs)
	if in.SOAP.To == "" {
		out["error"] = intro + "\n\n- Please select a destination IPPC Hub connection before submitting."
		writeEphytoOutput(ctx, out)
		return nil
	}
	certXML, err := ephyto.BuildCertXML(in)
	if err != nil {
		out["error"] = intro + "\n\n- " + err.Error()
		writeEphytoOutput(ctx, out)
		return nil
	}

	client, err := p.getClient()
	if err != nil {
		out["error"] = intro + "\n\n- " + err.Error()
		writeEphytoOutput(ctx, out)
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx.Context, p.cfg.Timeout)
	defer cancel()

	resp, callErr := client.Send(cctx, ephyto.BuildDeliverSOAP(in, certXML))
	if callErr != nil {
		slog.Warn("ephyto submit: hub error", "taskId", ctx.Record.TaskID, "error", callErr)
	}

	submitted := callErr == nil && resp != nil && resp.Delivered()
	out["submitted"] = submitted
	if resp != nil {
		if resp.HubDeliveryNumber != "" {
			out["tracking_number"] = resp.HubDeliveryNumber
		}
		if resp.HUBTrackingInfo != "" {
			out["tracking_info"] = resp.HUBTrackingInfo
		}
	}
	if !submitted {
		out["error"] = ephyto.DescribeFailure(intro, callErr, resp)
	}
	writeEphytoOutput(ctx, out)
	ctx.Record.State = "EPHYTO_SUBMITTED"
	slog.Info("ephyto submit", "taskId", ctx.Record.TaskID, "submitted", submitted, "tracking", out["tracking_number"])
	return nil
}

// poll queries the Hub once for the delivery status of a previously submitted
// envelope and records whether it has been delivered to the importing NPPO. The
// trader triggers each poll by clicking "Check Status"; the workflow gateway
// loops back to the status-check screen while ephyto.delivered is false and ends
// the flow once it is true.
func (p *EphytoHubPlugin) poll(ctx pluginContext) error {
	out := map[string]any{"delivered": false, "error": ""}

	tracking, _ := ctx.Inputs["tracking_number"].(string)
	if tracking == "" {
		out["error"] = "No IPPC Hub tracking number is available to poll."
		writeEphytoOutput(ctx, out)
		return nil
	}

	client, err := p.getClient()
	if err != nil {
		out["error"] = err.Error()
		writeEphytoOutput(ctx, out)
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx.Context, p.cfg.Timeout)
	defer cancel()

	resp, callErr := client.GetEnvelopeTrackingInfo(cctx, tracking)
	if callErr != nil {
		slog.Warn("ephyto poll: hub error", "taskId", ctx.Record.TaskID, "tracking", tracking, "error", callErr)
	}
	if resp != nil && resp.HUBTrackingInfo != "" {
		out["tracking_info"] = resp.HUBTrackingInfo
	}
	delivered := callErr == nil && resp != nil && ephyto.IsDelivered(resp.HUBTrackingInfo)
	out["delivered"] = delivered
	if callErr != nil {
		out["error"] = "Could not fetch the delivery status from the IPPC ePhyto Hub. Please check again."
	}
	writeEphytoOutput(ctx, out)
	ctx.Record.State = "EPHYTO_POLLING"
	slog.Info("ephyto poll", "taskId", ctx.Record.TaskID, "delivered", delivered, "status", out["tracking_info"])
	return nil
}

// writeEphytoOutput records the plugin result under the task's output namespace,
// replacing it wholesale (so stale keys from a prior loop iteration never leak).
func writeEphytoOutput(ctx pluginContext, out map[string]any) {
	ns := ctx.OutputNamespace
	if ns == "" {
		return
	}
	if ctx.Record.Data == nil {
		ctx.Record.Data = map[string]any{}
	}
	ctx.Record.Data[ns] = out
}
