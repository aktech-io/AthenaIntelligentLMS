package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// fakeCompliance implements service.ComplianceAPI, capturing what the BFF
// forwards and returning canned responses/errors (the same typed errors the
// real ComplianceClient produces).
type fakeCompliance struct {
	submitResp map[string]any
	submitErr  error
	getResp    map[string]any
	getErr     error

	called    bool
	gotBody   map[string]any
	gotID     string
	gotTenant string
}

func (f *fakeCompliance) SubmitOnboarding(ctx context.Context, body map[string]any) (map[string]any, error) {
	f.called = true
	f.gotBody = body
	f.gotTenant = auth.TenantIDOrDefault(ctx)
	return f.submitResp, f.submitErr
}

func (f *fakeCompliance) GetOnboarding(ctx context.Context, id string) (map[string]any, error) {
	f.called = true
	f.gotID = id
	f.gotTenant = auth.TenantIDOrDefault(ctx)
	return f.getResp, f.getErr
}

// fakeMedia implements service.MediaAPI.
type fakeMedia struct {
	resp map[string]any
	err  error

	gotUpload  client.MediaUpload
	gotContent []byte
	gotTenant  string
}

func (f *fakeMedia) Upload(ctx context.Context, up client.MediaUpload) (map[string]any, error) {
	f.gotUpload = up
	f.gotContent, _ = io.ReadAll(up.File)
	f.gotTenant = auth.TenantIDOrDefault(ctx)
	return f.resp, f.err
}

func newOnboardingRouter(compliance *fakeCompliance, media *fakeMedia) chi.Router {
	r := chi.NewRouter()
	NewOnboardingHandler(service.NewOnboardingService(compliance, media)).Routes(r)
	return r
}

func doJSON(t *testing.T, r chi.Router, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var resp map[string]any
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	}
	return rec, resp
}

func TestSubmitAutoApproved(t *testing.T) {
	fc := &fakeCompliance{submitResp: map[string]any{
		"id":       "6a1f8b1e-0000-4000-8000-000000000001",
		"status":   "AUTO_APPROVED",
		"riskTier": "LOW",
	}}
	r := newOnboardingRouter(fc, &fakeMedia{})

	rec, resp := doJSON(t, r, http.MethodPost, "/api/v1/mobile/onboarding", map[string]any{
		"tenantId":    "acme",
		"phone":       "+254700000001",
		"fullName":    "Amina Njeri",
		"nationalId":  "12345678",
		"documentRef": "doc-ref-1",
		"selfieRef":   "selfie-ref-1",
	})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, service.NextStepProceedToRegistration, resp["nextStep"])
	app := resp["application"].(map[string]any)
	assert.Equal(t, "AUTO_APPROVED", app["status"])

	// The BFF forwards the applicant fields and stamps the tenant on context
	// (ServiceKeyTransport turns that into X-Service-Tenant downstream).
	assert.Equal(t, "acme", fc.gotTenant)
	assert.Equal(t, "+254700000001", fc.gotBody["phone"])
	assert.Equal(t, "Amina Njeri", fc.gotBody["fullName"])
	assert.Equal(t, "12345678", fc.gotBody["nationalId"])
	assert.Equal(t, "doc-ref-1", fc.gotBody["documentRef"])
	assert.Equal(t, "selfie-ref-1", fc.gotBody["selfieRef"])
	// The applicant has no account yet: the BFF must never invent a customerId.
	_, hasCustomer := fc.gotBody["customerId"]
	assert.False(t, hasCustomer)
}

func TestSubmitReferred(t *testing.T) {
	fc := &fakeCompliance{submitResp: map[string]any{
		"id":     "6a1f8b1e-0000-4000-8000-000000000002",
		"status": "REFERRED",
	}}
	r := newOnboardingRouter(fc, &fakeMedia{})

	rec, resp := doJSON(t, r, http.MethodPost, "/api/v1/mobile/onboarding", map[string]any{
		"phone":      "+254700000002",
		"fullName":   "Brian Otieno",
		"nationalId": "87654321",
	})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, service.NextStepAwaitReview, resp["nextStep"])
	// No tenantId in the body -> BFF default tenant.
	assert.Equal(t, "default", fc.gotTenant)
}

