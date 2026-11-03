package asycuda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type auditDeclarationRepo struct {
	cusdec.DeclarationRepository
	decl    *cusdec.CusdecDeclaration
	err     error
	lookups []string
}

func (r *auditDeclarationRepo) GetByEdgeID(_ context.Context, id string) (*cusdec.CusdecDeclaration, error) {
	r.lookups = append(r.lookups, id)
	return r.decl, r.err
}

func (r *auditDeclarationRepo) GetByCusdecRef(_ context.Context, ref cusdec.DocumentReference) (*cusdec.CusdecDeclaration, error) {
	r.lookups = append(r.lookups, ref.String())
	return r.decl, r.err
}

type auditDispatchNoteRepo struct {
	cdn.DispatchNoteRepository
	note    *cdn.DispatchNote
	err     error
	lookups []string
}

func (r *auditDispatchNoteRepo) GetByEdgeID(_ context.Context, id string) (*cdn.DispatchNote, error) {
	r.lookups = append(r.lookups, id)
	return r.note, r.err
}

func (r *auditDispatchNoteRepo) GetByCDNRef(_ context.Context, ref cdn.DocumentReference) (*cdn.DispatchNote, error) {
	r.lookups = append(r.lookups, ref.String())
	return r.note, r.err
}

const auditRef = `{"year":"2026","office":"CBEX1","serial":"E","number":1047}`

func callbackAuditRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/webhooks/slce", strings.NewReader(body))
	ctx := authn.ContextWithPrincipal(r.Context(), &authn.Principal{Kind: authn.KindClient, ClientID: "customs-client"})
	return r.WithContext(trace.ContextWithTraceID(ctx, "callback-trace"))
}

// Every dispatched event audits one outcome, whether the service accepts it or fails.
func TestSLCEHandler_AuditOutcomes(t *testing.T) {
	for _, tc := range []struct {
		event, method, payload string
		cdn                    bool
	}{
		{"CUSDEC_INTEGRATED", "ProcessIntegrationResult", `{"edgeId":"edge-1","integrated":true,"totalAssessedAmount":1,"amountPayable":1,"cusDecRef":` + auditRef + `}`, false},
		{"PAYMENT_CONFIRMED", "ProcessEvent", `{"cusDecRef":` + auditRef + `}`, false},
		{"WARRANTING_COMPLETED", "ProcessEvent", `{"cusDecRef":` + auditRef + `}`, false},
		{"EXPORT_RELEASED", "ProcessEvent", `{"cusDecRef":` + auditRef + `}`, false},
		{"CUSDEC_AMENDED", "ProcessAmendment", `{"cusDecRef":` + auditRef + `,"cusDec":{"sensitive":"do-not-log"},"operationTime":"2026-10-14T05:07:38Z"}`, false},
		{"CUSDEC_CANCELLED", "ProcessCancellation", `{"cusDecRef":` + auditRef + `}`, false},
		{"CDN_INTEGRATED", "ProcessIntegrationResult", `{"edgeId":"edge-1","integrated":true,"cdnRef":` + auditRef + `}`, true},
		{"CDN_ACKNOWLEDGED", "ProcessAcknowledgment", `{"cdnRef":` + auditRef + `}`, true},
		{"CDN_AMENDED", "ProcessAmendment", `{"cdnRef":` + auditRef + `,"cdn":{"sensitive":"do-not-log"},"operationTime":"2026-10-14T05:07:38Z"}`, true},
		{"CDN_CANCELLED", "ProcessCancellation", `{"cdnRef":` + auditRef + `}`, true},
	} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%v", tc.event, fail), func(t *testing.T) {
				decls := &auditDeclarationRepo{decl: &cusdec.CusdecDeclaration{ID: "declaration-1", CaseID: "case-1"}}
				notes := &auditDispatchNoteRepo{note: &cdn.DispatchNote{ID: "note-1", CaseID: "case-1"}}
				cs, ns := new(mockCusdecService), new(mockCDNService)
				var serviceErr error
				if fail {
					serviceErr = errors.New("private service error")
				}
				if tc.cdn {
					ns.On(tc.method, mock.Anything, mock.Anything).Return(serviceErr).Once()
				} else {
					cs.On(tc.method, mock.Anything, mock.Anything).Return(serviceErr).Once()
				}
				client, capture := nswaudit.NewWithCapture()
				h := NewHandler(cs, ns).WithAuditor(client, decls, notes)
				body := fmt.Sprintf(`{"eventType":%q,"processedAt":"2026-07-23T11:00:00Z","payload":%s}`, tc.event, tc.payload)
				w := httptest.NewRecorder()
				h.HandleWebhook(w, callbackAuditRequest(body))
				wantHTTP, wantStatus := http.StatusOK, nswaudit.StatusSuccess
				if fail {
					wantHTTP, wantStatus = http.StatusInternalServerError, nswaudit.StatusFailure
				}
				require.Equal(t, wantHTTP, w.Code, w.Body.String())
				records := capture.Records()
				require.Len(t, records, 1)
				r := records[0]
				assert.Equal(t, nswaudit.EventCustoms, r.EventType)
				assert.Equal(t, nswaudit.ActionUpdate, r.Action)
				assert.Equal(t, wantStatus, r.Status)
				assert.Equal(t, nswaudit.ActorService, r.ActorType)
				assert.Equal(t, "customs-client", r.ActorID)
				assert.Equal(t, "callback-trace", r.TraceID)
				assert.Equal(t, tc.event, r.Metadata["eventType"])
				assert.Equal(t, "case-1", r.Metadata["caseId"])
				assert.Equal(t, wantHTTP, r.Metadata["httpStatus"])
				wantLookup := "CBEX1/2026/E/1047"
				if strings.HasSuffix(tc.event, "_INTEGRATED") {
					wantLookup = "edge-1"
				}
				if tc.cdn {
					assert.Equal(t, nswaudit.TargetDispatchNote, r.TargetType)
					assert.Equal(t, "note-1", r.TargetID)
					assert.Equal(t, []string{wantLookup}, notes.lookups)
					assert.Empty(t, decls.lookups)
				} else {
					assert.Equal(t, nswaudit.TargetDeclaration, r.TargetType)
					assert.Equal(t, "declaration-1", r.TargetID)
					assert.Equal(t, []string{wantLookup}, decls.lookups)
					assert.Empty(t, notes.lookups)
				}
				metadata, err := json.Marshal(r.Metadata)
				require.NoError(t, err)
				assert.NotContains(t, string(metadata), "do-not-log")
				assert.NotContains(t, string(metadata), "private service error")
				assert.Empty(t, r.Message)
				cs.AssertExpectations(t)
				ns.AssertExpectations(t)
			})
		}
	}
}

