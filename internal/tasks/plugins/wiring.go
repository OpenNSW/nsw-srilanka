// Package plugins wires the native taskflow plugins into a plugin registry.
// The taskType keys must match the Type field on SubTaskTemplate configs
// loaded into the artifact registry.
package plugins

import (
	"fmt"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs"
	"github.com/OpenNSW/nsw-srilanka/external-integration/ephyto"
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

	// TaskTypeNPQSEphytoHub is the generic SOAP-call plugin wired with the IPPC
	// ePhyto Hub interpreter; the subtask template's plugin_properties select
	// the service ("ippc_hub") and operation ("submit" or "poll"). The trader
	// drives the flow through the standard task endpoint; submit validates the
	// document (locally and at the Hub) before delivery, so there is no
	// separate validate step.
	TaskTypeNPQSEphytoHub = "NPQS_EPHYTO_HUB"
)

// Register installs the taskv2 plugins on reg.
//
// EXTERNAL_REVIEW uses our local plugin (ExternalReviewPlugin) that resolves
// targets via remote.Manager and posts the OGA submission envelope. Payment
// uses our local plugin (PaymentPlugin) that initiates checkout sessions via
// payments.PaymentService. NOTIFICATION uses NotificationPlugin which
// dispatches SMS/email through notifications.Manager.
func Register(reg *flowplugins.Registry, mgr *remote.Manager, paymentService payment.PaymentService, backendBaseURL string, devMode bool) error {
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
		{TaskTypeCustomsCusdecDispatch, NewAPICallPluginWithInterpreter(mgr, customs.NewCusdecInterpreter())},
		{TaskTypeNPQSEphytoHub, flowplugins.NewSOAPCallPlugin(mgr, ephyto.NewHubInterpreter())},
	}

	for _, e := range entries {
		if err := reg.Register(e.taskType, e.plugin); err != nil {
			return fmt.Errorf("plugins: register %s: %w", e.taskType, err)
		}
	}
	return nil
}
