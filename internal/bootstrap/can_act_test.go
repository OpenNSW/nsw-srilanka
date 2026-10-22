package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/store"

	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

// stepCanAct loads the active step's template and leaves the decision to the
// authz package (see its MayActOnStep tests).
func TestStepCanAct(t *testing.T) {
	reg := artifact.NewRegistry(testutil.MemLoader{
		"traders.json": []byte(`{"id": "traders", "task_type": "USER_INPUT", "extensions": [
			{"id": "authz", "phase": "PRE_RESUME", "properties": {"PENDING_USER": {"submit": ["trader"]}}}
		]}`),
		"no_rule.json": []byte(`{"id": "no_rule", "task_type": "USER_INPUT"}`),
	})
	reg.RegisterArtifact("traders", "subtask_template", "", "traders.json")
	reg.RegisterArtifact("no_rule", "subtask_template", "", "no_rule.json")
	canAct := stepCanAct(reg, taskauthzext.NewExtension(taskauthz.Catalog{Roles: map[string]string{"trader": "Trader", "cha": "CHA"}}))
	input := func(role, owned string) taskauthz.Input {
		return taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{role}, OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{owned: true}, nil
		}}
	}
	record := func(template string) store.TaskRecord {
		return store.TaskRecord{State: "PENDING_USER", ActiveTaskTemplateID: template}
	}

	for name, tc := range map[string]struct {
		record store.TaskRecord
		in     taskauthz.Input
		want   bool
	}{
		"the step's rules allow the caller":  {record("traders"), input("Trader", "trader"), true},
		"the step's rules refuse the caller": {record("traders"), input("CHA", "cha"), false},
		"a step with no rule":                {record("no_rule"), input("Trader", "trader"), false},
		// Nothing is loaded: the registry has no template with an empty ID.
		"no active step template": {record(""), input("Trader", "trader"), false},
	} {
		got, err := canAct(context.Background(), tc.record, tc.in)
		if err != nil || got != tc.want {
			t.Errorf("%s: canAct = %v, %v; want %v", name, got, err, tc.want)
		}
	}
	if _, err := canAct(context.Background(), record("missing"), input("Trader", "trader")); !errors.Is(err, artifact.ErrNotFound) {
		t.Errorf("a template that doesn't load: err = %v, want ErrNotFound", err)
	}
}