func TestSLCEHandler_AuditRejections(t *testing.T) {
	for _, body := range []string{
		`{broken`, `{"eventType":"UNKNOWN"}`, `{"eventType":"PAYMENT_CONFIRMED","payload":{}}`,
		`{"eventType":"PAYMENT_CONFIRMED","payload":{"cusDecRef":` + auditRef + `}}`,
		strings.Repeat("x", (1<<20)+1),
	} {
		client, capture := nswaudit.NewWithCapture()
		h := NewHandler(new(mockCusdecService), new(mockCDNService)).WithAuditor(client, nil, nil)
		w := httptest.NewRecorder()
		h.HandleWebhook(w, callbackAuditRequest(body))
		require.Equal(t, http.StatusBadRequest, w.Code)
		records := capture.Records()
		require.Len(t, records, 1)
		assert.Equal(t, nswaudit.StatusFailure, records[0].Status)
		assert.Equal(t, "customs-client", records[0].ActorID)
		assert.NotEmpty(t, records[0].Metadata["error_code"])
	}
}

func TestSLCEHandler_AuditLookupFailureDoesNotChangeResponse(t *testing.T) {
	for _, lookupErr := range []error{nil, errors.New("database unavailable")} {
		cs := new(mockCusdecService)
		cs.On("ProcessCancellation", mock.Anything, mock.Anything).Return(nil).Once()
		decls := &auditDeclarationRepo{err: lookupErr}
		client, capture := nswaudit.NewWithCapture()
		h := NewHandler(cs, new(mockCDNService)).WithAuditor(client, decls, nil)
		w := httptest.NewRecorder()
		h.HandleWebhook(w, callbackAuditRequest(cancelledPayload))
		require.Equal(t, http.StatusOK, w.Code)
		records := capture.Records()
		require.Len(t, records, 1)
		assert.Equal(t, nswaudit.StatusSuccess, records[0].Status)
		assert.Empty(t, records[0].TargetID)
		assert.NotEmpty(t, records[0].Metadata["targetResolutionError"])
		assert.Equal(t, "CBEX1/2026/E/1047", records[0].Metadata["documentRef"])
	}
}

func TestSLCEHandler_AuditDuplicateAndCustomsRejection(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"duplicate", cusdec.ErrDuplicateIntegrationResult, http.StatusAccepted},
		{"customs rejection processed", nil, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := new(mockCusdecService)
			cs.On("ProcessIntegrationResult", mock.Anything, mock.Anything).Return(tc.err).Once()
			client, capture := nswaudit.NewWithCapture()
			h := NewHandler(cs, new(mockCDNService)).WithAuditor(client, nil, nil)
			w := httptest.NewRecorder()
			h.HandleWebhook(w, callbackAuditRequest(`{"eventType":"CUSDEC_INTEGRATED","processedAt":"2026-07-23T11:00:00Z","payload":{"edgeId":"edge-1","integrated":false,"errors":{"secret":"do-not-log"}}}`))
			require.Equal(t, tc.want, w.Code)
			records := capture.Records()
			require.Len(t, records, 1)
			assert.Equal(t, nswaudit.StatusSuccess, records[0].Status)
			assert.Equal(t, false, records[0].Metadata["integrated"])
			assert.Equal(t, "edge-1", records[0].Metadata["edgeId"])
			assert.NotContains(t, records[0].Metadata, "errors")
		})
	}
}

