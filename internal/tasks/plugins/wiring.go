// Package plugins wires the native taskflow plugins into a plugin registry.
// The taskType keys must match the Type field on SubTaskTemplate configs
// loaded into the artifact registry.
package plugins

import (
	"context"
	"fmt"
	"io"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/consolidation"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/ecdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/gatepass"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/serviceorder"
)

// Task type keys. These must match the SubTaskTemplate.Type values declared
// in the JSON configs loaded into the artifact registry.
const (
	TaskTypeUserInput      = "USER_INPUT"
	TaskTypeExternalReview = "EXTERNAL_REVIEW"
	TaskTypePayment        = "PAYMENT"
	TaskTypeAPICall        = "API_CALL"
	TaskTypeAuthAPICall    = "AUTH_API_CALL"
	TaskTypeNotification   = "NOTIFICATION"

	// TaskTypeCustomsCusdecDispatch is the generic AUTH_API_CALL plugin wired
	// with the Sri Lanka Customs (SLC Edge) CusDec response interpreter.
	TaskTypeCustomsCusdecDispatch = "CUSTOMS_CUSDEC_DISPATCH"

	// TaskTypeCustomsCDNDispatch is the generic AUTH_API_CALL plugin wired with
	// the Sri Lanka Customs (SLC Edge) Cargo Dispatch Note interpreter. One
	// dispatch note covers one container, so a consignment fans out to this task
	// once per container (see TaskTypeCDNSplitBuilder).
	TaskTypeCustomsCDNDispatch = "CUSTOMS_CDN_DISPATCH"

	// TaskTypeSLPAECDNUpload is the generic AUTH_API_CALL plugin wired with the
	// SLPA CMS interpreter: it renders the trader's form as the Electronic Cargo
	// Declaration Note and uploads it. SLPA's CMS is the system of record for the
	// declaration, so nothing is validated or stored here beyond the outcome the
	// trader is shown.
	TaskTypeSLPAECDNUpload = "SLPA_ECDN_UPLOAD"

	// TaskTypeSLPAServiceOrder raises the Export Service Order against a
	// declaration the CMS has already validated — the step the ECDN unblocks. The
	// declaration and its containers are on SLPA's side by then, so the trader is
	// asked only which service to order per container, and the CMS derives the
	// cargo type from the CUSDEC record itself.
	TaskTypeSLPAServiceOrder = "SLPA_SERVICE_ORDER"

	// TaskTypeSLPAConsolidationFetch looks up the containers available for
	// consolidation under a CUSDEC serial and matches the two sides SLPA holds
	// separately — the containers a terminal pre-advised and the containers
	// priced on the service order. A GET, so the CUSDEC serial travels in the
	// query rather than a body.
	TaskTypeSLPAConsolidationFetch = "SLPA_CONSOLIDATION_FETCH"

	// TaskTypeSLPAConsolidationSave saves the pairs the lookup matched. Split
	// from the lookup so what will be written is recorded, and shown to the
	// trader, before anything changes on SLPA's side.
	TaskTypeSLPAConsolidationSave = "SLPA_CONSOLIDATION_SAVE"

	// TaskTypeSLPAConsolidationDelete removes a consolidation the trader wants
	// to redo against a different real container. The CMS deletes the pairing
	// and everything hanging off it, so the flow offers it only before a gate
	// pass has been generated.
	TaskTypeSLPAConsolidationDelete = "SLPA_CONSOLIDATION_DELETE"

	// TaskTypeSLPAGatePass issues the export container gate pass, which SLPA
	// grants only against a paid service order. One pass covers one container,
	// so a consignment fans out to this task once per consolidated container.
	// gosec reads "pass" here as a credential; it is the gate pass the haulier
	// presents at the terminal.
	TaskTypeSLPAGatePass = "SLPA_GATE_PASS" //nolint:gosec // G101 false positive: a task type name, not a credential.

	// TaskTypeNPQSEphytoHub is the generic SOAP-call plugin wired with the IPPC
	// ePhyto Hub interpreter; the subtask template's plugin_properties select
	// the service ("ippc_hub") and operation ("submit" or "poll"). The trader
	// drives the flow through the standard task endpoint; submit validates the
	// document (locally and at the Hub) before delivery, so there is no
	// separate validate step.
	TaskTypeNPQSEphytoHub = "NPQS_EPHYTO_HUB"
)

