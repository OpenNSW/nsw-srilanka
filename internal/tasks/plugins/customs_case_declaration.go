package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
)

// TaskTypeCustomsCaseDeclaration finds the declaration ASYCUDA registered for
// the task's case, for a flow started on a consignment after its customs flow
// has run, such as raising a dispatch note for a container whose note Customs
// cancelled. Such a flow starts with only the consignment's id, so it reads the
// declaration's reference from what the customs flow recorded.
const TaskTypeCustomsCaseDeclaration = "CUSTOMS_CASE_DECLARATION"

// CaseDeclarations finds a case's registered declaration. cusdec's
// DeclarationRepository satisfies it.
type CaseDeclarations interface {
	GetRegisteredByCase(ctx context.Context, caseID string) (*cusdec.CusdecDeclaration, error)
}

// CaseDeclarationPlugin records, under its output namespace, whether the case
// has a registered declaration (found) and, when it has, its reference in the
// office/year/serial/number form the customs steps take (cusdec_ref).
type CaseDeclarationPlugin struct {
	declarations CaseDeclarations
}

// NewCaseDeclarationPlugin returns the plugin, reading declarations from
// declarations.
func NewCaseDeclarationPlugin(declarations CaseDeclarations) *CaseDeclarationPlugin {
	return &CaseDeclarationPlugin{declarations: declarations}
}

// Execute looks the declaration up by the task's case. A case with none is
// recorded as not found, for the flow to tell the trader; only a lookup that
// fails is an error.
func (p *CaseDeclarationPlugin) Execute(ctx pluginContext, _ json.RawMessage) error {
	if ctx.Record == nil {
		return fmt.Errorf("customs_case_declaration: task record is nil")
	}
	if ctx.OutputNamespace == "" {
		return fmt.Errorf("customs_case_declaration: an output namespace is required")
	}
	decl, err := p.declarations.GetRegisteredByCase(ctx.Context, ctx.Record.CaseID)
	if err != nil {
		return fmt.Errorf("customs_case_declaration: find the declaration of case %s: %w", ctx.Record.CaseID, err)
	}

	out := map[string]any{"found": decl != nil}
	if decl != nil {
		ref := document.Reference{Office: decl.CusdecOffice, Year: decl.CusdecYear, Serial: decl.CusdecSerial, Number: decl.CusdecNumber}
		out["cusdec_ref"] = ref.String()
	}
	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = out
	slog.Info("customs_case_declaration: looked up the case's declaration",
		"taskId", ctx.Record.TaskID, "caseId", ctx.Record.CaseID, "found", decl != nil)
	return nil
}
