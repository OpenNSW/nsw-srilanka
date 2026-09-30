package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenNSW/core/refid"
)

// TaskTypeRefIDGenerator generates a reference ID from one of the refid formats
// configured in config.yaml.
const TaskTypeRefIDGenerator = "REFID_GENERATOR"

// refIDKey is the key under the template's output_namespace that holds the
// generated ID, e.g. "refid.reference_id" in the node's output_mapping.
const refIDKey = "reference_id"

// RefIDGeneratorPlugin is a synchronous plugin that generates a reference ID
// for the (issuer, id_type) named in the subtask template's plugin_properties:
//
//	{"task_type": "REFID_GENERATOR", "output_namespace": "refid",
//	 "plugin_properties": {"issuer": "TNSW", "id_type": "consignment_ref"}}
//
// Every string-valued input is passed to the format as a param, so the node's
// input_mapping names its inputs after the params the format expects (a list
// segment's param, or a scope-key placeholder); refid ignores the rest.
//
// The ID is written to the task's own state at <output_namespace>.reference_id
// and goes no further on its own: the workflow reads it from there through the
// node's output_mapping, as it does any other plugin's output.
type RefIDGeneratorPlugin struct {
	refIDs refid.Registry
}

// NewRefIDGeneratorPlugin builds the plugin. refIDs must be non-nil; a
// deployment with no refid section passes a registry whose Generate always
// fails, so a template using this plugin fails loudly rather than silently.
func NewRefIDGeneratorPlugin(refIDs refid.Registry) *RefIDGeneratorPlugin {
	if refIDs == nil {
		panic("refIDs is nil")
	}
	return &RefIDGeneratorPlugin{refIDs: refIDs}
}

type refIDGeneratorConfig struct {
	Issuer string `json:"issuer"`
	IDType string `json:"id_type"`
}

// Execute generates the ID and returns nil (not ErrSuspended), so the engine
// advances immediately.
func (p *RefIDGeneratorPlugin) Execute(ctx pluginContext, configRaw json.RawMessage) error {
	var cfg refIDGeneratorConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return fmt.Errorf("refid_generator: invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.Issuer) == "" {
		return errors.New("refid_generator: plugin_properties.issuer is required")
	}
	if strings.TrimSpace(cfg.IDType) == "" {
		return errors.New("refid_generator: plugin_properties.id_type is required")
	}
	if ctx.OutputNamespace == "" {
		return errors.New("refid_generator: output_namespace is required to hold the generated ID")
	}
	if ctx.Record == nil {
		return errors.New("refid_generator: task record is nil")
	}

	// StartSubTask runs as a Temporal activity, so it can run again after the
	// record holding this ID was saved. Generating again would take another
	// sequence number and hand the workflow a different ID.
	if existing, ok := ctx.Record.Data[ctx.OutputNamespace].(map[string]any); ok {
		if id, ok := existing[refIDKey].(string); ok && id != "" {
			slog.Info("refid_generator: reference ID already generated for this task; reusing it",
				"taskId", ctx.Record.TaskID, "issuer", cfg.Issuer, "idType", cfg.IDType)
			return nil
		}
	}

	params := make(map[string]string, len(ctx.Inputs))
	for k, v := range ctx.Inputs {
		if s, ok := v.(string); ok {
			params[k] = s
		}
	}

	id, err := p.refIDs.Generate(ctx.Context, cfg.Issuer, cfg.IDType, params)
	if err != nil {
		return fmt.Errorf("refid_generator: generate (%q, %q): %w", cfg.Issuer, cfg.IDType, err)
	}

	if ctx.Record.Data == nil {
		ctx.Record.Data = make(map[string]any)
	}
	ctx.Record.Data[ctx.OutputNamespace] = map[string]any{refIDKey: id}
	return nil
}
