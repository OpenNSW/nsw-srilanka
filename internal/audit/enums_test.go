package audit

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type enumsConfig struct {
	Enums struct {
		EventTypes   []string `yaml:"eventTypes"`
		EventActions []string `yaml:"eventActions"`
		ActorTypes   []string `yaml:"actorTypes"`
		TargetTypes  []string `yaml:"targetTypes"`
	} `yaml:"enums"`
}

func TestEnumsSync(t *testing.T) {
	// Look up the path to configs/argus/config.yaml relative to package dir
	path := filepath.Join("..", "..", "configs", "argus", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	var cfg enumsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config.yaml: %v", err)
	}

	// Helper to assert item is in slice
	assertContains := func(t *testing.T, list []string, item string, name string) {
		t.Helper()
		for _, x := range list {
			if x == item {
				return
			}
		}
		t.Errorf("Go constant %q is missing from configs/argus/config.yaml %s list", item, name)
	}

	// Assert EventTypes
	assertContains(t, cfg.Enums.EventTypes, EventConsignment, "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, EventTask, "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, EventStorage, "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, EventPayment, "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, EventUserMgmt, "eventTypes")

	// Assert EventActions
	assertContains(t, cfg.Enums.EventActions, ActionCreate, "eventActions")
	assertContains(t, cfg.Enums.EventActions, ActionRead, "eventActions")
	assertContains(t, cfg.Enums.EventActions, ActionUpdate, "eventActions")
	assertContains(t, cfg.Enums.EventActions, ActionDelete, "eventActions")

	// Assert ActorTypes
	assertContains(t, cfg.Enums.ActorTypes, ActorAdmin, "actorTypes")
	assertContains(t, cfg.Enums.ActorTypes, ActorMember, "actorTypes")
	assertContains(t, cfg.Enums.ActorTypes, ActorService, "actorTypes")
	assertContains(t, cfg.Enums.ActorTypes, ActorSystem, "actorTypes")

	// Assert TargetTypes
	assertContains(t, cfg.Enums.TargetTypes, TargetConsignment, "targetTypes")
	assertContains(t, cfg.Enums.TargetTypes, TargetTask, "targetTypes")
	assertContains(t, cfg.Enums.TargetTypes, TargetStorage, "targetTypes")
	assertContains(t, cfg.Enums.TargetTypes, TargetPayment, "targetTypes")
}