// FileFetcher retrieves an uploaded file's content by storage key, returning
// the content and its MIME type. It is the slice of the storage service an
// interpreter needs to attach a trader's uploads to an outbound call.
//
// Declared here rather than taking *storage.Service so registration stays
// testable, and so no single domain owns what any file-attaching interpreter
// needs. Interpreters declare their own matching interface rather than
// importing this one — they are imported by this package, so depending back on
// it would be a cycle; Go's structural typing means the same value satisfies
// both without the two being coupled.
type FileFetcher interface {
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
}

// Register installs the taskv2 plugins on reg.
//
// EXTERNAL_REVIEW uses our local plugin (ExternalReviewPlugin) that resolves
// targets via remote.Manager and posts the OGA submission envelope. Payment
// uses our local plugin (PaymentPlugin) that initiates checkout sessions via
// payments.PaymentService. NOTIFICATION uses NotificationPlugin which
// dispatches SMS/email through notifications.Manager.
func Register(reg *flowplugins.Registry, mgr *remote.Manager, paymentService payment.PaymentService, files FileFetcher, backendBaseURL string, devMode bool) error {
	if reg == nil {
		return fmt.Errorf("plugins: registry is nil")
	}
	if mgr == nil {
		return fmt.Errorf("plugins: remote manager is nil")
	}
	if paymentService == nil {
		return fmt.Errorf("plugins: payment service is nil")
	}

	entries := []struct {
		taskType string
		plugin   flowplugins.TaskPlugin
	}{
		{TaskTypeUserInput, flowplugins.NewUserInputPlugin()},
		{TaskTypeExternalReview, NewExternalReviewPlugin(mgr, backendBaseURL, devMode)},
		{TaskTypePayment, NewPaymentPlugin(paymentService)},
		{TaskTypeAPICall, flowplugins.NewAPICallPlugin(flowplugins.DefaultHTTPDispatcher)},
		{TaskTypeAuthAPICall, NewAPICallPlugin(mgr)},
		{TaskTypeCustomsCusdecDispatch, NewAPICallPluginWithInterpreter(mgr, cusdec.NewCusdecInterpreter(files))},
		{TaskTypeCustomsCDNDispatch, NewAPICallPluginWithInterpreter(mgr, cdn.NewCDNInterpreter())},
		{TaskTypeSLPAECDNUpload, NewAPICallPluginWithInterpreter(mgr, ecdn.NewInterpreter())},
		{TaskTypeSLPAServiceOrder, NewAPICallPluginWithInterpreter(mgr, serviceorder.NewInterpreter())},
		{TaskTypeSLPAConsolidationFetch, NewAPICallPluginWithInterpreter(mgr, consolidation.NewFetchInterpreter())},
		{TaskTypeSLPAConsolidationSave, NewAPICallPluginWithInterpreter(mgr, consolidation.NewSaveInterpreter())},
		{TaskTypeSLPAConsolidationDelete, NewAPICallPluginWithInterpreter(mgr, consolidation.NewDeleteInterpreter())},
		{TaskTypeSLPAGatePass, NewAPICallPluginWithInterpreter(mgr, gatepass.NewInterpreter())},
		{TaskTypeNPQSEphytoHub, flowplugins.NewSOAPCallPlugin(mgr, ephyto.NewHubInterpreter())},
	}

	for _, e := range entries {
		if err := reg.Register(e.taskType, e.plugin); err != nil {
			return fmt.Errorf("plugins: register %s: %w", e.taskType, err)
		}
	}
	return nil
}
