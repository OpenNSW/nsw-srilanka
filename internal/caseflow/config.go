package caseflow

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/tasktemplate"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	"github.com/OpenNSW/core/json/jsonpointer"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
)

// Config is the flows section of config.yaml: the workflows a caller outside this
// deployment's own code may start on a case, in one list per kind of caller. Code in this
// deployment starts flows through StartFlow directly and needs no list.
//
// The lists are kept apart because their callers are trusted differently. A person may
// start a listed flow only on a case they own and sends no input: the server sets what the
// flow trusts. Another system may create the case and send the flow its input. Each list
// names, per workflow, who may start it, by the catalog's logical names.
type Config struct {
	// Users are the workflows a person may start on a case they own.
	Users UserFlows `yaml:"users"`
	// Clients are the workflows another system may start on a case, as an OAuth2 client.
	Clients ClientFlows `yaml:"clients"`
}

// UserFlows are the workflows a person may start on a case.
type UserFlows struct {
	// Enabled turns the list on. Off, which is the default, nothing in it can be started.
	Enabled   bool       `yaml:"enabled"`
	Workflows []UserFlow `yaml:"workflows"`
}

// UserFlow is one workflow a person may start on a case.
type UserFlow struct {
	// ID is the workflow definition the flow runs.
	ID string `yaml:"id"`
	// Label is what the portal's button for the flow says.
	Label string `yaml:"label"`
	// Roles are the catalog roles that may start it: the caller must hold one and own the
	// case in it.
	Roles []string `yaml:"roles"`
}

// ClientFlows are the workflows another system may start on a case.
type ClientFlows struct {
	// Enabled turns the list on. Off, which is the default, nothing in it can be started.
	Enabled bool `yaml:"enabled"`
	// OUHandle is this deployment's own OU, by the handle its users' tokens carry. Starting
	// a client flow records it as an owner of the case, in each role the flow's task reads
	// (see Owner): the caller is a system with no OU, and the case belongs to this
	// deployment. A user sees the case once a flow on it has a task they may read.
	OUHandle string `yaml:"ouHandle"`
	// CaseColumns fills cases columns from the attributes a client sends with a flow: each
	// key is a column (CaseColumnName is the only one), each value the JSON Pointer
	// (RFC 6901) of a string in the attributes, such as "/traderCompany/name". The
	// attributes themselves are stored whether or not anything maps them.
	CaseColumns map[string]string `yaml:"caseColumns"`
	Workflows   []ClientFlow      `yaml:"workflows"`
}

// CaseColumnName is cases.name, the name an officer sees for a case.
const CaseColumnName = "name"

// ClientFlow is one workflow another system may start on a case.
type ClientFlow struct {
	// ID is the workflow definition the flow runs.
	ID string `yaml:"id"`
	// Clients are the catalog clients that may start it.
	Clients []string `yaml:"clients"`
}

// Validate checks every listed flow, enabled or not, against the catalog: it names a
// workflow once, and who may start it by names the catalog defines. A user flow also
// needs the label its button shows; a client flow, the list the deployment's OU that owns
// its case. Messages are unprefixed so the config can qualify them with the section.
func (c Config) Validate(cat *catalog.Catalog) error {
	seen := map[string]bool{}
	for i, f := range c.Users.Workflows {
		where := fmt.Sprintf("users.workflows[%d]", i)
		if err := checkID(where, f.ID, seen); err != nil {
			return err
		}
		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("%s (%s): label is required", where, f.ID)
		}
		if err := checkNames(where, f.ID, "roles", f.Roles, cat.Roles); err != nil {
			return err
		}
	}
	if len(c.Clients.Workflows) > 0 && strings.TrimSpace(c.Clients.OUHandle) == "" {
		return fmt.Errorf("clients.ouHandle is required: the OU that owns the cases client flows start")
	}
	for column, pointer := range c.Clients.CaseColumns {
		if column != CaseColumnName {
			return fmt.Errorf("clients.caseColumns names %q, which is not a case column it can fill (only %q)", column, CaseColumnName)
		}
		if !jsonpointer.Valid(pointer) {
			return fmt.Errorf("clients.caseColumns.%s: %q is not a JSON Pointer to an attribute, such as /traderCompany/name", column, pointer)
		}
	}
	seen = map[string]bool{}
	for i, f := range c.Clients.Workflows {
		where := fmt.Sprintf("clients.workflows[%d]", i)
		if err := checkID(where, f.ID, seen); err != nil {
			return err
		}
		if err := checkNames(where, f.ID, "clients", f.Clients, cat.Clients); err != nil {
			return err
		}
	}
	return nil
}

// checkID reports an empty id, or one already in seen, and adds it to seen.
func checkID(where, id string, seen map[string]bool) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s: id is required", where)
	}
	if seen[id] {
		return fmt.Errorf("%s: workflow %q is listed twice", where, id)
	}
	seen[id] = true
	return nil
}

