package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/store"

	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// recordingCompleter records what reached the task manager.
type recordingCompleter struct {
	ctx     context.Context
	payload map[string]any
	token   string
}

func (r *recordingCompleter) CompleteTaskStep(ctx context.Context, _, _ string, payload map[string]any) error {
	r.ctx, r.payload = ctx, payload
	return nil
}

func (r *recordingCompleter) CompleteTaskStepByToken(ctx context.Context, token string, payload map[string]any) error {
	r.ctx, r.payload, r.token = ctx, payload, token
	return nil
}

// Both ways in reach the task manager as the system principal, whatever the
// caller's context carried.
func TestSystemCompleter_AttachesSystemPrincipal(t *testing.T) {
	// A context that already carries a user must not lend them its authority.
	userCtx := taskauthz.WithInput(context.Background(), taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}})

	for name, complete := range map[string]func(systemCompleter) error{
		"by ids":   func(s systemCompleter) error { return s.CompleteTaskStep(userCtx, "t1", "s1", map[string]any{}) },
		"by token": func(s systemCompleter) error { return s.CompleteTaskStepByToken(userCtx, "tok", map[string]any{}) },
	} {
		rec := &recordingCompleter{}
		if err := complete(systemCompleter{tm: rec}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		in, ok := taskauthz.InputFromContext(rec.ctx)
		if !ok || in.Kind != taskauthz.KindSystem || in.Roles != nil {
			t.Errorf("%s: the task manager saw %+v, %v; want only the system principal", name, in, ok)
		}
	}
}

func TestSystemCompleter_Command(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    any
	}{
		// The payment service settles a payment with no command.
		{"none named", map[string]any{"payment_status": "success"}, systemCommand},
		{"empty", map[string]any{orchestrator.CommandVar: ""}, systemCommand},
		{"nil payload", nil, systemCommand},
		// The webhook handlers name theirs.
		{"named", map[string]any{orchestrator.CommandVar: "approve"}, "approve"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var before map[string]any
			if tc.payload != nil {
				before = make(map[string]any, len(tc.payload))
				for k, v := range tc.payload {
					before[k] = v
				}
			}

			rec := &recordingCompleter{}
			if err := (systemCompleter{tm: rec}).CompleteTaskStepByToken(context.Background(), "tok", tc.payload); err != nil {
				t.Fatal(err)
			}
			if got := rec.payload[orchestrator.CommandVar]; got != tc.want {
				t.Errorf("command = %v, want %v", got, tc.want)
			}
			if rec.payload["payment_status"] != tc.payload["payment_status"] {
				t.Errorf("payload fields were not passed on: %v", rec.payload)
			}
			// The caller's map is theirs: it must come back as they passed it.
			if len(tc.payload) != len(before) {
				t.Errorf("caller's payload was changed: %v, want %v", tc.payload, before)
			}
			for k, v := range before {
				if tc.payload[k] != v {
					t.Errorf("caller's payload[%q] = %v, want %v", k, tc.payload[k], v)
				}
			}
		})
	}
}

// authzCompleter stands in for the task manager's PRE_RESUME phase: it runs the
// real authz extension on one step, as core does before a step resumes.
type authzCompleter struct {
	state string
	rules map[string]map[string][]string
}

func (a authzCompleter) run(ctx context.Context, payload map[string]any) error {
	props, err := json.Marshal(a.rules)
	if err != nil {
		return err
	}
	ext := taskauthzext.NewExtension(taskauthz.Catalog{
		Roles:   map[string]string{"trader": "Trader", "cha": "CHA"},
		Clients: map[string]string{"sltb": "SLTB_TO_NSW"},
	})
	return ext.Execute(ctx, &store.TaskRecord{TaskID: "t1", CaseID: "c1", State: a.state}, payload, props)
}

func (a authzCompleter) CompleteTaskStep(ctx context.Context, _, _ string, payload map[string]any) error {
	return a.run(ctx, payload)
}

func (a authzCompleter) CompleteTaskStepByToken(ctx context.Context, _ string, payload map[string]any) error {
	return a.run(ctx, payload)
}

// A step only this service may complete (a webhook wait, a payment) accepts its
// in-process callers through the adapter, and nobody without it.
func TestSystemCompleter_PassesSystemOnlyRules(t *testing.T) {
	wait := authzCompleter{state: "QUEUED_EXTERNALLY", rules: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"submit": {taskauthz.SystemPrincipal}}}}
	payment := authzCompleter{state: "PENDING_PAYMENT", rules: map[string]map[string][]string{"PENDING_PAYMENT": {"submit": {taskauthz.SystemPrincipal}}}}

	// A webhook resumes a wait with the command it names.
	webhook := map[string]any{orchestrator.CommandVar: "submit", "release_status": "RELEASED"}
	if err := (systemCompleter{tm: wait}).CompleteTaskStep(context.Background(), "t1", "s1", webhook); err != nil {
		t.Errorf("webhook through the adapter: %v", err)
	}
	if err := wait.CompleteTaskStep(context.Background(), "t1", "s1", webhook); !errors.Is(err, taskauthzext.ErrUnauthenticated) {
		t.Errorf("webhook without the adapter: err = %v, want ErrUnauthenticated", err)
	}

	// The payment service settles a payment with no command at all.
	settled := map[string]any{"payment_status": "success", "reference_number": "R1", "amount": 100}
	if err := (systemCompleter{tm: payment}).CompleteTaskStepByToken(context.Background(), "tok", settled); err != nil {
		t.Errorf("payment through the adapter: %v", err)
	}
	if err := payment.CompleteTaskStepByToken(context.Background(), "tok", settled); err == nil {
		t.Error("payment without the adapter: allowed, want it refused")
	}

	// A user with every role still can't, and neither can a client.
	owner := taskauthz.WithInput(context.Background(), taskauthz.Input{
		Kind: taskauthz.KindUser, Roles: []string{"Trader", "CHA"},
		OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{"trader": true, "cha": true}, nil
		},
	})
	forged := map[string]any{orchestrator.CommandVar: "submit", "payment_status": "success"}
	if err := payment.CompleteTaskStep(owner, "t1", "s1", forged); !errors.Is(err, taskauthzext.ErrForbidden) {
		t.Errorf("trader on a payment step: err = %v, want ErrForbidden", err)
	}
	client := taskauthz.WithInput(context.Background(), taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "SLTB_TO_NSW"})
	if err := wait.CompleteTaskStep(client, "t1", "s1", forged); !errors.Is(err, taskauthzext.ErrForbidden) {
		t.Errorf("client on a wait step: err = %v, want ErrForbidden", err)
	}
}

// The adapter adds an identity; it doesn't widen a rule. A step for users
// refuses the system principal like anyone else.
func TestSystemCompleter_RefusedByUserRules(t *testing.T) {
	userStep := authzCompleter{state: "PENDING_USER", rules: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader", "cha"}}}}
	err := (systemCompleter{tm: userStep}).CompleteTaskStep(context.Background(), "t1", "s1", map[string]any{orchestrator.CommandVar: "submit"})
	if !errors.Is(err, taskauthzext.ErrForbidden) {
		t.Errorf("system on a user step: err = %v, want ErrForbidden", err)
	}
}
