package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/OpenNSW/core/remote"
	corestorage "github.com/OpenNSW/core/storage"

	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileref"
)

// maxOwnerResponseBytes caps the answer of an owning service's download route,
// which is a few hundred bytes of JSON.
const maxOwnerResponseBytes = 64 << 10

// maxFetchBytes caps the bytes a server-side read takes from an owner's
// storage. Readers apply tighter limits of their own.
const maxFetchBytes = 64 << 20

var extensionPattern = regexp.MustCompile(`^\.[A-Za-z0-9]{1,16}$`)

// Files reads the files a stored value stands for, wherever they are held.
// A value naming this deployment (a bare key) is read from its own Service. A
// value naming another service (see fileref) is read through that service: its
// storage API gives a download URL, which a browser opens or, for a server-side
// reader, Download fetches.
//
// Only the peers it was given are reached. The owner is trusted to hold the
// file, and to nothing else: what it answers is checked before it is passed on,
// and none of its text is.
type Files struct {
	own    Service
	caller ServiceCaller
	peers  map[string]string
	inline map[string]bool
	fetch  *http.Client
}

// NewFiles returns the Files of a deployment that stores its own files in own.
// peers are the services whose files it may open, each with the base URL it is
// registered under in caller's registry, and inline are the media types a
// download of a file another service owns may open in the browser. Both may be
// empty, in which case every file is this deployment's own.
func NewFiles(own Service, caller ServiceCaller, peers map[string]string, inline []string) *Files {
	types := make(map[string]bool, len(inline))
	for _, t := range inline {
		types[t] = true
	}
	return &Files{
		own:    own,
		caller: caller,
		peers:  peers,
		inline: types,
		fetch:  newFetchClient(peers),
	}
}

// locate describes the download of the file value stands for.
func (f *Files) locate(ctx context.Context, value string) (fileDownload, error) {
	ref := fileref.Parse(value)
	if ref.IsOwn() {
		return downloadOf(ctx, f.own, ref.Value)
	}
	return f.ownerDownload(ctx, ref)
}

// Download streams the file value stands for and returns its MIME type. It is
// what a server-side step that reads files uses. Nothing it returns names the
// file: a reader must not make a file name from value.
func (f *Files) Download(ctx context.Context, value string) (io.ReadCloser, string, error) {
	ref := fileref.Parse(value)
	if ref.IsOwn() {
		return f.own.Download(ctx, ref.Value)
	}
	dl, err := f.ownerDownload(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	body, contentType, err := f.fetchFile(ctx, dl.DownloadURL)
	if err != nil {
		slog.ErrorContext(ctx, "storage: could not read a file another service owns", "service", ref.Namespace, "error", err)
		return nil, "", err
	}
	if dl.MimeType != "" {
		contentType = dl.MimeType
	}
	return body, contentType, nil
}

// ownerDownload asks the service that owns ref for its download. Every failure
// is the owner's or the route to it, and is answered alike: not found when the
// owner says so, otherwise a gateway failure. The owner's text is never relayed.
func (f *Files) ownerDownload(ctx context.Context, ref fileref.Ref) (fileDownload, error) {
	if _, ok := f.peers[ref.Namespace]; !ok || f.caller == nil {
		slog.ErrorContext(ctx, "storage: a stored value names a service that is not a storage peer", "service", ref.Namespace)
		return fileDownload{}, wrapUpstreamFailure("get download URL", errors.New("not a storage peer"))
	}
	resp, err := f.caller.CallRaw(ctx, ref.Namespace, remote.Request{
		Method: http.MethodGet,
		Path:   RoutePrefix + "/" + url.PathEscape(ref.Value),
	})
	if err != nil {
		slog.ErrorContext(ctx, "storage: the service that owns a file could not be reached", "service", ref.Namespace, "cause", transportCause(err))
		return fileDownload{}, wrapUpstreamFailure("get download URL", errors.New("owning service unreachable"))
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fileDownload{}, fmt.Errorf("%w: the owning service has no such file", corestorage.ErrNotFound)
	case resp.StatusCode != http.StatusOK:
		slog.ErrorContext(ctx, "storage: the service that owns a file refused or failed the request", "service", ref.Namespace, "status", resp.StatusCode)
		return fileDownload{}, wrapUpstreamFailure("get download URL", fmt.Errorf("owning service answered %d", resp.StatusCode))
	}

	dl, err := f.checkOwnerAnswer(resp.Body)
	if err != nil {
		slog.ErrorContext(ctx, "storage: the service that owns a file gave an unusable answer", "service", ref.Namespace, "error", err)
		return fileDownload{}, wrapUpstreamFailure("get download URL", errors.New("owning service gave an unusable answer"))
	}
	return dl, nil
}

// checkOwnerAnswer validates the body of an owner's download response. The URL
// must be safe to hand to a browser. The extension and type are hints and are
// dropped when malformed. How the file opens is decided here, from the media
// types this deployment lets open in a browser, not by the owner.
func (f *Files) checkOwnerAnswer(body []byte) (fileDownload, error) {
	if len(body) > maxOwnerResponseBytes {
		return fileDownload{}, errors.New("answer too large")
	}
	var answer struct {
		DownloadURL string `json:"download_url"`
		ExpiresAt   int64  `json:"expires_at"`
		Extension   string `json:"extension"`
		MimeType    string `json:"mime_type"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return fileDownload{}, errors.New("answer is not JSON")
	}
	if err := checkPresignedURL(answer.DownloadURL); err != nil {
		return fileDownload{}, err
	}
	dl := fileDownload{DownloadURL: answer.DownloadURL, ExpiresAt: answer.ExpiresAt}
	if extensionPattern.MatchString(answer.Extension) {
		dl.Extension = answer.Extension
	}
	if t := bareMediaType(answer.MimeType); t != "" {
		dl.MimeType = t
	}
	dl.Disposition = "attachment"
	if f.inline[dl.MimeType] {
		dl.Disposition = "inline"
	}
	return dl, nil
}

// bareMediaType returns s when it is a lower-case type/subtype with no
// parameters, and "" otherwise.
func bareMediaType(s string) string {
	mediaType, params, err := mime.ParseMediaType(s)
	if err != nil || len(params) > 0 || mediaType != s || !strings.Contains(s, "/") {
		return ""
	}
	return s
}

// transportCause names why a call failed without repeating its error, which
// carries the request URL and so the token.
func transportCause(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "timeout"
	default:
		return "connection or protocol failure"
	}
}
