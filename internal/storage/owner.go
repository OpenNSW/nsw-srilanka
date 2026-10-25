package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/OpenNSW/core/remote"
)

// maxPresignedURLLength bounds a presigned URL a service returns. A real one is
// a few hundred characters.
const maxPresignedURLLength = 4096

// ServiceCaller calls a service in the outbound services registry;
// *remote.Manager satisfies it.
type ServiceCaller interface {
	GetClient(id string) (*remote.Client, error)
	Call(ctx context.Context, serviceID string, req remote.Request, response any) error
	CallRaw(ctx context.Context, serviceID string, req remote.Request) (*remote.RawResponse, error)
}

// RemoteRejection is a client error (4xx) the owning service returned for a
// request, other than a rejection of this service's own credentials. It is
// the caller's request that was refused — an unsupported file type, say — so
// ProxyHandler relays it rather than reporting a gateway failure.
type RemoteRejection struct {
	StatusCode int
	// Message is the owning service's error message, when it sent one.
	Message string
	err     error
}

// upstreamFailure marks an error caused by the owning service or the route to
// it. HTTP handlers answer it as a gateway failure; a RemoteRejection wraps
// one when the owner returned a client error that should instead be relayed.
type upstreamFailure struct {
	err error
}

func (e *upstreamFailure) Error() string { return e.err.Error() }

func (e *upstreamFailure) Unwrap() error { return e.err }

func (e *RemoteRejection) Error() string {
	return e.err.Error()
}

func (e *RemoteRejection) Unwrap() error {
	return e.err
}

// remoteFailure wraps an error from the owning service's API, classifying a
// 4xx rejection of the request as a *RemoteRejection. remote reads the error
// body into the response before failing, which is where message comes from.
func remoteFailure(op string, err error, message string) error {
	wrapped := wrapUpstreamFailure(op, err)

	var re *remote.RemoteError
	if !errors.As(err, &re) || re.StatusCode < 400 || re.StatusCode > 499 {
		return wrapped
	}
	// 401/403 mean the owning service refused this service's credentials —
	// a misconfiguration here, not a problem with the caller's request.
	if re.StatusCode == http.StatusUnauthorized || re.StatusCode == http.StatusForbidden {
		return wrapped
	}
	return &RemoteRejection{StatusCode: re.StatusCode, Message: message, err: wrapped}
}

func wrapUpstreamFailure(op string, err error) *upstreamFailure {
	return &upstreamFailure{err: fmt.Errorf("storage proxy: %s: %w", op, err)}
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
