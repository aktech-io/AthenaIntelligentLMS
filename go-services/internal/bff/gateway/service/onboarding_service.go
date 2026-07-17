// OnboardingService fronts the compliance-service A2 self-service eKYC
// onboarding API for the (not-yet-authenticated) mobile applicant, and the
// media-service upload path for KYC document/selfie captures.
//
// How this composes with the existing registration flow (no new
// account-creation flow is invented here):
//
//  1. App uploads ID photo + selfie   -> POST /api/v1/mobile/onboarding/media
//     (returns mediaRef per file)
//  2. App submits the application     -> POST /api/v1/mobile/onboarding
//     compliance-service runs the eKYC provider + risk tiering and answers
//     AUTO_APPROVED (low risk) or REFERRED (officer queue).
//  3. On nextStep=PROCEED_TO_REGISTRATION the app drives the EXISTING
//     registration flow: POST /api/v1/mobile/auth/otp/send (purpose
//     REGISTRATION) then /otp/verify, which creates the mobile user +
//     customerId and issues JWTs (see AuthService.VerifyOTP). Binding that
//     customerId back onto the onboarding application is the compliance
//     service's approval-side concern (SubmitOnboardingRequest.customerId /
//     officer decision), not re-implemented here.
//  4. On nextStep=AWAIT_REVIEW the app polls GET /api/v1/mobile/onboarding/{id}
//     until an officer decides.
package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// ComplianceAPI is the slice of the compliance client the service needs
// (interface so handler tests can fake the downstream).
type ComplianceAPI interface {
	SubmitOnboarding(ctx context.Context, body map[string]any) (map[string]any, error)
	GetOnboarding(ctx context.Context, id string) (map[string]any, error)
}

// MediaAPI is the slice of the media client the service needs.
type MediaAPI interface {
	Upload(ctx context.Context, up client.MediaUpload) (map[string]any, error)
}

type OnboardingService struct {
	compliance ComplianceAPI
	media      MediaAPI
}

func NewOnboardingService(compliance ComplianceAPI, media MediaAPI) *OnboardingService {
	return &OnboardingService{compliance: compliance, media: media}
}

// SubmitOnboardingRequest is the app-facing submission. tenantId follows the
// BFF's pre-auth tenant resolution convention (same as VerifyOTPRequest):
// taken from the body, defaulting to "default" — the applicant has no JWT yet.
type SubmitOnboardingRequest struct {
	TenantID    string  `json:"tenantId"`
	Phone       string  `json:"phone"`
	FullName    string  `json:"fullName"`
	NationalID  string  `json:"nationalId"`
	DateOfBirth *string `json:"dateOfBirth,omitempty"`
	DocumentRef *string `json:"documentRef,omitempty"`
	SelfieRef   *string `json:"selfieRef,omitempty"`
}

// Next-step hints the app can switch on after submit/poll.
const (
	// NextStepProceedToRegistration — application approved: drive the existing
	// OTP registration flow (/api/v1/mobile/auth/otp/send purpose=REGISTRATION).
	NextStepProceedToRegistration = "PROCEED_TO_REGISTRATION"
	// NextStepAwaitReview — referred to an officer: poll the status endpoint.
	NextStepAwaitReview = "AWAIT_REVIEW"
	// NextStepContactSupport — rejected: terminal, direct the user to support.
	NextStepContactSupport = "CONTACT_SUPPORT"
)

// OnboardingResponse wraps the compliance application with the app-facing
// next-step hint.
type OnboardingResponse struct {
	// Application is the compliance-service OnboardingApplication as-is
	// (id, status, riskTier, decisionReasons, ...).
	Application map[string]any `json:"application"`
	NextStep    string         `json:"nextStep"`
}

