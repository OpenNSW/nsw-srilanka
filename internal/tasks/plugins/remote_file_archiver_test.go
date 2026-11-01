package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFetcher answers every download with one response, and records the request.
type fakeFetcher struct {
	resp      *remote.RawResponse
	err       error
	calls     int
	serviceID string
	req       remote.Request
}

func (f *fakeFetcher) CallRaw(_ context.Context, serviceID string, req remote.Request) (*remote.RawResponse, error) {
	f.calls++
	f.serviceID, f.req = serviceID, req
	return f.resp, f.err
}

func pdfResponse(status int, body string) *remote.RawResponse {
	return &remote.RawResponse{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/pdf; charset=binary"}},
		Body:       []byte(body),
	}
}

const archiveConfig = `{"service_id": "slpa", "filename": "gate-pass"}`

func archiveCtx(source any) plugins.PluginContext {
	inputs := map[string]any{}
	if source != nil {
		inputs["source"] = source
	}
	return plugins.PluginContext{
		Context:         context.Background(),
		Inputs:          inputs,
		Record:          &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		OutputNamespace: "gate_pass_file",
	}
}

func TestRemoteFileArchiver_DownloadsAndStoresAURL(t *testing.T) {
	fetcher := &fakeFetcher{resp: pdfResponse(http.StatusOK, "%PDF-1.7 pass")}
	files := &fakeSaver{}
	p := NewRemoteFileArchiverPlugin(fetcher, files)
	ctx := archiveCtx("https://slpacargoapi.slpa.lk/pdf/gate-pass/GP1?expires=1&signature=abc")

	require.NoError(t, p.Execute(ctx, json.RawMessage(archiveConfig)))

	assert.Equal(t, "slpa", fetcher.serviceID, "the download goes through the service's own client")
	assert.Equal(t, http.MethodGet, fetcher.req.Method)
	assert.Equal(t, "https://slpacargoapi.slpa.lk/pdf/gate-pass/GP1?expires=1&signature=abc", fetcher.req.Path,
		"the signed link is fetched as sent")
	assert.Equal(t, "%PDF-1.7 pass", string(files.content))
	assert.Equal(t, "gate-pass.pdf", files.filename, "the stored type's extension is added")
	assert.Equal(t, "application/pdf", files.mime, "parameters are dropped from the stored type")
	assert.Equal(t, map[string]any{
		"key":       "0f8e7c1a-0000-4000-8000-000000000001.html",
		"name":      "gate-pass.pdf",
		"mime_type": "application/pdf",
		"size":      int64(len("%PDF-1.7 pass")),
	}, ctx.Record.Data["gate_pass_file"])
}

func TestRemoteFileArchiver_DecodesAndStoresADataURI(t *testing.T) {
	fetcher := &fakeFetcher{}
	files := &fakeSaver{}
	p := NewRemoteFileArchiverPlugin(fetcher, files)
	ctx := archiveCtx("data:image/png;base64,iVBORw0KGgo=")

	require.NoError(t, p.Execute(ctx, json.RawMessage(`{"service_id": "slpa", "filename": "barcode"}`)))

	assert.Zero(t, fetcher.calls, "an inline document is not downloaded")
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, files.content)
	assert.Equal(t, "barcode.png", files.filename)
	assert.Equal(t, "image/png", files.mime)
	assert.Contains(t, ctx.Record.Data["gate_pass_file"], "key")
}

func TestRemoteFileArchiver_StoresNothingWithoutADocument(t *testing.T) {
	for name, source := range map[string]any{
		"no source":    nil,
		"empty":        "  ",
		"a plain code": "GP2026000123CONFCL001",
		"not a string": 42,
	} {
		t.Run(name, func(t *testing.T) {
			fetcher, files := &fakeFetcher{}, &fakeSaver{}
			ctx := archiveCtx(source)

			require.NoError(t, NewRemoteFileArchiverPlugin(fetcher, files).Execute(ctx, json.RawMessage(archiveConfig)))

			assert.Zero(t, fetcher.calls)
			assert.Zero(t, files.calls)
			assert.NotContains(t, ctx.Record.Data, "gate_pass_file", "the page keeps what the service sent")
		})
	}
}

