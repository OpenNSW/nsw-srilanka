package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/authn"

	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

type stubUserService struct {
	getUserFn func(ctx context.Context, id string) (*user.Record, error)
}

func (s *stubUserService) GetUser(ctx context.Context, id string) (*user.Record, error) {
	if s.getUserFn != nil {
		return s.getUserFn(ctx, id)
	}
	return nil, nil
}
func (s *stubUserService) GetOrCreateUser(_ context.Context, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (s *stubUserService) UpdateUserData(_ context.Context, _ string, _ []byte) error { return nil }
func (s *stubUserService) Health(_ context.Context) error                             { return nil }

type stubCompanyService struct {
	getCompanyByOUHandleFn func(ctx context.Context, ouHandle string) (*company.Record, error)
}

func (s *stubCompanyService) GetCompanyByID(_ context.Context, _ string) (*company.Record, error) {
	return nil, nil
}
func (s *stubCompanyService) GetCompanyByOUHandle(ctx context.Context, ouHandle string) (*company.Record, error) {
	if s.getCompanyByOUHandleFn != nil {
		return s.getCompanyByOUHandleFn(ctx, ouHandle)
	}
	return nil, nil
}
func (s *stubCompanyService) ListCompanies(_ context.Context, _ company.ListFilter) (*company.ListResult, error) {
	return nil, nil
}
func (s *stubCompanyService) UpdateCompany(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (s *stubCompanyService) Health(_ context.Context) error { return nil }
func (s *stubCompanyService) CreateCompany(_ context.Context, _ *company.Record) error {
	return nil
}

func withAuthContext(r *http.Request, userID, ouHandle string) *http.Request {
	authCtx := &authn.AuthContext{User: &authn.UserContext{ID: userID, OUHandle: ouHandle}}
	return r.WithContext(context.WithValue(r.Context(), authn.AuthContextKey, authCtx))
}

func TestHandler_HandleGetProfile_Unauthorized(t *testing.T) {
	h := NewHandler(&stubUserService{}, &stubCompanyService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.HandleGetProfile(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestHandler_HandleGetProfile_UserNotFound(t *testing.T) {
	uSvc := &stubUserService{
		getUserFn: func(_ context.Context, _ string) (*user.Record, error) {
			return nil, user.ErrUserNotFound
		},
	}
	h := NewHandler(uSvc, &stubCompanyService{})

	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil), "user-1", "")
	w := httptest.NewRecorder()
	h.HandleGetProfile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestHandler_HandleGetProfile_UserNilWithoutError(t *testing.T) {
	uSvc := &stubUserService{
		getUserFn: func(_ context.Context, _ string) (*user.Record, error) {
			return nil, nil
		},
	}
	h := NewHandler(uSvc, &stubCompanyService{})

	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil), "user-1", "")
	w := httptest.NewRecorder()
	h.HandleGetProfile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestHandler_HandleGetProfile_Success(t *testing.T) {
	uRecord := &user.Record{
		ID:          "user-1",
		Email:       "user@example.com",
		PhoneNumber: "123456",
		OUHandle:    "ou-handle-1",
		Data:        []byte(`{}`),
	}
	cRecord := &company.Record{
		ID:       "company-1",
		Name:     "Test Company",
		OUHandle: "ou-handle-1",
		HasCHA:   true,
	}

	uSvc := &stubUserService{
		getUserFn: func(_ context.Context, _ string) (*user.Record, error) { return uRecord, nil },
	}
	cSvc := &stubCompanyService{
		getCompanyByOUHandleFn: func(_ context.Context, ouHandle string) (*company.Record, error) {
			if ouHandle != "ou-handle-1" {
				t.Errorf("expected ouHandle 'ou-handle-1', got '%s'", ouHandle)
			}
			return cRecord, nil
		},
	}
	h := NewHandler(uSvc, cSvc)

	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil), "user-1", "ou-handle-1")
	w := httptest.NewRecorder()
	h.HandleGetProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp UserProfile
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != "user-1" {
		t.Errorf("expected id 'user-1', got %s", resp.ID)
	}
	if resp.Company == nil || resp.Company.ID != "company-1" {
		t.Errorf("expected company id 'company-1', got %+v", resp.Company)
	}
}

func TestHandler_HandleGetProfile_CompanyNotFound(t *testing.T) {
	uRecord := &user.Record{ID: "user-1", OUHandle: "non-existent-ou"}

	uSvc := &stubUserService{
		getUserFn: func(_ context.Context, _ string) (*user.Record, error) { return uRecord, nil },
	}
	cSvc := &stubCompanyService{
		getCompanyByOUHandleFn: func(_ context.Context, _ string) (*company.Record, error) {
			return nil, company.ErrCompanyNotFound
		},
	}
	h := NewHandler(uSvc, cSvc)

	req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil), "user-1", "non-existent-ou")
	w := httptest.NewRecorder()
	h.HandleGetProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK even if company not found, got %d", w.Code)
	}

	var resp UserProfile
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Company != nil {
		t.Errorf("expected company to be nil, got %+v", resp.Company)
	}
}
