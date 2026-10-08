package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

// fakeFileRefs issues references "ref1", "ref2", ... and resolves each only
// for the caller it was issued to, with fileaccess's errors.
type fakeFileRefs struct {
	issued  map[string]fakeRef
	expired map[string]bool
}

type fakeRef struct{ holder, value string }

func newFakeFileRefs() *fakeFileRefs {
	return &fakeFileRefs{issued: map[string]fakeRef{}, expired: map[string]bool{}}
}

func holder(p *authn.Principal) string {
	if p == nil {
		return ""
	}
	return string(p.Kind) + ":" + p.Subject()
}

func (f *fakeFileRefs) IssueFor(p *authn.Principal, value string) (string, error) {
	if holder(p) == "" {
		return "", errors.New("no caller")
	}
	ref := fmt.Sprintf("ref%d", len(f.issued)+1)
	f.issued[ref] = fakeRef{holder: holder(p), value: value}
	return ref, nil
}

func (f *fakeFileRefs) Resolve(p *authn.Principal, ref string) (string, error) {
	r, ok := f.issued[ref]
	switch {
	case !ok:
		return "", fileaccess.ErrInvalid
	case r.holder != holder(p):
		return "", fileaccess.ErrNotYours
	case f.expired[ref]:
		return "", fileaccess.ErrExpired
	}
	return r.value, nil
}

func userPrincipal(sub string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindUser, IDPUserID: sub}
}

// namespaces returns a NamespaceOf over a fixed map, counting lookups.
func namespaces(byTemplate map[string]string, calls *int) NamespaceOf {
	return func(_ context.Context, id string) (string, error) {
		*calls++
		ns, ok := byTemplate[id]
		if !ok {
			return "", fmt.Errorf("no step template %q", id)
		}
		return ns, nil
	}
}

func decodeMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

const filesRenderConfig = `{
  "id": "x:render",
  "files": ["form.invoice", "form.docs[*].file", "review.certificate"]
}`

func TestFileBinding_ForReaderGivesReferences(t *testing.T) {
	refs := newFakeFileRefs()
	b := NewFileBinding(refs, nil)
	record := store.TaskRecord{
		RenderConfig: json.RawMessage(filesRenderConfig),
		Data: decodeMap(t, `{
			"form": {"invoice": "k-invoice", "docs": [{"file": "k-doc", "name": "doc"}], "note": "k-note"},
			"review": {}
		}`),
	}
	stored := record.DeepCopy()

	view, err := b.ForReader(record, userPrincipal("alice"))
	require.NoError(t, err)

	assert.Equal(t, decodeMap(t, `{
		"form": {"invoice": "ref1", "docs": [{"file": "ref2", "name": "doc"}], "note": "k-note"},
		"review": {}
	}`), view.Data)
	assert.Equal(t, stored, record, "the record itself must not change")

	value, err := refs.Resolve(userPrincipal("alice"), "ref2")
	require.NoError(t, err)
	assert.Equal(t, "k-doc", value)
	_, err = refs.Resolve(userPrincipal("bob"), "ref2")
	assert.ErrorIs(t, err, fileaccess.ErrNotYours)
}

func TestFileBinding_ForReaderWithoutFiles(t *testing.T) {
	b := NewFileBinding(newFakeFileRefs(), nil)
	record := store.TaskRecord{RenderConfig: json.RawMessage(`{"id":"x:render"}`), Data: decodeMap(t, `{"form":{"invoice":"k"}}`)}

	view, err := b.ForReader(record, userPrincipal("alice"))
	require.NoError(t, err)
	assert.Equal(t, record, view)
}

func TestFileBinding_ForReaderRejectsABadDeclaration(t *testing.T) {
	b := NewFileBinding(newFakeFileRefs(), nil)
	record := store.TaskRecord{RenderConfig: json.RawMessage(`{"files":["form.docs[0]"]}`), Data: decodeMap(t, `{}`)}

	_, err := b.ForReader(record, userPrincipal("alice"))
	assert.Error(t, err)
}

func TestFileBinding_FromCallerResolvesTheStepsFields(t *testing.T) {
	refs := newFakeFileRefs()
	alice := userPrincipal("alice")
	invoice, _ := refs.IssueFor(alice, "k-invoice")
	doc, _ := refs.IssueFor(alice, "k-doc")
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls))
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), ActiveTaskTemplateID: "form-step"}
	// "certificate" is declared only under another step's namespace.
	payload := map[string]any{"invoice": invoice, "docs": []any{map[string]any{"file": doc}}, "certificate": "ref1", "__command": "submit"}

	require.NoError(t, b.FromCaller(context.Background(), record, payload, alice))

	assert.Equal(t, map[string]any{
		"invoice":     "k-invoice",
		"docs":        []any{map[string]any{"file": "k-doc"}},
		"certificate": "ref1",
		"__command":   "submit",
	}, payload)
	assert.Equal(t, 1, calls)
}

func TestFileBinding_FromCallerRefuses(t *testing.T) {
	refs := newFakeFileRefs()
	alicesRef, _ := refs.IssueFor(userPrincipal("alice"), "k")
	expiredRef, _ := refs.IssueFor(userPrincipal("bob"), "k")
	refs.expired[expiredRef] = true
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls))
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), ActiveTaskTemplateID: "form-step"}

	err := b.FromCaller(context.Background(), record, map[string]any{"invoice": alicesRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrNotYours)

	err = b.FromCaller(context.Background(), record, map[string]any{"invoice": expiredRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrExpired)
}

// A raw storage key, or anything else that is not a reference, is not stored.
func TestFileBinding_FromCallerRefusesValuesThatAreNotReferences(t *testing.T) {
	var calls int
	b := NewFileBinding(newFakeFileRefs(), namespaces(map[string]string{"form-step": "form"}, &calls))
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), ActiveTaskTemplateID: "form-step"}

	err := b.FromCaller(context.Background(), record, map[string]any{"invoice": "k-raw"}, userPrincipal("alice"))
	assert.ErrorIs(t, err, fileaccess.ErrInvalid)
}

func TestFileBinding_FromCallerWithoutNamespaceOrFiles(t *testing.T) {
	refs := newFakeFileRefs()
	ref, _ := refs.IssueFor(userPrincipal("alice"), "k")

	// Core stores nothing from a step without a namespace, so nothing is resolved.
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"bare-step": ""}, &calls))
	payload := map[string]any{"invoice": ref}
	record := store.TaskRecord{RenderConfig: json.RawMessage(`{"files":["invoice"]}`), ActiveTaskTemplateID: "bare-step"}
	require.NoError(t, b.FromCaller(context.Background(), record, payload, userPrincipal("bob")))
	assert.Equal(t, map[string]any{"invoice": ref}, payload)

	// A task that declares no files needs no step template.
	calls = 0
	record = store.TaskRecord{RenderConfig: json.RawMessage(`{"id":"x:render"}`), ActiveTaskTemplateID: "unknown"}
	require.NoError(t, b.FromCaller(context.Background(), record, payload, userPrincipal("bob")))
	assert.Zero(t, calls)
}

func TestFileBinding_FromCallerFailsWithoutTheStepTemplate(t *testing.T) {
	var calls int
	b := NewFileBinding(newFakeFileRefs(), namespaces(nil, &calls))
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), ActiveTaskTemplateID: "missing"}

	err := b.FromCaller(context.Background(), record, map[string]any{}, userPrincipal("alice"))
	assert.ErrorContains(t, err, `"missing"`)
}
