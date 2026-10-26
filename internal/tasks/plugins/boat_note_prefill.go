package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/boatnote"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
)

// TaskTypeCustomsBoatNotePrefill prefills the boat note application from what
// Sri Lanka Customs holds for the consignment: the declaration, as the
// CUSTOMS_CUSDEC_RETRIEVE step before it captured it (§6.7), and each Cargo
// Dispatch Note registered against it, which this step retrieves (§7.8). It is
// synchronous and never fails the step: what cannot be retrieved is left for
// the trader to fill, with one container row per dispatch note the consignment
// registered, as before.
const TaskTypeCustomsBoatNotePrefill = "CUSTOMS_BOAT_NOTE_PREFILL"

// BoatNotePrefillPlugin runs the prefill against the configured SLC Edge
// service.
type BoatNotePrefillPlugin struct {
	post func(ctx context.Context, serviceID, path string, body any) (map[string]any, error)
}

// NewBoatNotePrefillPlugin returns the prefill plugin, calling SLC Edge through
// manager.
func NewBoatNotePrefillPlugin(manager *remote.Manager) *BoatNotePrefillPlugin {
	return &BoatNotePrefillPlugin{post: func(ctx context.Context, serviceID, path string, body any) (map[string]any, error) {
		var resp map[string]any
		err := manager.Call(ctx, serviceID, remote.Request{
			Method: http.MethodPost,
			Path:   path,
			Body:   remote.JSONBody{V: body},
		}, &resp)
		return resp, err
	}}
}

type boatNotePrefillConfig struct {
	ServiceID string `json:"service_id"`
}

// Execute builds the prefill from the task inputs — the retrieved declaration
// and its cdn_refs, the declaration form (for the declarant and exporter codes),
// and the containers the consignment registered — and records it under the
// output namespace as userform, with retrieved saying whether it came from the
// declaration.
func (p *BoatNotePrefillPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg boatNotePrefillConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("boat_note_prefill: invalid config: %w", err)
	}
	if cfg.ServiceID == "" {
		return fmt.Errorf("boat_note_prefill: service_id is required")
	}
	if ctx.Record == nil {
		return fmt.Errorf("boat_note_prefill: task record is nil")
	}
	if ctx.OutputNamespace == "" {
		return fmt.Errorf("boat_note_prefill: an output namespace is required")
	}

	declarant, company := document.TraderCodes(asMap(ctx.Inputs[cusdec.RetrieveFormKey]))
	result := boatnote.Prefill(ctx.Context, serviceCaller{post: p.post, serviceID: cfg.ServiceID}, boatnote.Input{
		Declaration:   asMap(ctx.Inputs[cusdec.RetrieveDeclarationKey]),
		CDNRefs:       rows(ctx.Inputs[cusdec.RetrieveCDNRefsKey]),
		DeclarantCode: declarant,
		CompanyCode:   company,
		Fallback:      rows(ctx.Inputs["containers"]),
	})

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{
		"userform":  result.Form,
		"retrieved": result.Retrieved,
	}
	slog.Info("boat_note_prefill: prefilled the boat note",
		"taskId", ctx.Record.TaskID, "retrieved", result.Retrieved, "containers", len(rows(result.Form["containers"])))
	return nil
}

type serviceCaller struct {
	post      func(ctx context.Context, serviceID, path string, body any) (map[string]any, error)
	serviceID string
}

func (c serviceCaller) Post(ctx context.Context, path string, body any) (map[string]any, error) {
	return c.post(ctx, c.serviceID, path, body)
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// rows reads a container rows array, as decoded JSON ([]any) or as built in Go.
func rows(v any) []map[string]any {
	switch list := v.(type) {
	case []map[string]any:
		return list
	case []any:
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}
