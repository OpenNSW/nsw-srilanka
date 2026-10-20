package authz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact/adapter/types"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

func testCatalog() taskauthz.Catalog {
	return taskauthz.Catalog{
		Roles:   map[string]string{"trader": "Trader", "cha": "CHA"},
		Clients: map[string]string{"fcau": "FCAU_TO_NSW"},
	}
}

func mustProps(t *testing.T, r map[string]map[string][]string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal props: %v", err)
	}
	return b
}

// ownedFn returns an taskauthz.OwnedRolesFunc yielding owned/err and counting invocations.
func ownedFn(owned map[string]bool, err error, calls *int) taskauthz.OwnedRolesFunc {
	return func(context.Context, string) (map[string]bool, error) {
		*calls++
		return owned, err
	}
}

func TestExtension_Execute(t *testing.T) {
	tests := []struct {
		name string

		props   map[string]map[string][]string
		state   string
		command string

		noInput  bool
		kind     taskauthz.PrincipalKind
		roles    []string
		clientID string
		owned    map[string]bool
		ownedErr error
		nilOwned bool // user taskauthz.Input with a nil resolver

		wantErr        error // sentinel; nil = allowed
		wantPlainError bool  // non-sentinel error expected
		wantResolves   int   // expected ownership-resolver invocations
	}{
		{
			name:    "command not allowed in state",
			props:   map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state:   "PENDING_USER",
			command: "delete",
			kind:    taskauthz.KindUser, roles: []string{"Trader"}, owned: map[string]bool{"trader": true},
			wantErr: ErrForbidden, wantResolves: 0,
		},
		{
			name:  "no input on context",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			noInput: true,
			wantErr: ErrUnauthenticated,
		},
		{
			name:  "trader allowed: role held and owns",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader"}, owned: map[string]bool{"trader": true},
			wantErr: nil, wantResolves: 1,
		},
		{
			name:  "trader denied: role held but not owner",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader"}, owned: map[string]bool{"trader": false},
			wantErr: ErrForbidden, wantResolves: 1,
		},
		{
			name:  "role not in allow list: denied without resolving",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader"}, owned: map[string]bool{"cha": true},
			wantErr: ErrForbidden, wantResolves: 0,
		},
		{
			name:  "cha allowed",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"CHA"}, owned: map[string]bool{"cha": true},
			wantErr: nil, wantResolves: 1,
		},
		{
			// Security case: holds Trader+CHA, owns only as trader, CHA-only task.
			name:  "cross-role: owns only as trader, CHA-only task",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader", "CHA"}, owned: map[string]bool{"trader": true, "cha": false},
			wantErr: ErrForbidden, wantResolves: 1,
		},
		{
			name:  "user with nil resolver denied",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader"}, nilOwned: true,
			wantErr: ErrForbidden,
		},
		{
			name:  "resolver error is a 500-class error",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader"}, ownedErr: context.DeadlineExceeded,
			wantPlainError: true, wantResolves: 1,
		},
		{
			name:  "client allowed",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"approve": {"fcau"}}},
			state: "QUEUED_EXTERNALLY", command: "approve",
			kind: taskauthz.KindClient, clientID: "FCAU_TO_NSW",
			wantErr: nil,
		},
		{
			name:  "client denied: wrong client id",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"approve": {"fcau"}}},
			state: "QUEUED_EXTERNALLY", command: "approve",
			kind: taskauthz.KindClient, clientID: "NPQS_TO_NSW",
			wantErr: ErrForbidden,
		},
		{
			name:  "system allowed: the rule names it",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"submit": {taskauthz.SystemPrincipal}}},
			state: "QUEUED_EXTERNALLY", command: "submit",
			kind:    taskauthz.KindSystem,
			wantErr: nil,
		},
		{
			name:  "system denied: the rule names only users",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader", "cha"}}},
			state: "PENDING_USER", command: "submit",
			kind:    taskauthz.KindSystem,
			wantErr: ErrForbidden,
		},
		{
			name:  "system denied: the rule names only a client",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"approve": {"fcau"}}},
			state: "QUEUED_EXTERNALLY", command: "approve",
			kind:    taskauthz.KindSystem,
			wantErr: ErrForbidden,
		},
		{
			// Security case: a system-only step (a payment, a webhook wait) must
			// refuse a user however many roles they hold and own.
			name:  "user denied on a system-only rule",
			props: map[string]map[string][]string{"PENDING_PAYMENT": {"submit": {taskauthz.SystemPrincipal}}},
			state: "PENDING_PAYMENT", command: "submit",
			kind: taskauthz.KindUser, roles: []string{"Trader", "CHA"}, owned: map[string]bool{"trader": true, "cha": true},
			wantErr: ErrForbidden, wantResolves: 0,
		},
		{
			name:  "client denied on a system-only rule",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"submit": {taskauthz.SystemPrincipal}}},
			state: "QUEUED_EXTERNALLY", command: "submit",
			kind: taskauthz.KindClient, clientID: "FCAU_TO_NSW",
			wantErr: ErrForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolves := 0
			in := taskauthz.Input{Kind: tc.kind, Roles: tc.roles, ClientID: tc.clientID}
			if tc.kind == taskauthz.KindUser && !tc.nilOwned {
				in.OwnedRoles = ownedFn(tc.owned, tc.ownedErr, &resolves)
			}

			ctx := context.Background()
			if !tc.noInput {
				ctx = taskauthz.WithInput(ctx, in)
			}
			record := &store.TaskRecord{TaskID: "task-1", State: tc.state, CaseID: "c1"}

			err := NewExtension(testCatalog()).Execute(ctx, record, map[string]any{"__command": tc.command}, mustProps(t, tc.props))

			switch {
			case tc.wantPlainError:
				if err == nil || errors.Is(err, ErrForbidden) || errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("want a non-sentinel error, got %v", err)
				}
			case tc.wantErr == nil:
				if err != nil {
					t.Fatalf("want allow (nil), got %v", err)
				}
			default:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want errors.Is(_, %v), got %v", tc.wantErr, err)
				}
			}
			if resolves != tc.wantResolves {
				t.Errorf("ownership resolver called %d times, want %d", resolves, tc.wantResolves)
			}
		})
	}
}

