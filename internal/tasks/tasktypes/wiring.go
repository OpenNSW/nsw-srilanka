// Package tasktypes holds the task types any single window can use: the ones a
// template names in a step's type and that need nothing specific to one
// deployment. Register installs them on a plugin registry.
package tasktypes

import (
	"fmt"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
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
)

// Register installs the generic task types on reg.
//
// EXTERNAL_REVIEW uses our local plugin (ExternalReviewPlugin) that resolves
// targets via remote.Manager and starts the OGA's review flow. Payment
// uses our local plugin (PaymentPlugin) that initiates checkout sessions via
// payments.PaymentService. ogaFiles issues the file tokens EXTERNAL_REVIEW
// sends OGAs, and cases the case attributes it sends them.
func Register(reg *flowplugins.Registry, mgr *remote.Manager, paymentService payment.Service, ogaFiles OGAFiles, cases CaseAttributes) error {
	if reg == nil {
		return fmt.Errorf("plugins: registry is nil")
	}
	if mgr == nil {
		return fmt.Errorf("plugins: remote manager is nil")
	}
	if paymentService == nil {
		return fmt.Errorf("plugins: payment service is nil")
	}
	if cases == nil {
		return fmt.Errorf("plugins: case attributes reader is nil")
	}
	if (ogaFiles.ClientFor == nil) != (ogaFiles.IssueForClient == nil) {
		return fmt.Errorf("plugins: OGA file tokens need both a client lookup and an issuer, or neither")
	}
	if ogaFiles.IssueForClient != nil && ogaFiles.RenderConfigOf == nil {
		return fmt.Errorf("plugins: OGA file tokens need a render config resolver")
	}

	entries := []struct {
		taskType string
		plugin   flowplugins.TaskPlugin
	}{
		{TaskTypeUserInput, flowplugins.NewUserInputPlugin()},
		{TaskTypeValueWriter, NewValueWriterPlugin()},
		{TaskTypeExternalReview, NewExternalReviewPlugin(mgr, ogaFiles, cases)},
		{TaskTypePayment, NewPaymentPlugin(paymentService)},
		{TaskTypeAPICall, flowplugins.NewAPICallPlugin(flowplugins.DefaultHTTPDispatcher)},
		{TaskTypeAuthAPICall, NewAPICallPlugin(mgr)},
	}

	for _, e := range entries {
		if err := reg.Register(e.taskType, e.plugin); err != nil {
			return fmt.Errorf("plugins: register %s: %w", e.taskType, err)
		}
	}
	return nil
}
