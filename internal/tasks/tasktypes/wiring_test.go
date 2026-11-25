package tasktypes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister_InstallsTheGenericTaskTypes(t *testing.T) {
	reg := flowplugins.NewRegistry()

	require.NoError(t, Register(reg, remote.NewManager(), &fakePaymentService{}, OGAFiles{}, noCaseAttributes{}))

	for _, taskType := range []string{
		TaskTypeUserInput,
		TaskTypeValueWriter,
		TaskTypeExternalReview,
		TaskTypePayment,
		TaskTypeAPICall,
		TaskTypeAuthAPICall,
	} {
		_, ok := reg.Get(taskType)
		assert.True(t, ok, "%s is registered", taskType)
	}
}

func TestRegister_RejectsMissingDependencies(t *testing.T) {
	issue := func(string, string) (string, error) { return "", nil }
	clientFor := func(string) (string, bool) { return "", false }
	renderConfig := func(context.Context, store.TaskRecord) (json.RawMessage, error) { return nil, nil }

	tests := []struct {
		name     string
		reg      *flowplugins.Registry
		mgr      *remote.Manager
		noPay    bool
		files    OGAFiles
		noCases  bool
		contains string
	}{
		{name: "nil registry", mgr: remote.NewManager(), contains: "registry is nil"},
		{name: "nil remote manager", reg: flowplugins.NewRegistry(), contains: "remote manager is nil"},
		{name: "nil payment service", reg: flowplugins.NewRegistry(), mgr: remote.NewManager(), noPay: true, contains: "payment service is nil"},
		{name: "nil case attributes", reg: flowplugins.NewRegistry(), mgr: remote.NewManager(), noCases: true, contains: "case attributes reader is nil"},
		{name: "issuer without client lookup", reg: flowplugins.NewRegistry(), mgr: remote.NewManager(), files: OGAFiles{IssueForClient: issue, RenderConfigOf: renderConfig}, contains: "both a client lookup and an issuer"},
		{name: "issuer without render config", reg: flowplugins.NewRegistry(), mgr: remote.NewManager(), files: OGAFiles{ClientFor: clientFor, IssueForClient: issue}, contains: "render config resolver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pay := &fakePaymentService{}
			var cases CaseAttributes = noCaseAttributes{}
			var err error
			switch {
			case tt.noPay:
				err = Register(tt.reg, tt.mgr, nil, tt.files, cases)
			case tt.noCases:
				err = Register(tt.reg, tt.mgr, pay, tt.files, nil)
			default:
				err = Register(tt.reg, tt.mgr, pay, tt.files, cases)
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

func TestRegister_FailsWhenATaskTypeIsTaken(t *testing.T) {
	reg := flowplugins.NewRegistry()
	require.NoError(t, reg.Register(TaskTypePayment, NewValueWriterPlugin()))

	err := Register(reg, remote.NewManager(), &fakePaymentService{}, OGAFiles{}, noCaseAttributes{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "register PAYMENT")
}
