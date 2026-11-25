package plugins

import (
	"testing"

	"github.com/OpenNSW/core/remote"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/platform/tasktypes"
)

func TestRegister_InstallsTheSriLankanTaskTypes(t *testing.T) {
	reg := flowplugins.NewRegistry()

	require.NoError(t, Register(reg, remote.NewManager(), nil, CustomsSubmissions{}))

	for _, taskType := range []string{
		TaskTypeCustomsCusdecDispatch,
		TaskTypeCustomsCusdecVerify,
		TaskTypeCustomsCusdecRetrieve,
		TaskTypeCustomsCDNDispatch,
		TaskTypeCustomsBoatNotePrefill,
		TaskTypeCustomsCaseDeclaration,
		TaskTypeCustomsCDNCancel,
		TaskTypeSLPAECDNUpload,
		TaskTypeSLPAServiceOrder,
		TaskTypeSLPASundryOrder,
		TaskTypeSLPASundryServices,
		TaskTypeSLPAInvoiceGenerate,
		TaskTypeSLPAConsolidationFetch,
		TaskTypeSLPAConsolidationSave,
		TaskTypeSLPAConsolidationDelete,
		TaskTypeSLPAGatePass,
		TaskTypeNPQSEphytoHub,
	} {
		_, ok := reg.Get(taskType)
		assert.True(t, ok, "%s is registered", taskType)
	}
	// The generic task types are bootstrap's to install.
	_, ok := reg.Get(tasktypes.TaskTypeAuthAPICall)
	assert.False(t, ok)
}

func TestRegister_RejectsMissingDependencies(t *testing.T) {
	err := Register(nil, remote.NewManager(), nil, CustomsSubmissions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry is nil")

	err = Register(flowplugins.NewRegistry(), nil, nil, CustomsSubmissions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote manager is nil")
}

func TestRegister_FailsWhenATaskTypeIsTaken(t *testing.T) {
	reg := flowplugins.NewRegistry()
	require.NoError(t, reg.Register(TaskTypeSLPAGatePass, flowplugins.NewUserInputPlugin()))

	err := Register(reg, remote.NewManager(), nil, CustomsSubmissions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "register SLPA_GATE_PASS")
}