// Submit validates and forwards the application to compliance-service. The
// resolved tenant is stamped on ctx so ServiceKeyTransport propagates it as
// X-Service-Tenant.
func (s *OnboardingService) Submit(ctx context.Context, req SubmitOnboardingRequest) (*OnboardingResponse, error) {
	req.Phone = strings.TrimSpace(req.Phone)
	req.FullName = strings.TrimSpace(req.FullName)
	req.NationalID = strings.TrimSpace(req.NationalID)
	if req.Phone == "" || req.FullName == "" || req.NationalID == "" {
		return nil, apperrors.BadRequest("phone, fullName and nationalId are required")
	}

	body := map[string]any{
		"phone":      req.Phone,
		"fullName":   req.FullName,
		"nationalId": req.NationalID,
	}
	if req.DateOfBirth != nil {
		body["dateOfBirth"] = *req.DateOfBirth
	}
	if req.DocumentRef != nil {
		body["documentRef"] = *req.DocumentRef
	}
	if req.SelfieRef != nil {
		body["selfieRef"] = *req.SelfieRef
	}
	// Note: no customerId — the applicant has no account yet; it is bound on
	// the approval path once the user completes OTP registration.

	app, err := s.compliance.SubmitOnboarding(withTenant(ctx, req.TenantID), body)
	if err != nil {
		return nil, err
	}
	return wrapApplication(app), nil
}

// Get fetches an application for status polling.
func (s *OnboardingService) Get(ctx context.Context, id, tenantID string) (*OnboardingResponse, error) {
	app, err := s.compliance.GetOnboarding(withTenant(ctx, tenantID), id)
	if err != nil {
		return nil, err
	}
	return wrapApplication(app), nil
}

// allowedKYCMediaTypes are the media-service MediaType values accepted on the
// onboarding media path.
var allowedKYCMediaTypes = map[string]bool{
	"ID_FRONT":         true,
	"ID_BACK":          true,
	"PASSPORT":         true,
	"SELFIE":           true,
	"PROOF_OF_ADDRESS": true,
}

// MediaUploadResult is the app-facing upload response; MediaRef feeds the
// submission's documentRef/selfieRef.
type MediaUploadResult struct {
	MediaRef  string `json:"mediaRef"`
	MediaType string `json:"mediaType"`
	FileName  string `json:"fileName"`
}

// UploadMedia validates the KYC media type and streams the file to
// media-service under category CUSTOMER_DOCUMENT.
func (s *OnboardingService) UploadMedia(ctx context.Context, tenantID, mediaType, fileName, contentType string, file io.Reader) (*MediaUploadResult, error) {
	mediaType = strings.ToUpper(strings.TrimSpace(mediaType))
	if !allowedKYCMediaTypes[mediaType] {
		return nil, apperrors.BadRequest("mediaType must be one of ID_FRONT, ID_BACK, PASSPORT, SELFIE, PROOF_OF_ADDRESS")
	}

	media, err := s.media.Upload(withTenant(ctx, tenantID), client.MediaUpload{
		FileName:    fileName,
		ContentType: contentType,
		MediaType:   mediaType,
		Category:    "CUSTOMER_DOCUMENT",
		File:        file,
	})
	if err != nil {
		return nil, err
	}
	id, _ := media["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("media service response missing id")
	}
	return &MediaUploadResult{MediaRef: id, MediaType: mediaType, FileName: fileName}, nil
}

// wrapApplication adds the nextStep hint derived from the compliance status.
func wrapApplication(app map[string]any) *OnboardingResponse {
	status, _ := app["status"].(string)
	next := NextStepAwaitReview // RECEIVED / REFERRED / unknown: keep polling
	switch status {
	case "AUTO_APPROVED", "APPROVED":
		next = NextStepProceedToRegistration
	case "REJECTED":
		next = NextStepContactSupport
	}
	return &OnboardingResponse{Application: app, NextStep: next}
}

// withTenant resolves the pre-auth tenant (body/query value, defaulting to
// "default") into the context so ServiceKeyTransport stamps X-Service-Tenant.
func withTenant(ctx context.Context, tenantID string) context.Context {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	return auth.WithTenantID(ctx, tenantID)
}