func TestExtension_MalformedProperties(t *testing.T) {
	ctx := taskauthz.WithInput(context.Background(), taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}})
	record := &store.TaskRecord{TaskID: "task-1", State: "PENDING_USER"}

	err := NewExtension(testCatalog()).Execute(ctx, record, map[string]any{"__command": "submit"}, json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("want error for malformed properties, got nil")
	}
	if errors.Is(err, ErrForbidden) || errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("malformed properties should be a non-sentinel (500-class) error, got %v", err)
	}
}

func TestExtension_EmptyProperties(t *testing.T) {
	ctx := taskauthz.WithInput(context.Background(), taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}})
	record := &store.TaskRecord{TaskID: "task-1", State: "PENDING_USER"}

	// Absent/empty rules deny (403), not 500. "null" and "{}" reach the same
	// deny-by-default path (nil/empty rule map).
	for _, props := range []json.RawMessage{nil, {}, json.RawMessage(`null`), json.RawMessage(`{}`)} {
		err := NewExtension(testCatalog()).Execute(ctx, record, map[string]any{"__command": "submit"}, props)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("properties %q: want ErrForbidden, got %v", string(props), err)
		}
	}
}

func TestExtension_MayAct(t *testing.T) {
	props := map[string]map[string][]string{
		"PENDING_USER":      {"submit": {"trader"}, "save_draft": {"cha"}},
		"PENDING_REVIEW":    {"approve": {"fcau"}},
		"QUEUED_EXTERNALLY": {"submit": {taskauthz.SystemPrincipal}},
		"COMPLETED":         {},
	}
	trader := func(owned map[string]bool, err error) taskauthz.Input {
		return taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}, OwnedRoles: ownedFn(owned, err, new(int))}
	}
	cha := taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"CHA"}, OwnedRoles: ownedFn(map[string]bool{"cha": true}, nil, new(int))}
	fcau := taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "FCAU_TO_NSW"}

	tests := []struct {
		name  string
		state string
		in    taskauthz.Input
		props json.RawMessage
		want  bool
	}{
		{"user with a role and ownership one command allows", "PENDING_USER", trader(map[string]bool{"trader": true}, nil), mustProps(t, props), true},
		{"another command's name in the same state", "PENDING_USER", cha, mustProps(t, props), true},
		{"user who doesn't own the case", "PENDING_USER", trader(map[string]bool{"cha": true}, nil), mustProps(t, props), false},
		{"user with no allowed role", "PENDING_REVIEW", trader(map[string]bool{"trader": true}, nil), mustProps(t, props), false},
		{"user without an ownership resolver", "PENDING_USER", taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}}, mustProps(t, props), false},
		{"client the catalog maps", "PENDING_REVIEW", fcau, mustProps(t, props), true},
		{"client in a state it has no rule in", "PENDING_USER", fcau, mustProps(t, props), false},
		{"another client", "PENDING_REVIEW", taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "CDA_TO_NSW"}, mustProps(t, props), false},
		{"a state with no commands", "COMPLETED", trader(map[string]bool{"trader": true}, nil), mustProps(t, props), false},
		{"a state with no rules", "ADVANCING", trader(map[string]bool{"trader": true}, nil), mustProps(t, props), false},
		{"no rules at all", "PENDING_USER", trader(map[string]bool{"trader": true}, nil), nil, false},
		{"no principal kind", "PENDING_USER", taskauthz.Input{}, mustProps(t, props), false},
		// A reader may act on a system-only step only if they are the system, which
		// no reader is: they get read-only file references there.
		{"user on a system-only step", "QUEUED_EXTERNALLY", trader(map[string]bool{"trader": true}, nil), mustProps(t, props), false},
		{"client on a system-only step", "QUEUED_EXTERNALLY", fcau, mustProps(t, props), false},
		{"system on a system-only step", "QUEUED_EXTERNALLY", taskauthz.Input{Kind: taskauthz.KindSystem}, mustProps(t, props), true},
		{"system on a step for users", "PENDING_USER", taskauthz.Input{Kind: taskauthz.KindSystem}, mustProps(t, props), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := &store.TaskRecord{TaskID: "t1", CaseID: "c1", State: tc.state}
			got, err := NewExtension(testCatalog()).MayAct(context.Background(), record, tc.in, tc.props)
			if err != nil {
				t.Fatalf("MayAct: %v", err)
			}
			if got != tc.want {
				t.Errorf("MayAct = %v, want %v", got, tc.want)
			}
		})
	}
}

