package authz

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
)

func testCatalog() *Catalog {
	return &Catalog{
		Users:   map[string]string{"trader": "Trader", "cha": "CHA"},
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

// ownedFn returns an OwnedRolesFunc yielding owned/err and counting invocations.
func ownedFn(owned map[string]bool, err error, calls *int) OwnedRolesFunc {
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
		kind     PrincipalKind
		roles    []string
		clientID string
		owned    map[string]bool
		ownedErr error
		nilOwned bool // user Input with a nil resolver

		wantErr        error // sentinel; nil = allowed
		wantPlainError bool  // non-sentinel error expected
		wantResolves   int   // expected ownership-resolver invocations
	}{
		{
			name:    "command not allowed in state",
			props:   map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state:   "PENDING_USER",
			command: "delete",
			kind:    KindUser, roles: []string{"Trader"}, owned: map[string]bool{"trader": true},
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
			kind: KindUser, roles: []string{"Trader"}, owned: map[string]bool{"trader": true},
			wantErr: nil, wantResolves: 1,
		},
		{
			name:  "trader denied: role held but not owner",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: KindUser, roles: []string{"Trader"}, owned: map[string]bool{"trader": false},
			wantErr: ErrForbidden, wantResolves: 1,
		},
		{
			name:  "role not in allow list: denied without resolving",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}},
			state: "PENDING_USER", command: "submit",
			kind: KindUser, roles: []string{"Trader"}, owned: map[string]bool{"cha": true},
			wantErr: ErrForbidden, wantResolves: 0,
		},
		{
			name:  "cha allowed",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}},
			state: "PENDING_USER", command: "submit",
			kind: KindUser, roles: []string{"CHA"}, owned: map[string]bool{"cha": true},
			wantErr: nil, wantResolves: 1,
		},
		{
			// Security case: holds Trader+CHA, owns only as trader, CHA-only task.
			name:  "cross-role: owns only as trader, CHA-only task",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"cha"}}},
			state: "PENDING_USER", command: "submit",
			kind: KindUser, roles: []string{"Trader", "CHA"}, owned: map[string]bool{"trader": true, "cha": false},
			wantErr: ErrForbidden, wantResolves: 1,
		},
		{
			name:  "user with nil resolver denied",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: KindUser, roles: []string{"Trader"}, nilOwned: true,
			wantErr: ErrForbidden,
		},
		{
			name:  "resolver error is a 500-class error",
			props: map[string]map[string][]string{"PENDING_USER": {"submit": {"trader"}}},
			state: "PENDING_USER", command: "submit",
			kind: KindUser, roles: []string{"Trader"}, ownedErr: context.DeadlineExceeded,
			wantPlainError: true, wantResolves: 1,
		},
		{
			name:  "client allowed",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"approve": {"fcau"}}},
			state: "QUEUED_EXTERNALLY", command: "approve",
			kind: KindClient, clientID: "FCAU_TO_NSW",
			wantErr: nil,
		},
		{
			name:  "client denied: wrong client id",
			props: map[string]map[string][]string{"QUEUED_EXTERNALLY": {"approve": {"fcau"}}},
			state: "QUEUED_EXTERNALLY", command: "approve",
			kind: KindClient, clientID: "NPQS_TO_NSW",
			wantErr: ErrForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolves := 0
			in := Input{Kind: tc.kind, Roles: tc.roles, ClientID: tc.clientID}
			if tc.kind == KindUser && !tc.nilOwned {
				in.OwnedRoles = ownedFn(tc.owned, tc.ownedErr, &resolves)
			}

			ctx := context.Background()
			if !tc.noInput {
				ctx = WithInput(ctx, in)
			}
			record := &store.TaskRecord{TaskID: "task-1", State: tc.state, RootWorkflowID: "c1"}

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
	ctx := WithInput(context.Background(), Input{Kind: KindUser, Roles: []string{"Trader"}})
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
	ctx := WithInput(context.Background(), Input{Kind: KindUser, Roles: []string{"Trader"}})
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

func TestCatalog_Validate(t *testing.T) {
	tests := []struct {
		name    string
		catalog *Catalog
		wantErr bool
	}{
		{name: "nil", catalog: nil, wantErr: true},
		{name: "empty", catalog: &Catalog{}, wantErr: true},
		{name: "user missing role", catalog: &Catalog{Users: map[string]string{"trader": ""}}, wantErr: true},
		{name: "client missing id", catalog: &Catalog{Clients: map[string]string{"fcau": ""}}, wantErr: true},
		{name: "valid users only", catalog: &Catalog{Users: map[string]string{"trader": "Trader"}}, wantErr: false},
		{name: "valid full", catalog: testCatalog(), wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.catalog.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestLoadCatalog(t *testing.T) {
	dir := t.TempDir()

	valid := filepath.Join(dir, "ok.json")
	if err := os.WriteFile(valid, []byte(`{"users":{"trader":"Trader"},"clients":{"fcau":"FCAU_TO_NSW"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := LoadCatalog(valid); err != nil || c == nil || c.Users["trader"] != "Trader" {
		t.Fatalf("LoadCatalog(valid) = %v, %v", c, err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(bad); err == nil {
		t.Fatal("LoadCatalog(malformed) want error, got nil")
	}

	if _, err := LoadCatalog(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("LoadCatalog(missing) want error, got nil")
	}

	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"users":{},"clients":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(invalid); err == nil {
		t.Fatal("LoadCatalog(empty catalog) want validation error, got nil")
	}
}