// checkNames reports an empty list of names, or a name the catalog section defined does
// not hold: a flow nobody may start is a mistake, and a name the catalog does not define
// matches nobody.
func checkNames(where, id, field string, names []string, defined map[string]string) error {
	if len(names) == 0 {
		return fmt.Errorf("%s (%s): %s is required", where, id, field)
	}
	for _, name := range names {
		if _, ok := defined[name]; !ok {
			return fmt.Errorf("%s (%s): %s names %q, which is not in the catalog's %s", where, id, field, name, field)
		}
	}
	return nil
}

// CheckWorkflows reports a listed workflow, enabled or not, that the current artifact
// manifest does not hold. A client flow's workflow must also have one task, whose
// read.roles name only catalog roles, and at least one: its case is owned in them. The
// server checks at startup, so a typo, or a role the artifacts name before the catalog
// maps it, fails there and not at the first start.
func (c Config) CheckWorkflows(ctx context.Context, artifacts *artifact.Set, cat *catalog.Catalog) error {
	reg := artifacts.CurrentRegistry()
	load := func(list, id string) (workflow.WorkflowDefinition, error) {
		def, err := workflowdef.Load(ctx, reg, id)
		if err != nil {
			return def, fmt.Errorf("flows.%s lists workflow %q, which manifest version %d does not hold: %w", list, id, artifacts.Current(), err)
		}
		return def, nil
	}
	for _, f := range c.Users.Workflows {
		if _, err := load("users", f.ID); err != nil {
			return err
		}
	}
	for _, f := range c.Clients.Workflows {
		def, err := load("clients", f.ID)
		if err != nil {
			return err
		}
		owner, err := c.Clients.Owner(ctx, reg, def)
		if err != nil {
			return fmt.Errorf("flows.clients lists workflow %q, in manifest version %d: %w", f.ID, artifacts.Current(), err)
		}
		for _, role := range owner.Roles {
			if _, ok := cat.Roles[role]; !ok {
				return fmt.Errorf("flows.clients lists workflow %q, whose task reads role %q in manifest version %d, which is not in the catalog's roles", f.ID, role, artifacts.Current())
			}
		}
	}
	return nil
}

// Startable returns the flows a person may start on a case they own in any of roles (logical
// names), in list order. None when the list is off.
func (f UserFlows) Startable(roles []string) []UserFlow {
	if !f.Enabled {
		return nil
	}
	var startable []UserFlow
	for _, flow := range f.Workflows {
		if flow.startableBy(roles) {
			startable = append(startable, flow)
		}
	}
	return startable
}

// Find returns the flow id names if a person owning the case in any of roles (logical
// names) may start it. ok is false when the list is off, does not hold id, or holds it
// for other roles only.
func (f UserFlows) Find(id string, roles []string) (flow UserFlow, ok bool) {
	for _, flow := range f.Startable(roles) {
		if flow.ID == id {
			return flow, true
		}
	}
	return UserFlow{}, false
}

func (f UserFlow) startableBy(roles []string) bool {
	return slices.ContainsFunc(f.Roles, func(role string) bool { return slices.Contains(roles, role) })
}

// Find returns the flow id names if client (a logical name) may start it. ok is false when
// the list is off, does not hold id, or holds it for other clients only.
func (f ClientFlows) Find(id, client string) (flow ClientFlow, ok bool) {
	if !f.Enabled {
		return ClientFlow{}, false
	}
	for _, flow := range f.Workflows {
		if flow.ID == id && slices.Contains(flow.Clients, client) {
			return flow, true
		}
	}
	return ClientFlow{}, false
}

// Owner returns this deployment's OU (OUHandle) as the owner of the case of a client flow
// that runs def, in each role the flow's task reads: the read.roles of the task template
// of def's one TASK node, as reg holds it. Each role that may read the task then sees the
// case. It fails when def has no TASK node or more than one, or when the task template
// names no role, which would leave the case owned in none.
func (f ClientFlows) Owner(ctx context.Context, reg *artifact.Registry, def workflow.WorkflowDefinition) (*Owner, error) {
	var templateID string
	tasks := 0
	for _, n := range def.Nodes {
		if n.Type == workflow.NodeTypeTask {
			tasks++
			templateID = n.TaskTemplateID
		}
	}
	if tasks != 1 {
		return nil, fmt.Errorf("workflow %q has %d tasks, want one: its case is owned in the roles that task reads", def.ID, tasks)
	}
	t, err := tasktemplate.Load(ctx, reg, templateID)
	if err != nil {
		return nil, fmt.Errorf("workflow %q: failed to load task template %q: %w", def.ID, templateID, err)
	}
	if len(t.Read.Roles) == 0 {
		return nil, fmt.Errorf("task template %q of workflow %q has no read.roles, so its case would be owned in no role", templateID, def.ID)
	}
	return &Owner{OUHandle: f.OUHandle, Roles: t.Read.Roles}, nil
}
