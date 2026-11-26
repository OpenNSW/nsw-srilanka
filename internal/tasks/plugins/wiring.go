// Package plugins holds the task types specific to Sri Lanka's integrations
// (SLC Edge, SLPA, the IPPC ePhyto Hub) and wires them into a plugin registry.
// The generic task types they build on live in internal/tasks/tasktypes.
// The taskType keys must match the Type field on SubTaskTemplate configs
// loaded into the artifact registry.
package plugins

import (
	"context"
	"fmt"
	"io"

	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/consolidation"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/ecdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/gatepass"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/invoice"
	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/serviceorder"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/tasktypes"
)

// Task type keys. These must match the SubTaskTemplate.Type values declared
// in the JSON configs loaded into the artifact registry.
const (
	// TaskTypeCustomsCusdecDispatch is the generic AUTH_API_CALL plugin wired
	// with the Sri Lanka Customs (SLC Edge) CusDec response interpreter.
	TaskTypeCustomsCusdecDispatch = "CUSTOMS_CUSDEC_DISPATCH"

	// TaskTypeCustomsCusdecVerify is the same plugin wired with the verify
	// interpreter: the declaration is priced and validated without being
	// registered, so the trader can check it before committing.
	TaskTypeCustomsCusdecVerify = "CUSTOMS_CUSDEC_VERIFY"

	// TaskTypeCustomsCusdecRetrieve is the same plugin wired with the retrieve
	// interpreter: it fetches the declaration as it stands in ASYCUDA World, so
	// the dispatch notes are raised for the containers Customs holds now, after
	// any amendment, rather than the ones the trader first declared.
	TaskTypeCustomsCusdecRetrieve = "CUSTOMS_CUSDEC_RETRIEVE"

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

	// TaskTypeSLPASundryOrder raises a sundry (add-on) order — a crane, a
	// forklift, a weighing — on the same endpoint as the service order. It names
	// the invoice of the order it adds to rather than a declaration, and the
	// trader picks a sundry service per container. It runs in its own flow, which
	// a trader starts from the consignment page once the main order is invoiced.
	TaskTypeSLPASundryOrder = "SLPA_SUNDRY_ORDER"

	// TaskTypeSLPASundryServices looks up the sundry services the CMS offers, so
	// the sundry form lists what SLPA can be ordered for today rather than a copy
	// of it. A GET with nothing to send but the trader's client key.
	TaskTypeSLPASundryServices = "SLPA_SUNDRY_SERVICES"

	// TaskTypeSLPAInvoiceGenerate asks the CMS to issue the official invoice for
	// an approved service order. Asked for rather than waited on: the Single
	// Window knows when the accountant approved the order, so a step that waited
	// to be told the invoice existed left the trader in front of an empty panel
	// for as long as the CMS took to call. The payment against it is still
	// reported by webhook, since that money moves outside the Single Window.
	TaskTypeSLPAInvoiceGenerate = "SLPA_INVOICE_GENERATE"

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

// CustomsSubmissions are where the SLC Edge dispatch steps record what they
// sent, so the callbacks that answer a declaration or a dispatch note can find
// the workflow that is waiting for them. The repositories satisfy them.
type CustomsSubmissions struct {
	Declarations  cusdec.Submissions
	DispatchNotes cdn.Submissions
	// Cases finds a case's registered declaration (CUSTOMS_CASE_DECLARATION).
	Cases CaseDeclarations
	// Notes reads and updates dispatch notes (CUSTOMS_CDN_CANCEL).
	Notes CDNNotes
}

// Register installs the Sri Lankan task types on reg. Most are the generic
// AUTH_API_CALL plugin (tasktypes.APICallPlugin) wired with an integration's
// interpreter; files lets an interpreter attach a trader's uploads, and customs
// records what the SLC Edge steps sent.
func Register(reg *flowplugins.Registry, mgr *remote.Manager, files FileFetcher, customs CustomsSubmissions) error {
	if reg == nil {
		return fmt.Errorf("plugins: registry is nil")
	}
	if mgr == nil {
		return fmt.Errorf("plugins: remote manager is nil")
	}

	entries := []struct {
		taskType string
		plugin   flowplugins.TaskPlugin
	}{
		{TaskTypeCustomsCusdecDispatch, tasktypes.NewAPICallPluginWithInterpreter(mgr, cusdec.NewCusdecInterpreter(files, customs.Declarations))},
		{TaskTypeCustomsCusdecVerify, tasktypes.NewAPICallPluginWithInterpreter(mgr, cusdec.NewVerifyInterpreter())},
		{TaskTypeCustomsCusdecRetrieve, tasktypes.NewAPICallPluginWithInterpreter(mgr, cusdec.NewRetrieveInterpreter())},
		{TaskTypeCustomsCDNDispatch, tasktypes.NewAPICallPluginWithInterpreter(mgr, cdn.NewCDNInterpreter(customs.DispatchNotes))},
		{TaskTypeCustomsBoatNotePrefill, NewBoatNotePrefillPlugin(mgr)},
		{TaskTypeCustomsCaseDeclaration, NewCaseDeclarationPlugin(customs.Cases)},
		{TaskTypeCustomsCDNCancel, NewCDNCancelPlugin(customs.Notes)},
		{TaskTypeSLPAECDNUpload, tasktypes.NewAPICallPluginWithInterpreter(mgr, ecdn.NewInterpreter())},
		{TaskTypeSLPAServiceOrder, tasktypes.NewAPICallPluginWithInterpreter(mgr, serviceorder.NewInterpreter())},
		{TaskTypeSLPASundryOrder, tasktypes.NewAPICallPluginWithInterpreter(mgr, serviceorder.NewSundryInterpreter())},
		{TaskTypeSLPASundryServices, tasktypes.NewAPICallPluginWithInterpreter(mgr, serviceorder.NewServicesInterpreter())},
		{TaskTypeSLPAInvoiceGenerate, tasktypes.NewAPICallPluginWithInterpreter(mgr, invoice.NewGenerateInterpreter())},
		{TaskTypeSLPAConsolidationFetch, tasktypes.NewAPICallPluginWithInterpreter(mgr, consolidation.NewFetchInterpreter())},
		{TaskTypeSLPAConsolidationSave, tasktypes.NewAPICallPluginWithInterpreter(mgr, consolidation.NewSaveInterpreter())},
		{TaskTypeSLPAConsolidationDelete, tasktypes.NewAPICallPluginWithInterpreter(mgr, consolidation.NewDeleteInterpreter())},
		{TaskTypeSLPAGatePass, tasktypes.NewAPICallPluginWithInterpreter(mgr, gatepass.NewInterpreter())},
		{TaskTypeNPQSEphytoHub, flowplugins.NewSOAPCallPlugin(mgr, ephyto.NewHubInterpreter(files))},
	}

	for _, e := range entries {
		if err := reg.Register(e.taskType, e.plugin); err != nil {
			return fmt.Errorf("plugins: register %s: %w", e.taskType, err)
		}
	}
	return nil
}
