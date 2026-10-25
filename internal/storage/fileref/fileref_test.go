package fileref_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileref"
)

// jwe has the shape of a sealed file token: a compact "dir" JWE, whose empty
// second segment leaves ".." in the middle.
const jwe = "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..AAAAAAAAAAAAAAAA.BBBB-_cc.DDDD"

func TestParse(t *testing.T) {
	const key = "3f2b9c1e-0a4d-4b7e-9c11-5d6e7f8a9b0c.pdf"
	tests := []struct {
		name   string
		stored string
		want   fileref.Ref
	}{
		{"own key", key, fileref.Ref{Value: key}},
		{"foreign", "cda:" + jwe, fileref.Ref{Namespace: "cda", Value: jwe}},
		{"hyphenated service", "customs-asycuda:abc", fileref.Ref{Namespace: "customs-asycuda", Value: "abc"}},
		{"empty", "", fileref.Ref{}},
		// Not well-formed foreign values stay own, and storage refuses them.
		{"url", "https://host/file.pdf", fileref.Ref{Value: "https://host/file.pdf"}},
		{"second colon", "cda:a:b", fileref.Ref{Value: "cda:a:b"}},
		{"empty token", "cda:", fileref.Ref{Value: "cda:"}},
		{"empty service", ":abc", fileref.Ref{Value: ":abc"}},
		{"service starts with digit", "1cda:abc", fileref.Ref{Value: "1cda:abc"}},
		{"token with slash", "cda:a/b", fileref.Ref{Value: "cda:a/b"}},
		{"token is dot", "cda:.", fileref.Ref{Value: "cda:."}},
		{"token is dotdot", "cda:..", fileref.Ref{Value: "cda:.."}},
		{"token over the limit", "cda:" + strings.Repeat("a", fileref.MaxValueLength+1), fileref.Ref{Value: "cda:" + strings.Repeat("a", fileref.MaxValueLength+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fileref.Parse(tt.stored))
		})
	}
}

func TestParse_TokenAtTheLimit(t *testing.T) {
	stored := "cda:" + strings.Repeat("a", fileref.MaxValueLength)
	ref := fileref.Parse(stored)
	assert.False(t, ref.IsOwn())
	assert.Equal(t, stored, ref.String())
}

func TestForeign_Rejects(t *testing.T) {
	tests := []struct {
		name, namespace, token string
	}{
		{"empty service", "", "abc"},
		{"service with colon", "a:b", "abc"},
		{"service with dot", "a.b", "abc"},
		{"service with slash", "a/b", "abc"},
		{"service over 63 characters", "a" + strings.Repeat("b", 63), "abc"},
		{"empty token", "cda", ""},
		{"token is dot", "cda", "."},
		{"token is dotdot", "cda", ".."},
		{"token with colon", "cda", "a:b"},
		{"token with query", "cda", "a?b"},
		{"token with fragment", "cda", "a#b"},
		{"token with percent", "cda", "a%2fb"},
		{"token with space", "cda", "a b"},
		{"token with newline", "cda", "a\nb"},
		{"token over the limit", "cda", strings.Repeat("a", fileref.MaxValueLength+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fileref.Foreign(tt.namespace, tt.token)
			assert.ErrorIs(t, err, fileref.ErrInvalid)
		})
	}
}

func TestForeign_AcceptsASealedToken(t *testing.T) {
	ref, err := fileref.Foreign("cda", jwe)
	require.NoError(t, err)
	assert.Equal(t, "cda:"+jwe, ref.String())
	assert.False(t, ref.IsOwn())
}

func TestOwn(t *testing.T) {
	ref := fileref.Own("k.pdf")
	assert.True(t, ref.IsOwn())
	assert.Equal(t, "k.pdf", ref.String())
}

func TestValidNamespace(t *testing.T) {
	assert.True(t, fileref.ValidNamespace("cda"))
	assert.True(t, fileref.ValidNamespace("slce-debug_forward2"))
	assert.False(t, fileref.ValidNamespace(""))
	assert.False(t, fileref.ValidNamespace("a:b"))
	assert.False(t, fileref.ValidNamespace("2a"))
}

// FuzzParse checks Parse never fails and that a foreign result survives a round
// trip, whatever the stored value.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"", "k.pdf", "cda:" + jwe, "a:b:c", ":", "..", "cda:..", "x:\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, stored string) {
		ref := fileref.Parse(stored)
		if ref.IsOwn() {
			if ref.Value != stored {
				t.Fatalf("own value %q differs from stored %q", ref.Value, stored)
			}
			return
		}
		if ref.String() != stored {
			t.Fatalf("round trip %q != %q", ref.String(), stored)
		}
		if _, err := fileref.Foreign(ref.Namespace, ref.Value); err != nil {
			t.Fatalf("parsed foreign value does not validate: %v", err)
		}
	})
}
