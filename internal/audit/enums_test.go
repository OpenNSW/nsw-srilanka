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

// TestEnumsSync asserts that the Go constants declared in enums.go match the enum values
// defined in our configs/argus/config.yaml. This ensures the codebase remains synchronized
// with the Argus deployment configuration we define.
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
	assertContains(t, cfg.Enums.EventTypes, string(EventConsignment), "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, string(EventTask), "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, string(EventStorage), "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, string(EventPayment), "eventTypes")
	assertContains(t, cfg.Enums.EventTypes, string(EventUserMgmt), "eventTypes")

	// Assert EventActions
	assertContains(t, cfg.Enums.EventActions, string(ActionCreate), "eventActions")
	assertContains(t, cfg.Enums.EventActions, string(ActionRead), "eventActions")
	assertContains(t, cfg.Enums.EventActions, string(ActionUpdate), "eventActions")
	assertContains(t, cfg.Enums.EventActions, string(ActionDelete), "eventActions")

	// Assert ActorTypes
	assertContains(t, cfg.Enums.ActorTypes, string(ActorAdmin), "actorTypes")
	assertContains(t, cfg.Enums.ActorTypes, string(ActorMember), "actorTypes")
	assertContains(t, cfg.Enums.ActorTypes, string(ActorService), "actorTypes")
	assertContains(t, cfg.Enums.ActorTypes, string(ActorSystem), "actorTypes")

	// Assert TargetTypes
	assertContains(t, cfg.Enums.TargetTypes, string(TargetConsignment), "targetTypes")
	assertContains(t, cfg.Enums.TargetTypes, string(TargetTask), "targetTypes")
	assertContains(t, cfg.Enums.TargetTypes, string(TargetStorage), "targetTypes")
	assertContains(t, cfg.Enums.TargetTypes, string(TargetPayment), "targetTypes")
}
