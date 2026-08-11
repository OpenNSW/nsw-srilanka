package bootstrap

import (
	"context"

	nswauthn "github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

// authUserProfileAdapter adapts user.Service to nswauthn.UserProfileService,
// spreading the authenticated principal's identity fields across the named
// parameters user.Service already takes. Keeps internal/profile/user free of
// any dependency on the authentication layer's types.
type authUserProfileAdapter struct {
	svc user.Service
}

func (a *authUserProfileAdapter) GetOrCreateUser(ctx context.Context, p *nswauthn.Principal) (string, error) {
	return a.svc.GetOrCreateUser(ctx, p.IDPUserID, p.Email, p.PhoneNumber, p.OUID, p.OUHandle)
}