func TestSubmitValidationFailure(t *testing.T) {
	fc := &fakeCompliance{}
	r := newOnboardingRouter(fc, &fakeMedia{})

	// Missing nationalId (and blank phone must not pass as "present").
	rec, resp := doJSON(t, r, http.MethodPost, "/api/v1/mobile/onboarding", map[string]any{
		"phone":    "   ",
		"fullName": "No Id",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, resp["message"], "required")
	assert.False(t, fc.called, "compliance must not be called on validation failure")
}

func TestSubmitMalformedBody(t *testing.T) {
	r := newOnboardingRouter(&fakeCompliance{}, &fakeMedia{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/onboarding", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestSubmitComplianceDown locks the failure contract: a dead or erroring
// compliance service surfaces as 503/502 — never a 2xx, never a silent pass.
func TestSubmitComplianceDown(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unreachable -> 503", &apperrors.BusinessError{StatusCode: http.StatusServiceUnavailable, Message: "onboarding service is unavailable, please retry"}, http.StatusServiceUnavailable},
		{"downstream 5xx -> 502", &apperrors.BusinessError{StatusCode: http.StatusBadGateway, Message: "onboarding service error"}, http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeCompliance{submitErr: tc.err}
			r := newOnboardingRouter(fc, &fakeMedia{})
			rec, resp := doJSON(t, r, http.MethodPost, "/api/v1/mobile/onboarding", map[string]any{
				"phone":      "+254700000003",
				"fullName":   "Carol W",
				"nationalId": "11223344",
			})
			assert.Equal(t, tc.want, rec.Code)
			assert.NotEmpty(t, resp["message"])
			assert.Nil(t, resp["nextStep"], "no next step on failure")
		})
	}
}

func TestGetStatusPolling(t *testing.T) {
	fc := &fakeCompliance{getResp: map[string]any{
		"id":     "6a1f8b1e-0000-4000-8000-000000000004",
		"status": "REJECTED",
	}}
	r := newOnboardingRouter(fc, &fakeMedia{})

	rec, resp := doJSON(t, r,
		http.MethodGet, "/api/v1/mobile/onboarding/6a1f8b1e-0000-4000-8000-000000000004?tenantId=acme", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, service.NextStepContactSupport, resp["nextStep"])
	assert.Equal(t, "6a1f8b1e-0000-4000-8000-000000000004", fc.gotID)
	assert.Equal(t, "acme", fc.gotTenant)
}

func TestGetInvalidID(t *testing.T) {
	fc := &fakeCompliance{}
	r := newOnboardingRouter(fc, &fakeMedia{})
	rec, _ := doJSON(t, r, http.MethodGet, "/api/v1/mobile/onboarding/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, fc.called)
}

func TestGetNotFound(t *testing.T) {
	fc := &fakeCompliance{getErr: apperrors.NotFound("onboarding application not found")}
	r := newOnboardingRouter(fc, &fakeMedia{})
	rec, _ := doJSON(t, r, http.MethodGet, "/api/v1/mobile/onboarding/6a1f8b1e-0000-4000-8000-000000000005", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func multipartUpload(t *testing.T, fields map[string]string, fileName, content string) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	part, err := mw.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return buf, mw.FormDataContentType()
}

func TestUploadMedia(t *testing.T) {
	fm := &fakeMedia{resp: map[string]any{"id": "9f0e1d2c-0000-4000-8000-00000000000a"}}
	r := newOnboardingRouter(&fakeCompliance{}, fm)

	body, contentType := multipartUpload(t,
		map[string]string{"mediaType": "selfie", "tenantId": "acme"}, "selfie.jpg", "jpeg-bytes")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/onboarding/media", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "9f0e1d2c-0000-4000-8000-00000000000a", resp["mediaRef"])
	assert.Equal(t, "SELFIE", resp["mediaType"]) // normalised
	assert.Equal(t, "selfie.jpg", resp["fileName"])

	assert.Equal(t, "acme", fm.gotTenant)
	assert.Equal(t, "CUSTOMER_DOCUMENT", fm.gotUpload.Category)
	assert.Equal(t, "SELFIE", fm.gotUpload.MediaType)
	assert.Equal(t, "jpeg-bytes", string(fm.gotContent))
}

func TestUploadMediaInvalidType(t *testing.T) {
	fm := &fakeMedia{resp: map[string]any{"id": "x"}}
	r := newOnboardingRouter(&fakeCompliance{}, fm)

	body, contentType := multipartUpload(t, map[string]string{"mediaType": "CAT_PHOTO"}, "cat.jpg", "meow")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/onboarding/media", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadMediaMissingFile(t *testing.T) {
	r := newOnboardingRouter(&fakeCompliance{}, &fakeMedia{})
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	require.NoError(t, mw.WriteField("mediaType", "SELFIE"))
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/onboarding/media", buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadMediaServiceDown(t *testing.T) {
	fm := &fakeMedia{err: &apperrors.BusinessError{StatusCode: http.StatusServiceUnavailable, Message: "media service is unavailable, please retry"}}
	r := newOnboardingRouter(&fakeCompliance{}, fm)

	body, contentType := multipartUpload(t, map[string]string{"mediaType": "ID_FRONT"}, "id.jpg", "bytes")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/onboarding/media", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
