package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/OpenNSW/core/platform/authn"
	"github.com/OpenNSW/core/platform/storage/fileaccess"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	return func(_ context.Context, record store.TaskRecord) (string, error) {
		*calls++
		id := record.ActiveTaskTemplateID
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

// fixedRenderConfig resolves every task's render config to renderConfig.
func fixedRenderConfig(renderConfig string) RenderConfigOf {
	return func(context.Context, store.TaskRecord) (json.RawMessage, error) {
		return json.RawMessage(renderConfig), nil
	}
}

const filesRenderConfig = `{
  "id": "x:render",
  "files": ["form.invoice", "form.docs[*].file", "review.certificate"]
}`

func TestFileBinding_ForReaderGivesReferences(t *testing.T) {
	refs := newFakeFileRefs()
	b := NewFileBinding(refs, nil, fixedRenderConfig(filesRenderConfig))
	record := store.TaskRecord{
		Data: decodeMap(t, `{
			"form": {"invoice": "k-invoice", "docs": [{"file": "k-doc", "name": "doc"}], "note": "k-note"},
			"review": {}
		}`),
	}
	stored := record.DeepCopy()

	view, err := b.ForReader(context.Background(), record, userPrincipal("alice"), true)
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
	record := store.TaskRecord{Data: decodeMap(t, `{"form": {"invoice": "k-invoice"}}`)}

	view, err := NewFileBinding(refs, nil, fixedRenderConfig(filesRenderConfig)).ForReader(context.Background(), record, userPrincipal("alice"), false)
	require.NoError(t, err)
	assert.Equal(t, decodeMap(t, `{"form": {"invoice": "ref1"}}`), view.Data)
	assert.Equal(t, []fileaccess.Scope{fileaccess.ScopeRead}, refs.issued["ref1"].scopes)
}

func TestFileBinding_ForReaderWithoutFiles(t *testing.T) {
	b := NewFileBinding(newFakeFileRefs(), nil, fixedRenderConfig(`{"id":"x:render"}`))
	record := store.TaskRecord{Data: decodeMap(t, `{"form":{"invoice":"k"}}`)}

	view, err := b.ForReader(context.Background(), record, userPrincipal("alice"), false)
	require.NoError(t, err)
	assert.Equal(t, record, view)
}

func TestFileBinding_ForReaderRejectsABadDeclaration(t *testing.T) {
	b := NewFileBinding(newFakeFileRefs(), nil, fixedRenderConfig(`{"files":["form.docs[0]"]}`))
	record := store.TaskRecord{Data: decodeMap(t, `{}`)}

	_, err := b.ForReader(context.Background(), record, userPrincipal("alice"), false)
	assert.Error(t, err)
}

func TestFileBinding_FromCallerResolvesTheStepsFields(t *testing.T) {
	refs := newFakeFileRefs()
	alice := userPrincipal("alice")
	invoice, _ := refs.Issue(alice, "k-invoice", readSubmit...)
	doc, _ := refs.Issue(alice, "k-doc", readSubmit...)
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(filesRenderConfig))
	record := store.TaskRecord{ActiveTaskTemplateID: "form-step"}
	// "certificate" is declared only under another step's namespace.
	payload := map[string]any{"invoice": invoice, "docs": []any{map[string]any{"file": doc}}, "certificate": "ref1", "__command": "submit"}

	added, removed, err := b.FromCaller(context.Background(), record, payload, alice)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"invoice":     "k-invoice",
		"docs":        []any{map[string]any{"file": "k-doc"}},
		"certificate": "ref1",
		"__command":   "submit",
	}, payload)
	assert.Equal(t, []string{"k-doc", "k-invoice"}, added)
	assert.Empty(t, removed)
	assert.Equal(t, 1, calls)
}