func TestSLCEHandler_AuditRequiresKnownCaller(t *testing.T) {
	client, capture := nswaudit.NewWithCapture()
	h := NewHandler(new(mockCusdecService), new(mockCDNService)).WithAuditor(client, nil, nil)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, httptest.NewRequest(http.MethodPost, "/webhooks/slce", strings.NewReader(`{broken`)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, capture.Records())
}

func TestSLCEHandler_AuditRetryableFailure(t *testing.T) {
	ns := new(mockCDNService)
	ns.On("ProcessCancellation", mock.Anything, mock.Anything).Return(cdn.ErrTaskNotParked).Once()
	notes := &auditDispatchNoteRepo{note: &cdn.DispatchNote{ID: "note-1", CaseID: "case-1"}}
	client, capture := nswaudit.NewWithCapture()
	h := NewHandler(new(mockCusdecService), ns).WithAuditor(client, nil, notes)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, callbackAuditRequest(cdnCancelledPayload))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), errTaskNotParkedRetry)
	records := capture.Records()
	require.Len(t, records, 1)
	assert.Equal(t, nswaudit.StatusFailure, records[0].Status)
	assert.Equal(t, "note-1", records[0].TargetID)
	assert.Equal(t, http.StatusServiceUnavailable, records[0].Metadata["httpStatus"])
	ns.AssertExpectations(t)
}

func TestSLCEHandler_AuditUnknownEventIsAllowlisted(t *testing.T) {
	client, capture := nswaudit.NewWithCapture()
	h := NewHandler(new(mockCusdecService), new(mockCDNService)).WithAuditor(client, nil, nil)
	unknown := strings.Repeat("untrusted-payload-", 100)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, callbackAuditRequest(fmt.Sprintf(`{"eventType":%q}`, unknown)))
	require.Equal(t, http.StatusBadRequest, w.Code)
	records := capture.Records()
	require.Len(t, records, 1)
	assert.Equal(t, "UNKNOWN", records[0].Metadata["eventType"])
	metadata, err := json.Marshal(records[0].Metadata)
	require.NoError(t, err)
	assert.NotContains(t, string(metadata), unknown)
}

func TestSLCEHandler_AuditEdgeIDBoundPreservesLookup(t *testing.T) {
	for _, length := range []int{maxAuditIDLen, maxAuditIDLen + 1} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			edgeID := strings.Repeat("e", length)
			cs := new(mockCusdecService)
			cs.On("ProcessIntegrationResult", mock.Anything, mock.MatchedBy(func(r cusdec.CusdecIntegrationResultRequest) bool {
				return r.EdgeID == edgeID
			})).Return(nil).Once()
			decls := &auditDeclarationRepo{decl: &cusdec.CusdecDeclaration{ID: "declaration-1"}}
			client, capture := nswaudit.NewWithCapture()
			h := NewHandler(cs, new(mockCDNService)).WithAuditor(client, decls, nil)
			w := httptest.NewRecorder()
			h.HandleWebhook(w, callbackAuditRequest(fmt.Sprintf(`{"eventType":" cusdec_integrated ","processedAt":"2026-07-23T11:00:00Z","payload":{"edgeId":%q,"integrated":false}}`, edgeID)))
			require.Equal(t, http.StatusOK, w.Code)
			records := capture.Records()
			require.Len(t, records, 1)
			assert.Equal(t, "CUSDEC_INTEGRATED", records[0].Metadata["eventType"])
			assert.Equal(t, "declaration-1", records[0].TargetID)
			assert.Equal(t, []string{edgeID}, decls.lookups)
			if length <= maxAuditIDLen {
				assert.Equal(t, edgeID, records[0].Metadata["edgeId"])
			} else {
				assert.NotContains(t, records[0].Metadata, "edgeId")
			}
			cs.AssertExpectations(t)
		})
	}
}

func TestAuditDocumentReferenceBound(t *testing.T) {
	for _, length := range []int{maxAuditIDLen, maxAuditIDLen + 1} {
		// The separators and the three single-character fields take six bytes.
		ref := document.Reference{Office: strings.Repeat("o", length-6), Year: "1", Serial: "E", Number: 1}
		require.Len(t, ref.String(), length)
		if length <= maxAuditIDLen {
			assert.Equal(t, ref.String(), auditDocumentReference(ref))
		} else {
			assert.Empty(t, auditDocumentReference(ref))
		}
	}
	assert.Empty(t, auditDocumentReference(document.Reference{}))
}
