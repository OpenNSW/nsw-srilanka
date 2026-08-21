package profile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/httputil"

	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

const errUnauthorized = "unauthorized"

// Handler exposes the authenticated caller's own profile.
type Handler struct {
	userSvc    user.Service
	companySvc company.Service
}

// NewHandler creates a new profile Handler.
func NewHandler(userSvc user.Service, companySvc company.Service) *Handler {
	return &Handler{
		userSvc:    userSvc,
		companySvc: companySvc,
	}
}

// UserProfile is the response shape for GET /api/v1/users/me. It trims the persisted
// user.Record down to caller-facing fields and nests the caller's company summary when
// the user is associated with one.
type UserProfile struct {
	ID          string           `json:"id"`
	Email       string           `json:"email"`
	PhoneNumber string           `json:"phoneNumber"`
	Data        json.RawMessage  `json:"data"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Company     *company.Summary `json:"company,omitempty"`
}

// HandleGetProfile handles GET /api/v1/users/me.
func (h *Handler) HandleGetProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil || authCtx.User == nil {
		httputil.Error(w, r, http.StatusUnauthorized, errUnauthorized)
		return
	}

	uRecord, err := h.userSvc.GetUser(ctx, authCtx.User.ID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			httputil.Error(w, r, http.StatusNotFound, "user profile not found")
			return
		}
		httputil.InternalServerError(w, r, "failed to retrieve user profile", err, "userId", authCtx.User.ID)
		return
	}
	if uRecord == nil {
		httputil.Error(w, r, http.StatusNotFound, "user profile not found")
		return
	}

	var companySummary *company.Summary
	if uRecord.OUHandle != "" {
		companyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		comp, err := h.companySvc.GetCompanyByOUHandle(companyCtx, uRecord.OUHandle)
		if err != nil {
			if !errors.Is(err, company.ErrCompanyNotFound) {
				httputil.InternalServerError(w, r, "failed to retrieve company profile", err, "ouHandle", uRecord.OUHandle)
				return
			}
		} else if comp != nil {
			companySummary = &company.Summary{ID: comp.ID, Name: comp.Name, HasCHA: comp.HasCHA}
		}
	}

	httputil.JSON(w, http.StatusOK, UserProfile{
		ID:          uRecord.ID,
		Email:       uRecord.Email,
		PhoneNumber: uRecord.PhoneNumber,
		Data:        uRecord.Data,
		CreatedAt:   uRecord.CreatedAt,
		UpdatedAt:   uRecord.UpdatedAt,
		Company:     companySummary,
	})
}
