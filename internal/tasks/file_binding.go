package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/OpenNSW/core/platform/authn"
	"github.com/OpenNSW/core/platform/filefields"
	"github.com/OpenNSW/core/platform/storage/fileaccess"
	"github.com/OpenNSW/core/platform/taskauthz"
	"github.com/OpenNSW/core/shared/deepcopy"
	"github.com/OpenNSW/core/taskflow/store"
)

// FileRefs issues the file references a caller sees in place of the stored
// file values in task data, and resolves the ones a caller sends back.
// *fileaccess.Access satisfies it, and its errors are the ones returned.
type FileRefs interface {
	Issue(p *authn.Principal, value string, scopes ...fileaccess.Scope) (string, error)
	Resolve(p *authn.Principal, ref string, need fileaccess.Scope) (string, error)
}

// CanAct reports whether in, the caller, may run some command on record's
// active step in its current state.
type CanAct func(ctx context.Context, record store.TaskRecord, in taskauthz.Input) (bool, error)

// NamespaceOf returns the output namespace of record's active step template:
// the top-level key of task data its submissions are stored under.
type NamespaceOf func(ctx context.Context, record store.TaskRecord) (string, error)

// RenderConfigOf returns record's render config, resolved through its task
// template in the manifest version it runs on (core's renderconfig.Load).
type RenderConfigOf func(ctx context.Context, record store.TaskRecord) (json.RawMessage, error)

// FileBinding swaps the file fields a task's render config declares (see
// filefields) between stored values, which stay on the server, and the file
// references callers hold.
type FileBinding struct {
	refs            FileRefs
	namespaceOf     NamespaceOf
	renderConfigOf  RenderConfigOf
	hashAuditValues bool
}

// FileBindingOption configures a FileBinding.
type FileBindingOption func(*FileBinding)

// WithHashedAuditValues hashes the values returned in FromCaller's added and
// removed diffs as lowercase hex SHA-256 instead of returning them verbatim.
// Use this when storage is proxied (agency mode), where the stored values are
// sealed file tokens from the owning service rather than plain storage keys.
func WithHashedAuditValues() FileBindingOption {
	return func(b *FileBinding) {
		b.hashAuditValues = true
	}
}

// NewFileBinding creates a FileBinding.
func NewFileBinding(refs FileRefs, namespaceOf NamespaceOf, renderConfigOf RenderConfigOf, opts ...FileBindingOption) *FileBinding {
	b := &FileBinding{refs: refs, namespaceOf: namespaceOf, renderConfigOf: renderConfigOf}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

// fileFields returns the file fields record's render config declares.
func (b *FileBinding) fileFields(ctx context.Context, record store.TaskRecord) ([]filefields.Path, error) {
	renderConfig, err := b.renderConfigOf(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	return filefields.Parse(renderConfig)
}

// ForReader returns a copy of record in which each declared file value is a
// reference for p. p can open each one and, when canAct says p may act on
// record's active step, submit it back. record itself is not changed.
func (b *FileBinding) ForReader(ctx context.Context, record store.TaskRecord, p *authn.Principal, canAct bool) (store.TaskRecord, error) {
	paths, err := b.fileFields(ctx, record)
	if err != nil || len(paths) == 0 {
		return record, err
	}
	scopes := []fileaccess.Scope{fileaccess.ScopeRead}
	if canAct {
		scopes = append(scopes, fileaccess.ScopeSubmit)
	}
	view := record.DeepCopy()
	err = filefields.Replace(view.Data, paths, func(value string) (string, error) {
		return b.refs.Issue(p, value, scopes...)
	})
	if err != nil {
		return store.TaskRecord{}, err
	}
	return view, nil
}

// FromCaller replaces, in place, each declared file reference in payload, a
// submission for record's active step, with the stored value it stands for,
// and returns the values added and removed compared to what the step last
// stored under its output namespace (hashed as hex SHA-256 when configured
// with WithHashedAuditValues). Each must be a reference issued to p that can
// be submitted: the error is fileaccess.ErrInvalid, fileaccess.ErrNotYours,
// fileaccess.ErrScope or fileaccess.ErrExpired otherwise.
func (b *FileBinding) FromCaller(ctx context.Context, record store.TaskRecord, payload map[string]any, p *authn.Principal) (added, removed []string, err error) {
	paths, err := b.fileFields(ctx, record)
	if err != nil || len(paths) == 0 {
		return nil, nil, err
	}
	namespace, err := b.namespaceOf(ctx, record)
	if err != nil {
		return nil, nil, fmt.Errorf("tasks: output namespace of step template %q: %w", record.ActiveTaskTemplateID, err)
	}
	if namespace == "" {
		// Core stores nothing from a step without one.
		return nil, nil, nil
	}
	// Declared paths start at the task data; the payload is what the step
	// stores under its namespace.
	var inPayload []filefields.Path
	for _, path := range paths {
		if rebased, ok := path.Rebase(namespace); ok {
			inPayload = append(inPayload, rebased)
		}
	}
	if len(inPayload) == 0 {
		return nil, nil, nil
	}
	current := make(map[string]struct{})
	err = filefields.Replace(payload, inPayload, func(ref string) (string, error) {
		val, resolveErr := b.refs.Resolve(p, ref, fileaccess.ScopeSubmit)
		if resolveErr != nil {
			return "", resolveErr
		}
		if val != "" {
			current[b.auditValue(val)] = struct{}{}
		}
		return val, nil
	})
	if err != nil {
		return nil, nil, err
	}

	previous := make(map[string]struct{})
	if nsData := record.Data[namespace]; nsData != nil {
		// Walk a copy so filefields.Replace never mutates record.Data in place.
		// A shape the audit walk cannot read omits the diff; the save still
		// proceeds with the values just resolved.
		if prevErr := filefields.Replace(deepcopy.Value(nsData), inPayload, func(v string) (string, error) {
			if v != "" {
				previous[b.auditValue(v)] = struct{}{}
			}
			return v, nil
		}); prevErr != nil {
			slog.WarnContext(ctx, "tasks: failed to read previously stored file values for audit diff",
				"taskId", record.TaskID, "stepId", record.ActiveStepID, "namespace", namespace, "error", prevErr)
			return nil, nil, nil
		}
	}
	added, removed = diffFileSets(previous, current)
	return added, removed, nil
}

func (b *FileBinding) auditValue(v string) string {
	if !b.hashAuditValues {
		return v
	}
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// diffFileSets returns the sorted stored values present only in current
// (added) or only in previous (removed).
func diffFileSets(previous, current map[string]struct{}) (added, removed []string) {
	for v := range current {
		if _, had := previous[v]; !had {
			added = append(added, v)
		}
	}
	for v := range previous {
		if _, has := current[v]; !has {
			removed = append(removed, v)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed
}