// A denial is an answer; a failure to decide is an error.
func TestExtension_MayActErrors(t *testing.T) {
	record := &store.TaskRecord{TaskID: "t1", CaseID: "c1", State: "PENDING_USER"}
	in := taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}, OwnedRoles: ownedFn(nil, context.DeadlineExceeded, new(int))}
	props := mustProps(t, map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}})

	if _, err := NewExtension(testCatalog()).MayAct(context.Background(), record, in, props); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("failed ownership lookup: err = %v, want it returned", err)
	}
	if _, err := NewExtension(testCatalog()).MayAct(context.Background(), record, in, json.RawMessage(`{not json`)); err == nil {
		t.Error("malformed properties: MayAct succeeded")
	}
}

func TestExtension_MayActOnStep(t *testing.T) {
	traderRule := mustProps(t, map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}})
	chaRule := mustProps(t, map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}})
	authz := func(phase types.ExecutionPhase, props json.RawMessage) types.ExtensionConfig {
		return types.ExtensionConfig{ID: ExtAuthz, Phase: phase, Properties: props}
	}
	trader := taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}, OwnedRoles: ownedFn(map[string]bool{"trader": true}, nil, new(int))}

	tests := []struct {
		name       string
		extensions []types.ExtensionConfig
		want       bool
	}{
		{"an authz extension that allows the caller", []types.ExtensionConfig{authz(types.PhasePreResume, traderRule)}, true},
		// Core runs them all, so every one must allow the caller.
		{"one of two authz extensions refuses", []types.ExtensionConfig{authz(types.PhasePreResume, traderRule), authz(types.PhasePreResume, chaRule)}, false},
		{"one that refuses", []types.ExtensionConfig{authz(types.PhasePreResume, chaRule)}, false},
		// None of these checks who submits.
		{"an authz extension after the step resumes", []types.ExtensionConfig{authz(types.PhasePostResume, chaRule)}, true},
		{"another extension", []types.ExtensionConfig{{ID: "audit", Phase: types.PhasePreResume, Properties: chaRule}}, true},
		{"no extensions", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := &store.TaskRecord{TaskID: "t1", CaseID: "c1", State: "PENDING_USER"}
			got, err := NewExtension(testCatalog()).MayActOnStep(context.Background(), record, trader, types.StepTemplate{Extensions: tc.extensions})
			if err != nil {
				t.Fatalf("MayActOnStep: %v", err)
			}
			if got != tc.want {
				t.Errorf("MayActOnStep = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("a failure to decide", func(t *testing.T) {
		record := &store.TaskRecord{TaskID: "t1", CaseID: "c1", State: "PENDING_USER"}
		failing := taskauthz.Input{Kind: taskauthz.KindUser, Roles: []string{"Trader"}, OwnedRoles: ownedFn(nil, context.DeadlineExceeded, new(int))}
		step := types.StepTemplate{Extensions: []types.ExtensionConfig{authz(types.PhasePreResume, traderRule)}}
		if _, err := NewExtension(testCatalog()).MayActOnStep(context.Background(), record, failing, step); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want the ownership lookup's", err)
		}
	})
}