func TestFileBinding_FromCallerRefuses(t *testing.T) {
	refs := newFakeFileRefs()
	alicesRef, _ := refs.Issue(userPrincipal("alice"), "k", readSubmit...)
	expiredRef, _ := refs.Issue(userPrincipal("bob"), "k", readSubmit...)
	refs.expired[expiredRef] = true
	readOnlyRef, _ := refs.Issue(userPrincipal("bob"), "k", fileaccess.ScopeRead)
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(filesRenderConfig))
	record := store.TaskRecord{ActiveTaskTemplateID: "form-step"}

	_, _, err := b.FromCaller(context.Background(), record, map[string]any{"invoice": alicesRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrNotYours)

	_, _, err = b.FromCaller(context.Background(), record, map[string]any{"invoice": expiredRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrExpired)

	_, _, err = b.FromCaller(context.Background(), record, map[string]any{"invoice": readOnlyRef}, userPrincipal("bob"))
	assert.ErrorIs(t, err, fileaccess.ErrScope)
}

// A raw storage key, or anything else that is not a reference, is not stored.
func TestFileBinding_FromCallerRefusesValuesThatAreNotReferences(t *testing.T) {
	var calls int
	b := NewFileBinding(newFakeFileRefs(), namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(filesRenderConfig))
	record := store.TaskRecord{ActiveTaskTemplateID: "form-step"}

	_, _, err := b.FromCaller(context.Background(), record, map[string]any{"invoice": "k-raw"}, userPrincipal("alice"))
	assert.ErrorIs(t, err, fileaccess.ErrInvalid)
}

func TestFileBinding_FromCallerWithoutNamespaceOrFiles(t *testing.T) {
	refs := newFakeFileRefs()
	ref, _ := refs.Issue(userPrincipal("alice"), "k", readSubmit...)

	// Core stores nothing from a step without a namespace, so nothing is resolved.
	var calls int
	renderConfig := `{"files":["invoice"]}`
	b := NewFileBinding(refs, namespaces(map[string]string{"bare-step": ""}, &calls), func(context.Context, store.TaskRecord) (json.RawMessage, error) {
		return json.RawMessage(renderConfig), nil
	})
	payload := map[string]any{"invoice": ref}
	record := store.TaskRecord{ActiveTaskTemplateID: "bare-step"}
	added, removed, err := b.FromCaller(context.Background(), record, payload, userPrincipal("bob"))
	require.NoError(t, err)
	assert.Empty(t, added)
	assert.Empty(t, removed)
	assert.Equal(t, map[string]any{"invoice": ref}, payload)

	// A task that declares no files needs no step template.
	calls = 0
	renderConfig = `{"id":"x:render"}`
	record = store.TaskRecord{ActiveTaskTemplateID: "unknown"}
	added, removed, err = b.FromCaller(context.Background(), record, payload, userPrincipal("bob"))
	require.NoError(t, err)
	assert.Empty(t, added)
	assert.Empty(t, removed)
	assert.Zero(t, calls)
}

func TestFileBinding_FromCallerFailsWithoutTheStepTemplate(t *testing.T) {
	var calls int
	b := NewFileBinding(newFakeFileRefs(), namespaces(nil, &calls), fixedRenderConfig(filesRenderConfig))
	record := store.TaskRecord{ActiveTaskTemplateID: "missing"}

	_, _, err := b.FromCaller(context.Background(), record, map[string]any{}, userPrincipal("alice"))
	assert.ErrorContains(t, err, `"missing"`)
}

func TestFileBinding_FromCallerDiffsStoredFiles(t *testing.T) {
	refs := newFakeFileRefs()
	alice := userPrincipal("alice")
	refA, _ := refs.Issue(alice, "k-a", readSubmit...)
	refB, _ := refs.Issue(alice, "k-b", readSubmit...)
	refC, _ := refs.Issue(alice, "k-c", readSubmit...)
	var calls int
	b := NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(filesRenderConfig))

	t.Run("first save reports attached files and ignores other namespaces", func(t *testing.T) {
		record := store.TaskRecord{
			ActiveTaskTemplateID: "form-step",
			Data:                 decodeMap(t, `{"review":{"certificate":"k-other"}}`),
		}
		added, removed, err := b.FromCaller(context.Background(), record, map[string]any{"invoice": refB, "docs": []any{map[string]any{"file": refA}}}, alice)
		require.NoError(t, err)
		assert.Equal(t, []string{"k-a", "k-b"}, added)
		assert.Empty(t, removed)
	})

	t.Run("unchanged files report no diff regardless of order or duplicates", func(t *testing.T) {
		record := store.TaskRecord{
			ActiveTaskTemplateID: "form-step",
			Data:                 decodeMap(t, `{"form":{"invoice":"k-a","docs":[{"file":"k-b"},{"file":"k-a"}]}}`),
		}
		added, removed, err := b.FromCaller(context.Background(), record, map[string]any{"invoice": refB, "docs": []any{map[string]any{"file": refA}}}, alice)
		require.NoError(t, err)
		assert.Empty(t, added)
		assert.Empty(t, removed)
	})

	t.Run("added and removed files are sorted and record is not mutated", func(t *testing.T) {
		record := store.TaskRecord{
			ActiveTaskTemplateID: "form-step",
			Data:                 decodeMap(t, `{"form":{"invoice":"k-a","docs":[{"file":"k-b"}]},"review":{"certificate":"k-other"}}`),
		}
		before := record.DeepCopy()
		added, removed, err := b.FromCaller(context.Background(), record, map[string]any{"invoice": refC, "docs": []any{map[string]any{"file": refB}}}, alice)
		require.NoError(t, err)
		assert.Equal(t, []string{"k-c"}, added)
		assert.Equal(t, []string{"k-a"}, removed)
		assert.Equal(t, before, record)
	})

	t.Run("removing all files reports every previously stored key", func(t *testing.T) {
		record := store.TaskRecord{
			ActiveTaskTemplateID: "form-step",
			Data:                 decodeMap(t, `{"form":{"invoice":"k-b","docs":[{"file":"k-a"}]}}`),
		}
		added, removed, err := b.FromCaller(context.Background(), record, map[string]any{"invoice": "", "docs": []any{}}, alice)
		require.NoError(t, err)
		assert.Empty(t, added)
		assert.Equal(t, []string{"k-a", "k-b"}, removed)
	})

	t.Run("omitting file fields reports previously stored keys as removed", func(t *testing.T) {
		record := store.TaskRecord{
			ActiveTaskTemplateID: "form-step",
			Data:                 decodeMap(t, `{"form":{"invoice":"k-b","docs":[{"file":"k-a"}]}}`),
		}
		added, removed, err := b.FromCaller(context.Background(), record, map[string]any{"note": "cleared"}, alice)
		require.NoError(t, err)
		assert.Empty(t, added)
		assert.Equal(t, []string{"k-a", "k-b"}, removed)
	})

	t.Run("malformed historical data does not block a valid save", func(t *testing.T) {
		record := store.TaskRecord{
			TaskID:               "t1",
			ActiveStepID:         "s1",
			ActiveTaskTemplateID: "form-step",
			Data:                 decodeMap(t, `{"form":{"invoice":["not-a-string"]}}`),
		}
		payload := map[string]any{"invoice": refC}
		added, removed, err := b.FromCaller(context.Background(), record, payload, alice)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"invoice": "k-c"}, payload)
		assert.Empty(t, added)
		assert.Empty(t, removed)
	})
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestFileBinding_FromCallerHashesAuditValuesInProxyMode(t *testing.T) {
	refs := newFakeFileRefs()
	alice := userPrincipal("alice")
	const (
		oldToken  = "jwe.tnsw.old-token"
		keptToken = "jwe.tnsw.kept-token"
		newTokenA = "jwe.tnsw.new-token-a"
		newTokenB = "jwe.tnsw.new-token-b"
	)
	refKept, _ := refs.Issue(alice, keptToken, readSubmit...)
	refNewA, _ := refs.Issue(alice, newTokenA, readSubmit...)
	refNewB, _ := refs.Issue(alice, newTokenB, readSubmit...)
	var calls int
	b := NewFileBinding(
		refs,
		namespaces(map[string]string{"form-step": "form"}, &calls),
		fixedRenderConfig(filesRenderConfig),
		WithHashedAuditValues(),
	)
	record := store.TaskRecord{
		ActiveTaskTemplateID: "form-step",
		Data: decodeMap(t, `{
			"form": {"invoice": "`+oldToken+`", "docs": [{"file": "`+keptToken+`"}]}
		}`),
	}
	payload := map[string]any{
		"invoice": refNewA,
		"docs":    []any{map[string]any{"file": refKept}, map[string]any{"file": refNewB}},
	}

	added, removed, err := b.FromCaller(context.Background(), record, payload, alice)
	require.NoError(t, err)

	// Payload still receives the raw stored tokens for core and workflows.
	assert.Equal(t, map[string]any{
		"invoice": newTokenA,
		"docs":    []any{map[string]any{"file": keptToken}, map[string]any{"file": newTokenB}},
	}, payload)

	wantAdded := []string{sha256Hex(newTokenA), sha256Hex(newTokenB)}
	slices.Sort(wantAdded)
	assert.Equal(t, wantAdded, added)
	assert.Equal(t, []string{sha256Hex(oldToken)}, removed)
	assert.NotContains(t, added, newTokenA)
	assert.NotContains(t, added, newTokenB)
	assert.NotContains(t, removed, oldToken)
}
