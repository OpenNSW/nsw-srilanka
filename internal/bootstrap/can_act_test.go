package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/core/platform/taskauthz"
	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
)

// stepCanAct loads the active step's template and leaves the decision to the
// authz package (see its MayActOnStep tests).
func TestStepCanAct(t *testing.T) {
	// Version 2 moves the step to CHA only; a task keeps its own version's rules.
	reg, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"traders.json": []byte(`{"id": "traders", "task_type": "USER_INPUT", "extensions": [
			{"id": "authz", "phase": "PRE_RESUME", "properties": {"PENDING_USER": {"submit": ["trader"]}}}
		]}`),
		"no_rule.json": []byte(`{"id": "no_rule", "task_type": "USER_INPUT"}`),
		"traders_v2.json": []byte(`{"id": "traders", "task_type": "USER_INPUT", "extensions": [
			{"id": "authz", "phase": "PRE_RESUME", "properties": {"PENDING_USER": {"submit": ["cha"]}}}
		]}`),
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"traders","kind":"subtask_template","path":"traders.json"},{"id":"no_rule","kind":"subtask_template","path":"no_rule.json"}]}`),
		"manifest_v2.json": []byte(`{"artifacts":[{"id":"traders","kind":"subtask_template","path":"traders_v2.json"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	canAct := stepCanAct(reg, taskauthzext.NewExtension(taskauthz.Catalog{Roles: map[string]string{"trader": "Trader", "cha": "CHA"}}))
	input := func(role, owned string) taskauthz.Input {
		return taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{role}, OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{owned: true}, nil
		}}
	}
	record := func(template string) store.TaskRecord {
		return store.TaskRecord{ManifestVersion: 1, State: "PENDING_USER", ActiveTaskTemplateID: template}
	}

	for name, tc := range map[string]struct {
		record  store.TaskRecord
		in      taskauthz.Input
		want    bool
		wantErr error
	}{
		"the step's rules allow the caller":  {record: record("traders"), in: input("Trader", "trader"), want: true},
		"the step's rules refuse the caller": {record: record("traders"), in: input("CHA", "cha")},
		"a step with no rule":                {record: record("no_rule"), in: input("Trader", "trader"), wantErr: orchestrator.ErrStepNotGuarded},
		// Nothing is loaded: the registry has no template with an empty ID.
		"no active step template": {record: record(""), in: input("Trader", "trader")},
	} {
		got, err := canAct(context.Background(), tc.record, tc.in)
		if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) || got != tc.want {
			t.Errorf("%s: canAct = %v, %v; want %v, %v", name, got, err, tc.want, tc.wantErr)
		}
	}
	onV2 := record("traders")
	onV2.ManifestVersion = 2
	if got, err := canAct(context.Background(), onV2, input("CHA", "cha")); err != nil || !got {
		t.Errorf("a task on version 2: canAct = %v, %v; want true", got, err)
	}
	if _, err := canAct(context.Background(), record("missing"), input("Trader", "trader")); !errors.Is(err, artifact.ErrNotFound) {
		t.Errorf("a template that doesn't load: err = %v, want ErrNotFound", err)
	}
}
