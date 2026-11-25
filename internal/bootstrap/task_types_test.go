package bootstrap

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/tasktypes"
)

// stubPaymentService stands in for the payment service; registering never calls it.
type stubPaymentService struct{ payment.Service }

// stubCaseAttributes stands in for the case attributes reader; registering never calls it.
type stubCaseAttributes struct{}

func (stubCaseAttributes) CaseAttributes(context.Context, string) (map[string]any, error) {
	return nil, nil
}

func TestRegisterTaskTypes_InstallsTheGenericTaskTypes(t *testing.T) {
	reg := plugins.NewRegistry()

	require.NoError(t, registerTaskTypes(reg, remote.NewManager(), stubPaymentService{}, tasktypes.OGAFiles{}, stubCaseAttributes{}))

	for _, taskType := range []string{
		tasktypes.TaskTypeUserInput,
		tasktypes.TaskTypeValueWriter,
		tasktypes.TaskTypeExternalReview,
		tasktypes.TaskTypePayment,
		tasktypes.TaskTypeAPICall,
		tasktypes.TaskTypeAuthAPICall,
	} {
		_, ok := reg.Get(taskType)
		assert.True(t, ok, "%s is registered", taskType)
	}
}

func TestRegisterTaskTypes_FailsWhenATaskTypeIsTaken(t *testing.T) {
	reg := plugins.NewRegistry()
	require.NoError(t, reg.Register(tasktypes.TaskTypePayment, tasktypes.NewValueWriterPlugin()))

	err := registerTaskTypes(reg, remote.NewManager(), stubPaymentService{}, tasktypes.OGAFiles{}, stubCaseAttributes{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "register PAYMENT")
}
