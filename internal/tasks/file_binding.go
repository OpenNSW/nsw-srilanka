package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/taskflow/store"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/filefields"
)

// FileRefs issues the file references a caller sees in place of the stored
// file values in task data, and resolves the ones a caller sends back.
// *fileaccess.Access satisfies it, and its errors are the ones returned.
type FileRefs interface {
	IssueFor(p *authn.Principal, value string) (string, error)
	Resolve(p *authn.Principal, ref string) (string, error)
}

// NamespaceOf returns the output namespace of the step template with the
// given ID: the top-level key of task data its submissions are stored under.
type NamespaceOf func(ctx context.Context, stepTemplateID string) (string, error)

// FileBinding swaps the file fields a task's render config declares (see
// filefields) between stored values, which stay on the server, and the file
// references callers hold.
type FileBinding struct {
	refs        FileRefs
	namespaceOf NamespaceOf
}

// NewFileBinding creates a FileBinding.
func NewFileBinding(refs FileRefs, namespaceOf NamespaceOf) *FileBinding {
	return &FileBinding{refs: refs, namespaceOf: namespaceOf}
}

// ForReader returns a copy of record in which each declared file value is a
// reference for p. record itself is not changed.
func (b *FileBinding) ForReader(record store.TaskRecord, p *authn.Principal) (store.TaskRecord, error) {
	paths, err := filefields.Parse(record.RenderConfig)
	if err != nil || len(paths) == 0 {
		return record, err
	}
	view := record.DeepCopy()
	err = filefields.Replace(view.Data, paths, func(value string) (string, error) {
		return b.refs.IssueFor(p, value)
	})
	if err != nil {
		return store.TaskRecord{}, err
	}
	return view, nil
}

// FromCaller replaces, in place, each declared file reference in payload, a
// submission for record's active step, with the stored value it stands for.
// Each must be a reference issued to p: the error is fileaccess.ErrNotYours
// or fileaccess.ErrExpired otherwise.
func (b *FileBinding) FromCaller(ctx context.Context, record store.TaskRecord, payload map[string]any, p *authn.Principal) error {
	paths, err := filefields.Parse(record.RenderConfig)
	if err != nil || len(paths) == 0 {
		return err
	}
	namespace, err := b.namespaceOf(ctx, record.ActiveTaskTemplateID)
	if err != nil {
		return fmt.Errorf("tasks: output namespace of step template %q: %w", record.ActiveTaskTemplateID, err)
	}
	if namespace == "" {
		// Core stores nothing from a step without one.
		return nil
	}
	// Declared paths start at the task data; the payload is what the step
	// stores under its namespace.
	var inPayload []filefields.Path
	for _, path := range paths {
		if rebased, ok := path.Rebase(namespace); ok {
			inPayload = append(inPayload, rebased)
		}
	}
	return filefields.Replace(payload, inPayload, func(ref string) (string, error) {
		value, err := b.refs.Resolve(p, ref)
		if errors.Is(err, fileaccess.ErrInvalid) {
			// Until every caller sends file tokens, a value that is not one
			// is stored as sent.
			return ref, nil
		}
		return value, err
	})
}
