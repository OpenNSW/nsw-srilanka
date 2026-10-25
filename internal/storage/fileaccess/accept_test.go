package fileaccess_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileref"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

// services is the catalog's clientToService for these tests.
var services = map[string]string{"CDA_TO_NSW": "cda", "FCAU_TO_NSW": "fcau"}

func serviceFor(clientID string) (string, bool) {
	s, ok := services[clientID]
	return s, ok
}

// newTNSW returns an Access that knows which service each client is, on the
// keyset filled with b, and the clock it runs on.
func newTNSW(t *testing.T, b byte) (*fileaccess.Access, *clock) {
	t.Helper()
	c := &clock{t: newAccessTime}
	ks, err := filetoken.ParseKeyset("k1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)))
	if err != nil {
		t.Fatalf("ParseKeyset: %v", err)
	}
	codec, err := filetoken.NewCodec(ks, c.now)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	a, err := fileaccess.New(codec, userTTL, fileaccess.WithClientServices(serviceFor))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, c
}

func assertAccepted(t *testing.T, a *fileaccess.Access, p *authn.Principal, ref, want string) {
	t.Helper()
	got, err := a.Accept(p, ref)
	if err != nil {
		t.Fatalf("Accept(%q): %v", ref, err)
	}
	if got != want {
		t.Fatalf("Accept(%q) = %q, want %q", ref, got, want)
	}
}

func assertNotAccepted(t *testing.T, a *fileaccess.Access, p *authn.Principal, ref string, want error) {
	t.Helper()
	got, err := a.Accept(p, ref)
	if !errors.Is(err, want) {
		t.Fatalf("Accept(%q): err = %v, want %v", ref, err, want)
	}
	if got != "" {
		t.Fatalf("Accept(%q) returned %q alongside an error", ref, got)
	}
}

// A token this deployment issued is accepted exactly as Resolve would take it
// for a submission, whoever submits it.
func TestAccept_OwnTokens(t *testing.T) {
	a, _ := newTNSW(t, 1)
	upload := issue(t, a, user("alice"), "k.pdf", fileaccess.ScopeSubmit)
	view := issue(t, a, user("alice"), "k.pdf", fileaccess.ScopeRead)

	assertAccepted(t, a, user("alice"), upload, "k.pdf")
	assertNotAccepted(t, a, user("bob"), upload, fileaccess.ErrNotYours)
	assertNotAccepted(t, a, user("alice"), view, fileaccess.ErrScope)

	// A client's own upload token is accepted back from that client.
	clientUpload := issue(t, a, client("SLPA_TO_NSW"), "c.pdf")
	assertAccepted(t, a, client("SLPA_TO_NSW"), clientUpload, "c.pdf")
}

// A token this deployment issued is never taken for another service's, whatever
// is wrong with it: it is refused.
func TestAccept_OwnTokenIsNeverTagged(t *testing.T) {
	a, c := newTNSW(t, 1)
	pushed, err := a.IssueForClient("CDA_TO_NSW", "k.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	// A pushed token is read only: it cannot be sent back, even by its holder.
	assertNotAccepted(t, a, client("CDA_TO_NSW"), pushed, fileaccess.ErrScope)
	// Nor by another client.
	assertNotAccepted(t, a, client("FCAU_TO_NSW"), pushed, fileaccess.ErrNotYours)

	expiring := issue(t, a, user("alice"), "k.pdf", fileaccess.ScopeRead, fileaccess.ScopeSubmit)
	c.t = c.t.Add(userTTL)
	assertNotAccepted(t, a, user("alice"), expiring, fileaccess.ErrExpired)
	assertNotAccepted(t, a, client("CDA_TO_NSW"), expiring, fileaccess.ErrNotYours)
}

// What a mapped client sends that is not this deployment's token is that
// client's service's token.
func TestAccept_NamespaceFromClientOnly(t *testing.T) {
	a, _ := newTNSW(t, 1)
	other, _ := newAccess(t, 2)
	// A token CDA's own deployment issued to TNSW's client.
	fromCDA, err := other.IssueForClient("NSW_TO_CDA", "k.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}

	assertAccepted(t, a, client("CDA_TO_NSW"), fromCDA, "cda:"+fromCDA)
	// The same token from another client is that client's service's file.
	assertAccepted(t, a, client("FCAU_TO_NSW"), fromCDA, "fcau:"+fromCDA)

	// A value cannot name its own service.
	assertNotAccepted(t, a, client("CDA_TO_NSW"), "fcau:"+fromCDA, fileaccess.ErrInvalid)
	assertNotAccepted(t, a, client("CDA_TO_NSW"), "cda:"+fromCDA, fileaccess.ErrInvalid)
	assertNotAccepted(t, a, client("CDA_TO_NSW"), "k.pdf:x", fileaccess.ErrInvalid)

	// A client the catalog does not list sends no files.
	assertNotAccepted(t, a, client("SLPA_TO_NSW"), fromCDA, fileaccess.ErrNoService)
	assertNotAccepted(t, a, client(""), fromCDA, fileaccess.ErrNoService)
}

// Users send tokens of this deployment only.
func TestAccept_UserSendsNothingForeign(t *testing.T) {
	a, _ := newTNSW(t, 1)
	other, _ := newAccess(t, 2)
	fromCDA, err := other.IssueForClient("NSW_TO_CDA", "k.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{fromCDA, "k.pdf", "cda:" + fromCDA, "https://host/f.pdf"} {
		assertNotAccepted(t, a, user("alice"), ref, fileaccess.ErrInvalid)
	}
	assertNotAccepted(t, a, nil, fromCDA, fileaccess.ErrInvalid)
}

// The classification table: what a mapped client sends is a foreign token only
// if it is shaped as one, and never when it is a token of this deployment.
func TestAccept_Classification(t *testing.T) {
	a, _ := newTNSW(t, 1)
	other, _ := newAccess(t, 2)
	foreign, err := other.IssueForClient("NSW_TO_CDA", "k.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	tampered := foreign[:len(foreign)-1] + map[bool]string{true: "A", false: "B"}[foreign[len(foreign)-1] != 'A']
	nested, err := other.IssueForClient("NSW_TO_CDA", "tnsw:"+foreign, fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		ref  string
		want string
		err  error
	}{
		{"a peer's token", foreign, "cda:" + foreign, nil},
		{"a peer's token that wraps another", nested, "cda:" + nested, nil},
		// Indistinguishable from a peer's token, so it is stored and fails
		// when the peer opens it.
		{"a tampered token", tampered, "cda:" + tampered, nil},
		{"empty", "", "", fileaccess.ErrInvalid},
		{"a raw key", "k.pdf", "cda:k.pdf", nil},
		{"over the limit", strings.Repeat("a", fileref.MaxValueLength+1), "", fileaccess.ErrInvalid},
		{"with a slash", "a/b", "", fileaccess.ErrInvalid},
		{"dotdot", "..", "", fileaccess.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err != nil {
				assertNotAccepted(t, a, client("CDA_TO_NSW"), tt.ref, tt.err)
				return
			}
			assertAccepted(t, a, client("CDA_TO_NSW"), tt.ref, tt.want)
		})
	}
}

// Without a mapping, no client may send a foreign value.
func TestAccept_WithoutClientToService(t *testing.T) {
	a, _ := newAccess(t, 1)
	other, _ := newAccess(t, 2)
	foreign, err := other.IssueForClient("NSW_TO_CDA", "k.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	assertNotAccepted(t, a, client("CDA_TO_NSW"), foreign, fileaccess.ErrNoService)
}

// A token for a peer is no longer than the peer accepts.
func TestIssueForClient_PeerCap(t *testing.T) {
	a, _ := newTNSW(t, 1)
	if _, err := a.IssueForClient("CDA_TO_NSW", "3f2b9c1e-0a4d-4b7e-9c11-5d6e7f8a9b0c.pdf", fileaccess.ScopeRead); err != nil {
		t.Fatalf("IssueForClient for a key: %v", err)
	}
	// A value wrapped four times grows past what a peer takes.
	value, err := a.IssueForClient("CDA_TO_NSW", "3f2b9c1e-0a4d-4b7e-9c11-5d6e7f8a9b0c.pdf", fileaccess.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	for i := 0; i < 8 && failed == nil; i++ {
		value, failed = a.IssueForClient("CDA_TO_NSW", "cda:"+value, fileaccess.ScopeRead)
	}
	if !errors.Is(failed, fileaccess.ErrTooLong) {
		t.Fatalf("wrapping repeatedly: err = %v, want ErrTooLong", failed)
	}
}

// A user's token around the longest foreign value a peer may send still seals,
// and opens.
func TestIssue_UserTokenWrapsTheLongestForeignValue(t *testing.T) {
	a, _ := newTNSW(t, 1)
	stored := "cda:" + strings.Repeat("a", fileref.MaxValueLength)
	token := issue(t, a, user("alice"), stored)
	assertResolves(t, a, user("alice"), token, stored)
}
