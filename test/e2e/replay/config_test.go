package replay_e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ActorIdentity holds the IdP credentials used to mint/validate tokens for any
// principal — both MEMBER users and SERVICE (M2M) agency clients share this shape.
type ActorIdentity struct {
	ClientID string   `json:"clientID"`
	Roles    []string `json:"roles"`
	Scopes   []string `json:"scopes"`
}

// MemberConfig describes a MEMBER actor (Trader, CHA, …). Members use the
// authorization_code grant; their token carries a user sub + ou claims.
// Add a new member type by dropping a configs/members/<id>.json file.
type MemberConfig struct {
	ID       string        `json:"id"`
	OUHandle string        `json:"ouHandle"`
	Identity ActorIdentity `json:"identity"`
}

// AgencyConfig describes a SERVICE/M2M agency actor. Agencies use the
// client_credentials grant and have an inbound inject protocol and an outbound
// callback protocol. Add a new agency by dropping a configs/agencies/<id>.json file.
type AgencyConfig struct {
	ID       string         `json:"id"`
	Identity ActorIdentity  `json:"identity"`
	Inbound  AgencyInbound  `json:"inbound"`
	Outbound AgencyOutbound `json:"outbound"`
}

// AgencyInbound describes the endpoint the mock SERVER exposes to receive injects
// from the NSW app.
type AgencyInbound struct {
	Endpoint    string `json:"endpoint"`    // Go mux pattern, e.g. "POST /api/v1/inject"
	TaskIDField string `json:"taskIDField"` // JSON field carrying the task id, e.g. "taskId"
}

// AgencyOutbound describes how the mock CLIENT calls back to the NSW app.
type AgencyOutbound struct {
	CallbackPath string `json:"callbackPath"` // URL path template, e.g. "/api/v1/tasks/{taskId}"
	CommandField string `json:"commandField"` // payload field for the command string
	PayloadField string `json:"payloadField"` // payload field for the content map
}

// PaymentConfig describes a payment gateway used in E2E flows.
// Add a new payment gateway by dropping a configs/payments/<id>.json file.
//
// identity is optional: current production gateways (e.g. GovPay) post unauthenticated
// webhooks. TODO: when the gateway webhook is made protected, enforce non-nil identity.
type PaymentConfig struct {
	ID          string         `json:"id"`
	WebhookPath string         `json:"webhookPath"`
	Identity    *ActorIdentity `json:"identity,omitempty"`
}

// loadMemberConfigs reads every *.json file under test/e2e/replay/configs/members/
// and asserts that each entry has a non-empty ouHandle (mandatory field).
func loadMemberConfigs(t *testing.T) []MemberConfig {
	t.Helper()
	configs := loadConfigs[MemberConfig](t, "members")
	seen := make(map[string]bool, len(configs))
	for _, cfg := range configs {
		if seen[cfg.ID] {
			t.Fatalf("member config: duplicate id %q", cfg.ID)
		}
		seen[cfg.ID] = true
		if cfg.OUHandle == "" {
			t.Fatalf("member config %q is missing required ouHandle field", cfg.ID)
		}
	}
	return configs
}

// loadAgencyConfigs reads every *.json file under test/e2e/replay/configs/agencies/
// and asserts that each entry has all mandatory fields populated.
func loadAgencyConfigs(t *testing.T) []AgencyConfig {
	t.Helper()
	configs := loadConfigs[AgencyConfig](t, "agencies")
	seen := make(map[string]bool, len(configs))
	for _, cfg := range configs {
		if seen[cfg.ID] {
			t.Fatalf("agency config: duplicate id %q", cfg.ID)
		}
		seen[cfg.ID] = true
		if cfg.Identity.ClientID == "" {
			t.Fatalf("agency config %q is missing required identity.clientID field", cfg.ID)
		}
		if cfg.Inbound.Endpoint == "" {
			t.Fatalf("agency config %q is missing required inbound.endpoint field", cfg.ID)
		}
		if cfg.Inbound.TaskIDField == "" {
			t.Fatalf("agency config %q is missing required inbound.taskIDField field", cfg.ID)
		}
		if cfg.Outbound.CallbackPath == "" {
			t.Fatalf("agency config %q is missing required outbound.callbackPath field", cfg.ID)
		}
		if cfg.Outbound.CommandField == "" {
			t.Fatalf("agency config %q is missing required outbound.commandField field", cfg.ID)
		}
		if cfg.Outbound.PayloadField == "" {
			t.Fatalf("agency config %q is missing required outbound.payloadField field", cfg.ID)
		}
	}
	return configs
}

// loadPaymentConfigs reads every *.json file under test/e2e/replay/configs/payments/
// and asserts each entry has a non-empty webhookPath (mandatory field).
func loadPaymentConfigs(t *testing.T) []PaymentConfig {
	t.Helper()
	configs := loadConfigs[PaymentConfig](t, "payments")
	seen := make(map[string]bool, len(configs))
	for _, cfg := range configs {
		if seen[cfg.ID] {
			t.Fatalf("payment config: duplicate id %q", cfg.ID)
		}
		seen[cfg.ID] = true
		if cfg.WebhookPath == "" {
			t.Fatalf("payment config %q is missing required webhookPath field", cfg.ID)
		}
		if !strings.HasPrefix(cfg.WebhookPath, "/") {
			t.Fatalf("payment config %q: webhookPath %q must start with '/'", cfg.ID, cfg.WebhookPath)
		}
	}
	return configs
}

func loadConfigs[T any](t *testing.T, subdir string) []T {
	t.Helper()
	dir := filepath.Join(findRepoRoot(), "test", "e2e", "replay", "configs", subdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("loadConfigs: read dir %s: %v", dir, err)
	}
	var configs []T
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("loadConfigs: read %s: %v", e.Name(), err)
		}
		var cfg T
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("loadConfigs: parse %s: %v", e.Name(), err)
		}
		configs = append(configs, cfg)
	}
	return configs
}
