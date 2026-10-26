// Package systemcaller lets this service's own in-process code complete task steps
// as the system principal (taskauthz.KindSystem).
//
// Some steps are completed by the service itself, on the authority of an event it
// was told about rather than a request a user made: the payment service settling a
// payment, and the customs and SLPA webhook handlers resuming the step a callback
// answers. They call the task manager directly, so no request identity reaches the
// step's authz rule. Completer attaches the system principal, which a rule admits by
// naming taskauthz.SystemPrincipal. It changes nothing else about the call.
package systemcaller

import (
	"context"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// StepCompleter is the part of the task manager that completes a parked step, by its
// ids or by the callback token its dispatch carried. *orchestrator.TaskManager
// satisfies it.
type StepCompleter interface {
	CompleteTaskStep(ctx context.Context, taskID, stepID string, payload map[string]any) error
	CompleteTaskStepByToken(ctx context.Context, token string, payload map[string]any) error
}

// Completer is a StepCompleter whose calls act as the system principal. Give it only
// to code that is itself the authority for the steps it completes.
type Completer struct {
	next StepCompleter
}

// New returns a Completer that completes steps through next.
func New(next StepCompleter) Completer {
	return Completer{next: next}
}

// CompleteTaskStep completes the step through the task manager as the system principal.
func (c Completer) CompleteTaskStep(ctx context.Context, taskID, stepID string, payload map[string]any) error {
	return c.next.CompleteTaskStep(taskauthz.WithSystem(ctx), taskID, stepID, payload)
}

// CompleteTaskStepByToken completes the step the token names, as the system principal.
func (c Completer) CompleteTaskStepByToken(ctx context.Context, token string, payload map[string]any) error {
	return c.next.CompleteTaskStepByToken(taskauthz.WithSystem(ctx), token, payload)
}
