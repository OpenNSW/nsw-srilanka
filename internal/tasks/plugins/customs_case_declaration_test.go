package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
)

type caseDeclarations struct {
	byCase map[string]*cusdec.CusdecDeclaration
	err    error
}

func (c caseDeclarations) GetRegisteredByCase(_ context.Context, caseID string) (*cusdec.CusdecDeclaration, error) {
	return c.byCase[caseID], c.err
}

func caseDeclarationCtx(caseID string) plugins.PluginContext {
	return plugins.PluginContext{
		Context:         context.Background(),
		Record:          &store.TaskRecord{TaskID: "task-1", CaseID: caseID},
		OutputNamespace: "declaration",
	}
}

// The case's registered declaration is recorded by its reference, in the form
// the customs steps take it.
func TestCaseDeclaration_RecordsTheCaseDeclaration(t *testing.T) {
	plugin := NewCaseDeclarationPlugin(caseDeclarations{byCase: map[string]*cusdec.CusdecDeclaration{
		"case-1": {CaseID: "case-1", CusdecOffice: "CBEX1", CusdecYear: "2026", CusdecSerial: "E", CusdecNumber: 1047},
	}})
	ctx := caseDeclarationCtx("case-1")

	require.NoError(t, plugin.Execute(ctx, nil))

	assert.Equal(t, map[string]any{"found": true, "cusdec_ref": "CBEX1/2026/E/1047"}, ctx.Record.Data["declaration"])
}

// A case without a registered declaration is recorded as not found, so the
// flow can say so, rather than failing the step.
func TestCaseDeclaration_RecordsACaseWithoutOne(t *testing.T) {
	ctx := caseDeclarationCtx("case-2")

	require.NoError(t, NewCaseDeclarationPlugin(caseDeclarations{}).Execute(ctx, nil))

	assert.Equal(t, map[string]any{"found": false}, ctx.Record.Data["declaration"])
}

func TestCaseDeclaration_ReportsAFailedLookup(t *testing.T) {
	ctx := caseDeclarationCtx("case-1")

	err := NewCaseDeclarationPlugin(caseDeclarations{err: errors.New("connection refused")}).Execute(ctx, nil)

	require.Error(t, err)
	assert.Nil(t, ctx.Record.Data)
}
