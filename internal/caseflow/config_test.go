package caseflow

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
)

var testCatalog = &catalog.Catalog{
	Roles:   map[string]string{"trader": "Trader", "cha": "CHA"},
	Clients: map[string]string{"tnsw": "NSW_TO_CDA", "fcau": "FCAU_TO_NSW"},
}

func TestConfigValidate(t *testing.T) {
	user := UserFlow{ID: "export", Label: "Export", Roles: []string{"trader"}}
	client := ClientFlow{ID: "notice", Clients: []string{"tnsw"}}
	for name, tc := range map[string]struct {
		cfg     Config
		wantErr string
	}{
		"none":  {Config{}, ""},
		"valid": {Config{Users: UserFlows{Enabled: true, Workflows: []UserFlow{user}}, Clients: ClientFlows{Enabled: true, Workflows: []ClientFlow{client}}}, ""},
		// A list that is off is still checked, so a mistake shows before it is turned on.
		"off but wrong":            {Config{Users: UserFlows{Workflows: []UserFlow{{ID: "export", Label: "Export"}}}}, "roles is required"},
		"user without id":          {Config{Users: UserFlows{Workflows: []UserFlow{{Label: "Export", Roles: []string{"trader"}}}}}, "id is required"},
		"user without label":       {Config{Users: UserFlows{Workflows: []UserFlow{{ID: "export", Roles: []string{"trader"}}}}}, "label is required"},
		"user role not in catalog": {Config{Users: UserFlows{Workflows: []UserFlow{{ID: "export", Label: "Export", Roles: []string{"officer"}}}}}, `roles names "officer"`},
		"user listed twice":        {Config{Users: UserFlows{Workflows: []UserFlow{user, user}}}, "listed twice"},
		"client without clients":   {Config{Clients: ClientFlows{Workflows: []ClientFlow{{ID: "notice"}}}}, "clients is required"},
		"client not in catalog":    {Config{Clients: ClientFlows{Workflows: []ClientFlow{{ID: "notice", Clients: []string{"cda"}}}}}, `clients names "cda"`},
		"client listed twice":      {Config{Clients: ClientFlows{Workflows: []ClientFlow{client, client}}}, "listed twice"},
		// The two lists are apart: a workflow may be in both.
		"in both lists": {Config{Users: UserFlows{Workflows: []UserFlow{{ID: "notice", Label: "Notice", Roles: []string{"cha"}}}}, Clients: ClientFlows{Workflows: []ClientFlow{client}}}, ""},
	} {
		err := tc.cfg.Validate(testCatalog)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: err = %v, want none", name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: err = %v, want one containing %q", name, err, tc.wantErr)
		}
	}
}

// Every listed workflow, in a list that is on or off, must be in the current manifest.
func TestConfigCheckWorkflows(t *testing.T) {
	ctx := context.Background()
	ok := Config{
		Users:   UserFlows{Enabled: true, Workflows: []UserFlow{{ID: "export"}}},
		Clients: ClientFlows{Workflows: []ClientFlow{{ID: "notice"}}},
	}
	if err := ok.CheckWorkflows(ctx, newArtifacts()); err != nil {
		t.Errorf("listed workflows in the manifest: err = %v", err)
	}
	for name, cfg := range map[string]Config{
		"user flow not a workflow":   {Users: UserFlows{Workflows: []UserFlow{{ID: "nope"}}}},
		"client flow not a workflow": {Clients: ClientFlows{Workflows: []ClientFlow{{ID: "nope"}}}},
		// legacy is only in manifest v1; v2 is current.
		"only in an older manifest": {Users: UserFlows{Workflows: []UserFlow{{ID: "legacy"}}}},
	} {
		if err := cfg.CheckWorkflows(ctx, newArtifacts()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestUserFlows(t *testing.T) {
	flows := UserFlows{Enabled: true, Workflows: []UserFlow{
		{ID: "both", Roles: []string{"trader", "cha"}},
		{ID: "cha", Roles: []string{"cha"}},
	}}
	ids := func(fs []UserFlow) []string {
		var out []string
		for _, f := range fs {
			out = append(out, f.ID)
		}
		return strings.Fields(strings.Join(out, " "))
	}

	for name, tc := range map[string]struct {
		roles []string
		want  string
	}{
		"trader":   {[]string{"trader"}, "both"},
		"cha":      {[]string{"cha"}, "both cha"},
		"both":     {[]string{"trader", "cha"}, "both cha"},
		"no roles": {nil, ""},
	} {
		if got := strings.Join(ids(flows.Startable(tc.roles)), " "); got != tc.want {
			t.Errorf("Startable(%s) = %q, want %q", name, got, tc.want)
		}
	}

	if _, ok := flows.Find("cha", []string{"trader"}); ok {
		t.Error("Find of a flow listed for another role: want none")
	}
	if f, ok := flows.Find("cha", []string{"cha"}); !ok || f.ID != "cha" {
		t.Errorf("Find(cha) = %+v, %v; want it", f, ok)
	}
	off := flows
	off.Enabled = false
	if got := off.Startable([]string{"cha"}); len(got) != 0 {
		t.Errorf("Startable while off = %v, want none", got)
	}
}

func TestClientFlowsFind(t *testing.T) {
	flows := ClientFlows{Enabled: true, Workflows: []ClientFlow{{ID: "review", Clients: []string{"tnsw"}}}}
	if f, ok := flows.Find("review", "tnsw"); !ok || f.ID != "review" {
		t.Errorf("Find(review, tnsw) = %+v, %v; want it", f, ok)
	}
	for name, tc := range map[string]struct {
		flows      ClientFlows
		id, client string
	}{
		"another client": {flows, "review", "fcau"},
		"not listed":     {flows, "other", "tnsw"},
		"list off":       {ClientFlows{Workflows: flows.Workflows}, "review", "tnsw"},
	} {
		if _, ok := tc.flows.Find(tc.id, tc.client); ok {
			t.Errorf("%s: want none", name)
		}
	}
}
