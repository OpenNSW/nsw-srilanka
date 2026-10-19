package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
)

// fakeFileRefs issues references "ref1", "ref2", ... and resolves each only
// for the caller it was issued to and for the scopes it was issued with, with
// fileaccess's errors.
type fakeFileRefs struct {
	issued  map[string]fakeRef
	expired map[string]bool
}

type fakeRef struct {
	holder, value string
	scopes        []fileaccess.Scope
}

// readSubmit are the scopes of a reference a reader who can act is given.
var readSubmit = []fileaccess.Scope{fileaccess.ScopeRead, fileaccess.ScopeSubmit}

func newFakeFileRefs() *fakeFileRefs {
	return &fakeFileRefs{issued: map[string]fakeRef{}, expired: map[string]bool{}}
}

func holder(p *authn.Principal) string {
	if p == nil {
		return ""
	}
	return string(p.Kind) + ":" + p.Subject()
}

func (f *fakeFileRefs) Issue(p *authn.Principal, value string, scopes ...fileaccess.Scope) (string, error) {
	if holder(p) == "" {
		return "", errors.New("no caller")
	}
	if len(scopes) == 0 {
		return "", errors.New("no scope")
	}
	ref := fmt.Sprintf("ref%d", len(f.issued)+1)
	f.issued[ref] = fakeRef{holder: holder(p), value: value, scopes: scopes}
	return ref, nil
}

func (f *fakeFileRefs) Resolve(p *authn.Principal, ref string, need fileaccess.Scope) (string, error) {
	r, ok := f.issued[ref]
	switch {
	case !ok:
		return "", fileaccess.ErrInvalid
	case r.holder != holder(p):
		return "", fileaccess.ErrNotYours
	case !slices.Contains(r.scopes, need):
		return "", fileaccess.ErrScope
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

	view, err := b.ForReader(record, userPrincipal("alice"), true)
	require.NoError(t, err)

	assert.Equal(t, decodeMap(t, `{
		"form": {"invoice": "ref1", "docs": [{"file": "ref2", "name": "doc"}], "note": "k-note"},
		"review": {}
	}`), view.Data)
	assert.Equal(t, stored, record, "the record itself must not change")
	assert.Equal(t, readSubmit, refs.issued["ref1"].scopes)

	value, err := refs.Resolve(userPrincipal("alice"), "ref2", fileaccess.ScopeRead)
	require.NoError(t, err)
	assert.Equal(t, "k-doc", value)
	_, err = refs.Resolve(userPrincipal("bob"), "ref2", fileaccess.ScopeRead)
	assert.ErrorIs(t, err, fileaccess.ErrNotYours)
}

// A reader who can't act on the step can open the task's files but not
// submit them.
func TestFileBinding_ForReaderWhoCannotAct(t *testing.T) {
	refs := newFakeFileRefs()
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), Data: decodeMap(t, `{"form": {"invoice": "k-invoice"}}`)}

	view, err := NewFileBinding(refs, nil).ForReader(record, userPrincipal("alice"), false)
	require.NoError(t, err)
	assert.Equal(t, decodeMap(t, `{"form": {"invoice": "ref1"}}`), view.Data)
	assert.Equal(t, []fileaccess.Scope{fileaccess.ScopeRead}, refs.issued["ref1"].scopes)
}

func TestFileBinding_ForReaderWithoutFiles(t *testing.T) {
	b := NewFileBinding(newFakeFileRefs(), nil)
	record := store.TaskRecord{RenderConfig: json.RawMessage(`{"id":"x:render"}`), Data: decodeMap(t, `{"form":{"invoice":"k"}}`)}

	view, err := b.ForReader(record, userPrincipal("alice"), false)
	require.NoError(t, err)
	assert.Equal(t, record, view)
}

func TestFileBinding_ForReaderRejectsABadDeclaration(t *testing.T) {
	b := NewFileBinding(newFakeFileRefs(), nil)
	record := store.TaskRecord{RenderConfig: json.RawMessage(`{"files":["form.docs[0]"]}`), Data: decodeMap(t, `{}`)}

	_, err := b.ForReader(record, userPrincipal("alice"), false)
	assert.Error(t, err)
}

func TestFileBinding_FromCallerResolvesTheStepsFields(t *testing.T) {
	refs := newFakeFileRefs()
	alice := userPrincipal("alice")
	invoice, _ := refs.Issue(alice, "k-invoice", readSubmit...)
	doc, _ := refs.Issue(alice, "k-doc", readSubmit...)
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
	alicesRef, _ := refs.Issue(userPrincipal("alice"), "k", readSubmit...)
	expiredRef, _ := refs.Issue(userPrincipal("bob"), "k", readSubmit...)
	refs.expired[expiredRef] = true
	readOnlyRef, _ := refs.Issue(userPrincipal("bob"), "k", fileaccess.ScopeRead)
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls))
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), ActiveTaskTemplateID: "form-step"}

	err := b.FromCaller(context.Background(), record, map[string]any{"invoice": alicesRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrNotYours)

	err = b.FromCaller(context.Background(), record, map[string]any{"invoice": expiredRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrExpired)

	err = b.FromCaller(context.Background(), record, map[string]any{"invoice": readOnlyRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrScope)
}

// Until every caller sends file tokens, a value that is not one is stored as
// sent.
func TestFileBinding_FromCallerKeepsValuesThatAreNotReferences(t *testing.T) {
	var calls int
	b := NewFileBinding(newFakeFileRefs(), namespaces(map[string]string{"form-step": "form"}, &calls))
	record := store.TaskRecord{RenderConfig: json.RawMessage(filesRenderConfig), ActiveTaskTemplateID: "form-step"}
	payload := map[string]any{"invoice": "k-raw"}

	require.NoError(t, b.FromCaller(context.Background(), record, payload, userPrincipal("alice")))
	assert.Equal(t, map[string]any{"invoice": "k-raw"}, payload)
}

func TestFileBinding_FromCallerWithoutNamespaceOrFiles(t *testing.T) {
	refs := newFakeFileRefs()
	ref, _ := refs.Issue(userPrincipal("alice"), "k", readSubmit...)

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
