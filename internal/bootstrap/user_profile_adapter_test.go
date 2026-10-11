package bootstrap

import (
	"context"
	"errors"
	"testing"

	nswauthn "github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

type stubUserService struct {
	idpUserID                    string
	email, phone, ouID, ouHandle string
	displayName                  string
	callCount                    int
	returnID                     string
	returnErr                    error
	// record and getErr are what GetUser returns.
	record *user.Record
	getErr error
}

func (s *stubUserService) GetUser(ctx context.Context, id string) (*user.Record, error) {
	return s.record, s.getErr
}

func (s *stubUserService) GetOrCreateUser(ctx context.Context, idpUserID, email, phone, ouID, ouHandle, displayName string) (string, error) {
	s.callCount++
	s.idpUserID = idpUserID
	s.email = email
	s.phone = phone
	s.ouID = ouID
	s.ouHandle = ouHandle
	s.displayName = displayName
	if s.returnErr != nil {
		return "", s.returnErr
	}
	return s.returnID, nil
}

func (s *stubUserService) UpdateUserData(ctx context.Context, id string, data []byte) error {
	return nil
}

func (s *stubUserService) Health(ctx context.Context) error {
	return nil
}

func TestAuthUserProfileAdapter_ForwardsPrincipalFields(t *testing.T) {
	stub := &stubUserService{returnID: "user-123"}
	adapter := &authUserProfileAdapter{svc: stub}

	got, err := adapter.GetOrCreateUser(context.Background(), &nswauthn.Principal{
		Kind:        nswauthn.KindUser,
		IDPUserID:   "idp-1",
		Email:       "trader@example.com",
		PhoneNumber: "+61400111222",
		OUID:        "OU-001",
		OUHandle:    "ou-001",
		DisplayName: "Kamal Perera",
	})
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	if got != "user-123" {
		t.Fatalf("GetOrCreateUser() = %q, want user-123", got)
	}
	if stub.callCount != 1 {
		t.Fatalf("expected underlying service to be called once, got %d", stub.callCount)
	}
	if stub.idpUserID != "idp-1" || stub.email != "trader@example.com" || stub.phone != "+61400111222" ||
		stub.ouID != "OU-001" || stub.ouHandle != "ou-001" || stub.displayName != "Kamal Perera" {
		t.Fatalf("unexpected forwarded args: %+v", stub)
	}
}

// A principal whose optional claims were absent from the token carries zero
// values, which must forward as empty strings rather than being dropped.
func TestAuthUserProfileAdapter_UnsetFieldsForwardEmptyStrings(t *testing.T) {
	stub := &stubUserService{returnID: "user-456"}
	adapter := &authUserProfileAdapter{svc: stub}

	if _, err := adapter.GetOrCreateUser(context.Background(), &nswauthn.Principal{
		Kind:      nswauthn.KindUser,
		IDPUserID: "idp-2",
	}); err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	if stub.idpUserID != "idp-2" {
		t.Fatalf("expected idp user id to be forwarded, got %q", stub.idpUserID)
	}
	if stub.email != "" || stub.phone != "" || stub.ouID != "" || stub.ouHandle != "" || stub.displayName != "" {
		t.Fatalf("expected empty strings for unset principal fields, got %+v", stub)
	}
}

func TestAuthUserProfileAdapter_PropagatesError(t *testing.T) {
	wantErr := errors.New("db down")
	stub := &stubUserService{returnErr: wantErr}
	adapter := &authUserProfileAdapter{svc: stub}

	_, err := adapter.GetOrCreateUser(context.Background(), &nswauthn.Principal{
		Kind:      nswauthn.KindUser,
		IDPUserID: "idp-3",
		Email:     "a@b.com",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}
