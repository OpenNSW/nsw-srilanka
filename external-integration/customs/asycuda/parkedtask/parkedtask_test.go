package parkedtask

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
)

type tasksOfCase map[string][]store.TaskRecord

func (t tasksOfCase) GetAllTasks(_ context.Context, caseID string) []store.TaskRecord {
	return t[caseID]
}

func TestFind(t *testing.T) {
	tasks := tasksOfCase{"case-1": {
		{TaskID: "done", ActiveStepID: "s0", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "wait", State: "COMPLETED"},
		{TaskID: "other-wf", ActiveStepID: "s1", ParentWorkflowID: "wf-2", ActiveTaskTemplateID: "wait", State: StateQueuedExternally},
		{TaskID: "other-template", ActiveStepID: "s2", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "review", State: StateQueuedExternally},
		{TaskID: "parked", ActiveStepID: "s3", ParentWorkflowID: "wf-1", ActiveTaskTemplateID: "wait", State: StateQueuedExternally},
	}}

	got, ok := Find(context.Background(), tasks, "case-1", "wf-1", "wait")
	assert.True(t, ok)
	assert.Equal(t, Task{TaskID: "parked", StepID: "s3"}, got)

	for name, args := range map[string][3]string{
		"another case":         {"case-2", "wf-1", "wait"},
		"no workflow recorded": {"case-1", "", "wait"},
		"no case recorded":     {"", "wf-1", "wait"},
		"nothing parked on it": {"case-1", "wf-1", "ack"},
	} {
		_, ok := Find(context.Background(), tasks, args[0], args[1], args[2])
		assert.False(t, ok, name)
	}
}
