package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

type stubUserService struct {
	idpUserID                    string
	email, phone, ouID, ouHandle string
	callCount                    int
	returnID                     string
	returnErr                    error
}

func (s *stubUserService) GetUser(ctx context.Context, id string) (*user.Record, error) {
	return nil, nil
}

func (s *stubUserService) GetOrCreateUser(ctx context.Context, idpUserID, email, phone, ouID, ouHandle string) (string, error) {
	s.callCount++
	s.idpUserID = idpUserID
	s.email = email
	s.phone = phone
	s.ouID = ouID
	s.ouHandle = ouHandle
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

func TestAuthUserProfileAdapter_TranslatesExtraClaims(t *testing.T) {
	stub := &stubUserService{returnID: "user-123"}
	adapter := &authUserProfileAdapter{svc: stub}

	extra := authn.ExtraClaims{
		"email":        "trader@example.com",
		"phone_number": "+61400111222",
		"ouId":         "OU-001",
		"ouHandle":     "ou-001",
	}

	got, err := adapter.GetOrCreateUser(context.Background(), "idp-1", extra)
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
		stub.ouID != "OU-001" || stub.ouHandle != "ou-001" {
		t.Fatalf("unexpected forwarded args: %+v", stub)
	}
}

func TestAuthUserProfileAdapter_MissingClaimsForwardEmptyStrings(t *testing.T) {
	stub := &stubUserService{returnID: "user-456"}
	adapter := &authUserProfileAdapter{svc: stub}

	if _, err := adapter.GetOrCreateUser(context.Background(), "idp-2", nil); err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	if stub.email != "" || stub.phone != "" || stub.ouID != "" || stub.ouHandle != "" {
		t.Fatalf("expected empty strings for undeclared/nil extra claims, got %+v", stub)
	}
}

func TestAuthUserProfileAdapter_PropagatesError(t *testing.T) {
	wantErr := errors.New("db down")
	stub := &stubUserService{returnErr: wantErr}
	adapter := &authUserProfileAdapter{svc: stub}

	_, err := adapter.GetOrCreateUser(context.Background(), "idp-3", authn.ExtraClaims{"email": "a@b.com"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}
