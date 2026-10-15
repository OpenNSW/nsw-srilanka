package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact/adapter/types"
	"github.com/OpenNSW/core/taskflow/store"

	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

func TestStepCanAct(t *testing.T) {
	traderRule := json.RawMessage(`{"PENDING_USER":{"submit":["trader"]}}`)
	chaRule := json.RawMessage(`{"PENDING_USER":{"submit":["cha"]}}`)
	authz := func(phase types.ExecutionPhase, props json.RawMessage) types.ExtensionConfig {
		return types.ExtensionConfig{ID: taskauthzext.ExtAuthz, Phase: phase, Properties: props}
	}
	templates := map[string]types.StepTemplate{
		"traders":     {Extensions: []types.ExtensionConfig{authz(types.PhasePreResume, traderRule)}},
		"traders-cha": {Extensions: []types.ExtensionConfig{authz(types.PhasePreResume, traderRule), authz(types.PhasePreResume, chaRule)}},
		"post-only":   {Extensions: []types.ExtensionConfig{authz(types.PhasePostResume, traderRule)}},
		"other-ext":   {Extensions: []types.ExtensionConfig{{ID: "audit", Phase: types.PhasePreResume, Properties: traderRule}}},
		"none":        {},
	}
	load := func(_ context.Context, id string) (types.StepTemplate, error) {
		tpl, ok := templates[id]
		if !ok {
			return types.StepTemplate{}, errors.New("not found")
		}
		return tpl, nil
	}
	canAct := stepCanAct(load, taskauthzext.NewExtension(taskauthz.Catalog{Roles: map[string]string{"trader": "Trader", "cha": "CHA"}}))
	trader := taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}, OwnedRoles: func(context.Context, string) (map[string]bool, error) {
		return map[string]bool{"trader": true}, nil
	}}

	for template, want := range map[string]bool{
		"traders":     true,
		"traders-cha": false, // every authz extension must allow it, as core runs them all
		// A step whose submissions no authz extension checks.
		"post-only": true,
		"other-ext": true,
		"none":      true,
		"":          false, // no active step template
	} {
		got, err := canAct(context.Background(), store.TaskRecord{State: "PENDING_USER", ActiveTaskTemplateID: template}, trader)
		if err != nil || got != want {
			t.Errorf("template %q: canAct = %v, %v; want %v", template, got, err, want)
		}
	}
	if _, err := canAct(context.Background(), store.TaskRecord{State: "PENDING_USER", ActiveTaskTemplateID: "missing"}, trader); err == nil {
		t.Error("a template that doesn't load: canAct succeeded")
	}
}
