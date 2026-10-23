package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
)

// draftStore holds one task whose active step can take a draft. Only what SaveDraft
// reads and writes is implemented; any other call panics on the nil TaskStore.
type draftStore struct {
	store.TaskStore
	record  store.TaskRecord
	drafted bool
}

func (s *draftStore) GetTask(_ context.Context, taskID string) (store.TaskRecord, bool) {
	return s.record, taskID == s.record.TaskID
}

func (s *draftStore) WriteDraft(context.Context, string, string, int64, map[string]any) (int64, error) {
	s.drafted = true
	return 1, nil
}

// The task manager bootstrap builds refuses a step that declares no authz rule.
func TestTaskManagerOptions(t *testing.T) {
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"manifest_v1.json": []byte(`{"artifacts":[{"id":"no_rule","kind":"subtask_template","path":"no_rule.json"},{"id":"with_rule","kind":"subtask_template","path":"with_rule.json"}]}`),
		"no_rule.json": []byte(`{"id": "no_rule", "task_type": "USER_INPUT", "output_namespace": "userform",
			"draft": {"command": "save_as_draft"}}`),
		"with_rule.json": []byte(`{"id": "with_rule", "task_type": "USER_INPUT", "output_namespace": "userform",
			"draft": {"command": "save_as_draft"},
			"extensions": [{"id": "authz", "phase": "PRE_RESUME", "properties": {"PENDING_USER": {"save_as_draft": ["trader"]}}}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	draft := func(t *testing.T, template string, ext extensions.TaskExtension) (bool, error) {
		t.Helper()
		db := &draftStore{record: store.TaskRecord{TaskID: "t1", ActiveStepID: "s1", State: "PENDING_USER", ActiveTaskTemplateID: template, ManifestVersion: 1}}
		exts := extensions.NewRegistry()
		if ext != nil {
			if err := exts.Register("authz", ext); err != nil {
				t.Fatal(err)
			}
		}
		tm := orchestrator.NewTaskManager(db, set, plugins.NewRegistry(), exts, nil, nil, nil, taskManagerOptions()...)
		err := tm.SaveDraft(context.Background(), "t1", "s1", map[string]any{"name": "Al"})
		return db.drafted, err
	}

	t.Run("a step with no rule is refused", func(t *testing.T) {
		drafted, err := draft(t, "no_rule", nil)
		if !errors.Is(err, orchestrator.ErrStepNotGuarded) || drafted {
			t.Errorf("err = %v, drafted = %v; want ErrStepNotGuarded and nothing written", err, drafted)
		}
	})
	t.Run("a step with a rule runs it", func(t *testing.T) {
		allow := allowAll{}
		drafted, err := draft(t, "with_rule", &allow)
		if err != nil || !drafted || !allow.ran {
			t.Errorf("err = %v, drafted = %v, rule ran = %v; want the rule run and the draft saved", err, drafted, allow.ran)
		}
	})
}

// allowAll is an authz extension that allows every call and records that it ran.
type allowAll struct{ ran bool }

func (a *allowAll) Execute(context.Context, *store.TaskRecord, map[string]any, json.RawMessage) error {
	a.ran = true
	return nil
}
