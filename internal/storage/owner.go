package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/OpenNSW/core/remote"
)

// maxPresignedURLLength bounds a presigned URL a service returns. A real one is
// a few hundred characters.
const maxPresignedURLLength = 4096

// ServiceCaller calls a service in the outbound services registry;
// *remote.Manager satisfies it.
type ServiceCaller interface {
	CallRaw(ctx context.Context, serviceID string, req remote.Request) (*remote.RawResponse, error)
}

// upstreamFailure marks an error caused by the owning service or the route to
// it. HTTP handlers answer it as a gateway failure.
type upstreamFailure struct {
	err error
}

func (e *upstreamFailure) Error() string { return e.err.Error() }

func (e *upstreamFailure) Unwrap() error { return e.err }

func wrapUpstreamFailure(op string, err error) *upstreamFailure {
	return &upstreamFailure{err: fmt.Errorf("storage: %s: %w", op, err)}
}

// checkPresignedURL rejects a download or upload URL a service returned that is
// not safe to hand to a client: it must be absolute, http or https, name a
// host, carry no credentials and be of sane length. A relative URL would
// resolve against this service instead of the owner, and another scheme (a
// `javascript:` URL, say) runs in the page that opens it.
func checkPresignedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxPresignedURLLength || !u.IsAbs() || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("storage: the owning service returned a URL that is not an absolute http(s) URL without credentials; its storage public URL must be")
	}
	return nil
}
