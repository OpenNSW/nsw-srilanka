package bootstrap

import (
	"context"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

// authUserProfileAdapter adapts user.Service to authn.UserProfileService,
// translating authn's generic ExtraClaims into user.Service's existing
// named parameters. Keeps internal/profile/user free of any dependency on
// the auth library's types.
type authUserProfileAdapter struct {
	svc user.Service
}

func (a *authUserProfileAdapter) GetOrCreateUser(ctx context.Context, idpUserID string, extra authn.ExtraClaims) (string, error) {
	return a.svc.GetOrCreateUser(ctx, idpUserID,
		extra.String("email"), extra.String("phone_number"), extra.String("ouId"), extra.String("ouHandle"))
}
