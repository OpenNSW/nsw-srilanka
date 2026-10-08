package fileaccess_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
)

const userTTL = time.Hour

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// newAccess returns an Access over a one-key keyset filled with b, and the
// clock it runs on.
func newAccess(t *testing.T, b byte) (*fileaccess.Access, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	ks, err := filetoken.ParseKeyset("k1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)))
	if err != nil {
		t.Fatalf("ParseKeyset: %v", err)
	}
	codec, err := filetoken.NewCodec(ks, c.now)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	a, err := fileaccess.New(codec, userTTL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, c
}

func user(sub string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindUser, IDPUserID: sub}
}

func client(id string) *authn.Principal {
	return &authn.Principal{Kind: authn.KindClient, ClientID: id}
}

func issue(t *testing.T, a *fileaccess.Access, p *authn.Principal, value string) string {
	t.Helper()
	token, err := a.IssueFor(p, value)
	if err != nil {
		t.Fatalf("IssueFor: %v", err)
	}
	return token
}

func assertResolves(t *testing.T, a *fileaccess.Access, p *authn.Principal, ref, want string) {
	t.Helper()
	got, err := a.Resolve(p, ref)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}

func assertRefused(t *testing.T, a *fileaccess.Access, p *authn.Principal, ref string, want error) {
	t.Helper()
	got, err := a.Resolve(p, ref)
	if !errors.Is(err, want) {
		t.Fatalf("Resolve: err = %v, want %v", err, want)
	}
	if got != "" {
		t.Fatalf("Resolve returned %q alongside an error", got)
	}
}

func TestUserToken(t *testing.T) {
	a, c := newAccess(t, 1)
	token := issue(t, a, user("alice"), "traders/invoice.pdf")

	assertResolves(t, a, user("alice"), token, "traders/invoice.pdf")
	assertRefused(t, a, user("bob"), token, fileaccess.ErrNotYours)
	assertRefused(t, a, client("alice"), token, fileaccess.ErrNotYours)

	c.t = c.t.Add(userTTL)
	assertRefused(t, a, user("alice"), token, fileaccess.ErrExpired)
	// Someone else's token is not theirs, expired or not.
	assertRefused(t, a, user("bob"), token, fileaccess.ErrNotYours)
}

// The token names the IdP subject, so it resolves whether or not the user's
// profile has been resolved on that request.
func TestUserTokenNamesSubject(t *testing.T) {
	a, _ := newAccess(t, 1)
	token := issue(t, a, &authn.Principal{Kind: authn.KindUser, IDPUserID: "sub-1", UserID: "user-1"}, "v")

	assertResolves(t, a, &authn.Principal{Kind: authn.KindUser, IDPUserID: "sub-1"}, token, "v")
	assertRefused(t, a, &authn.Principal{Kind: authn.KindUser, IDPUserID: "sub-2", UserID: "user-1"}, token, fileaccess.ErrNotYours)
}

func TestClientTokenNeverExpires(t *testing.T) {
	a, c := newAccess(t, 1)
	fromUpload := issue(t, a, client("CDA_TO_NSW"), "consignments/cert.pdf")
	fromPush, err := a.IssueForClient("CDA_TO_NSW", "consignments/cert.pdf")
	if err != nil {
		t.Fatalf("IssueForClient: %v", err)
	}

	c.t = c.t.AddDate(10, 0, 0)
	for _, token := range []string{fromUpload, fromPush} {
		assertResolves(t, a, client("CDA_TO_NSW"), token, "consignments/cert.pdf")
		assertRefused(t, a, client("CUSTOMS_TO_NSW"), token, fileaccess.ErrNotYours)
		assertRefused(t, a, user("CDA_TO_NSW"), token, fileaccess.ErrNotYours)
	}
}

func TestResolveRejectsNonTokens(t *testing.T) {
	a, _ := newAccess(t, 1)
	other, _ := newAccess(t, 2)
	fromAnotherDeployment := issue(t, other, user("alice"), "v")

	for _, ref := range []string{"", "traders/invoice.pdf", uuid.NewString(), fromAnotherDeployment} {
		assertRefused(t, a, user("alice"), ref, fileaccess.ErrInvalid)
	}
}

func TestCallerWithoutIdentity(t *testing.T) {
	a, _ := newAccess(t, 1)
	token := issue(t, a, user("alice"), "v")

	for name, p := range map[string]*authn.Principal{
		"nil":               nil,
		"user without sub":  {Kind: authn.KindUser, UserID: "alice"},
		"client without ID": {Kind: authn.KindClient},
		"unknown kind":      {IDPUserID: "alice", ClientID: "alice"},
	} {
		if _, err := a.IssueFor(p, "v"); err == nil {
			t.Errorf("%s: IssueFor succeeded", name)
		}
		if _, err := a.Resolve(p, token); !errors.Is(err, fileaccess.ErrNotYours) {
			t.Errorf("%s: Resolve err = %v, want ErrNotYours", name, err)
		}
	}
}

func TestIssueRejects(t *testing.T) {
	a, _ := newAccess(t, 1)
	if _, err := a.IssueFor(user("alice"), ""); err == nil {
		t.Error("IssueFor with no value succeeded")
	}
	if _, err := a.IssueForClient("", "v"); err == nil {
		t.Error("IssueForClient with no client ID succeeded")
	}
}

func TestNewRejects(t *testing.T) {
	ks, err := filetoken.ParseKeyset("k1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	codec, err := filetoken.NewCodec(ks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileaccess.New(nil, userTTL); err == nil {
		t.Error("New without a codec succeeded")
	}
	for _, ttl := range []time.Duration{0, -time.Second} {
		if _, err := fileaccess.New(codec, ttl); err == nil {
			t.Errorf("New with user lifetime %v succeeded", ttl)
		}
	}
}
