package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

// fetchTimeout bounds fetching a presigned URL, including streaming its body.
const fetchTimeout = 60 * time.Second

// deniedPrefixes are address ranges a fetch never dials, beyond what netip
// classifies: carrier-grade NAT, benchmarking, reserved and "this network".
var deniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// newFetchClient returns the client a server-side read fetches a peer's
// presigned URL with. The URL comes from the peer, so the client sends no
// credentials, follows no redirect, and dials only public addresses over
// https. The one exception is the exact host and port a peer is registered
// under, which may be private and http: that is where its own storage is
// served in a deployment that keeps files on local disk.
func newFetchClient(peers map[string]string) *http.Client {
	trusted := make(map[string]bool, len(peers))
	for _, base := range peers {
		if u, err := url.Parse(base); err == nil && u.Host != "" {
			trusted[hostPort(u)] = true
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		// A proxy would be dialed in place of the target, which the address
		// check below could not see.
		Proxy:               nil,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if trusted[addr] {
				return dialer.DialContext(ctx, network, addr)
			}
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !publicAddress(ip) {
					return nil, errors.New("the host resolves to an address that is not public")
				}
			}
			if len(ips) == 0 {
				return nil, errors.New("the host has no address")
			}
			// Dial the address that was checked, not the name again.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &http.Client{
		Timeout:   fetchTimeout,
		Transport: &guardedTransport{next: transport, trusted: trusted},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// guardedTransport refuses the requests a fetch may not make before they are
// sent: anything but https, unless the host is a registered peer's own.
type guardedTransport struct {
	next    http.RoundTripper
	trusted map[string]bool
}

func (g *guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL
	insecureAllowed := u.Scheme == "http" && g.trusted[hostPort(u)]
	if u.User != nil || (u.Scheme != "https" && !insecureAllowed) {
		return nil, errors.New("storage: refused to fetch a URL that is not https")
	}
	return g.next.RoundTrip(req)
}

// hostPort returns u's host and port, with the scheme's default port when it
// names none.
func hostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// publicAddress reports whether ip is a unicast address on the public internet.
func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range deniedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// fetchFile gets the file at downloadURL, a presigned URL a peer gave, and
// returns its body, limited to maxFetchBytes, and its Content-Type. Every
// failure is the peer's or the route to it, and names neither the URL nor the
// peer's words.
func (f *Files) fetchFile(ctx context.Context, downloadURL string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", wrapUpstreamFailure("build download request", errors.New("bad URL"))
	}
	resp, err := f.fetch.Do(req)
	if err != nil {
		return nil, "", wrapUpstreamFailure("download", errors.New(transportCause(err)))
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, "", wrapUpstreamFailure("download", fmt.Errorf("owning service's storage answered %d", resp.StatusCode))
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return limitedBody{Reader: io.LimitReader(resp.Body, maxFetchBytes), Closer: resp.Body}, contentType, nil
}

type limitedBody struct {
	io.Reader
	io.Closer
}