func TestRemoteFileArchiver_RecordsARefusedSourceAndGoesOn(t *testing.T) {
	for name, tc := range map[string]struct {
		source string
		resp   *remote.RawResponse
		want   string
	}{
		"an expired link": {"https://slpa.example/pdf/x", pdfResponse(http.StatusForbidden, "expired"), "slpa refused the download with status 403"},
		"an empty answer": {"https://slpa.example/pdf/x", pdfResponse(http.StatusOK, ""), "slpa sent an empty document"},
		"not base64":      {"data:image/png,abc", nil, "the data: URI is not base64"},
		"bad base64":      {"data:image/png;base64,***", nil, "the data: URI does not decode"},
	} {
		t.Run(name, func(t *testing.T) {
			files := &fakeSaver{}
			ctx := archiveCtx(tc.source)

			err := NewRemoteFileArchiverPlugin(&fakeFetcher{resp: tc.resp}, files).Execute(ctx, json.RawMessage(archiveConfig))

			require.NoError(t, err, "the trader still has the service's own link, and a retry would be refused again")
			assert.Zero(t, files.calls)
			out, _ := ctx.Record.Data["gate_pass_file"].(map[string]any)
			assert.Contains(t, out["error"], tc.want)
			assert.NotContains(t, out, "key")
		})
	}
}

func TestRemoteFileArchiver_ReturnsAFailureARetryCanFix(t *testing.T) {
	for name, fetcher := range map[string]*fakeFetcher{
		"transport": {err: errors.New("connection reset")},
		"5xx":       {resp: pdfResponse(http.StatusBadGateway, "")},
	} {
		t.Run(name, func(t *testing.T) {
			files := &fakeSaver{}
			ctx := archiveCtx("https://slpa.example/pdf/x")

			err := NewRemoteFileArchiverPlugin(fetcher, files).Execute(ctx, json.RawMessage(archiveConfig))

			require.Error(t, err)
			assert.Zero(t, files.calls)
			assert.NotContains(t, ctx.Record.Data, "gate_pass_file")
		})
	}
}

func TestRemoteFileArchiver_ReturnsAStorageFailure(t *testing.T) {
	files := &fakeSaver{err: errors.New("bucket unavailable")}
	ctx := archiveCtx("https://slpa.example/pdf/x")

	err := NewRemoteFileArchiverPlugin(&fakeFetcher{resp: pdfResponse(http.StatusOK, "%PDF")}, files).Execute(ctx, json.RawMessage(archiveConfig))

	require.ErrorContains(t, err, "bucket unavailable")
	assert.NotContains(t, ctx.Record.Data, "gate_pass_file")
}

func TestRemoteFileArchiver_RequiresItsConfig(t *testing.T) {
	p := NewRemoteFileArchiverPlugin(&fakeFetcher{}, &fakeSaver{})
	for name, cfg := range map[string]string{
		"service_id": `{"filename": "x"}`,
		"filename":   `{"service_id": "slpa"}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, p.Execute(archiveCtx("https://slpa.example/x"), json.RawMessage(cfg)), name)
		})
	}

	ctx := archiveCtx("https://slpa.example/x")
	ctx.OutputNamespace = ""
	require.ErrorContains(t, p.Execute(ctx, json.RawMessage(archiveConfig)), "output_namespace")
}

func TestArchiveFilename(t *testing.T) {
	assert.Equal(t, "receipt.pdf", archiveFilename("receipt", "application/pdf"))
	assert.Equal(t, "receipt.pdf", archiveFilename("receipt.pdf", "image/png"), "a name with an extension is kept")
	assert.Equal(t, "barcode.jpg", archiveFilename("barcode", "image/jpeg"))
	assert.Equal(t, "blob", archiveFilename("blob", "application/x-unknown-type"))
}
