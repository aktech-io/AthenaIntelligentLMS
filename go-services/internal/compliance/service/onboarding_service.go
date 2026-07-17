package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/compliance/ekyc"
	"github.com/athena-lms/go-services/internal/compliance/model"
)

// faceMatchThreshold is the minimum document-vs-selfie match for
// auto-approval; below it a human looks (A2 risk tiering v1).
const faceMatchThreshold = 0.85

// OnboardingRepository is the data surface OnboardingService needs
// (satisfied by *repository.Repository). Interface here for unit tests,
// matching the regulatory/tenant service pattern.
type OnboardingRepository interface {
	CreateOnboarding(ctx context.Context, a *model.OnboardingApplication) (*model.OnboardingApplication, error)
	GetOnboardingByID(ctx context.Context, id uuid.UUID, tenantID string) (*model.OnboardingApplication, error)
	ListOnboarding(ctx context.Context, tenantID string, status *model.OnboardingStatus, page, size int) ([]model.OnboardingApplication, int64, error)
	UpdateOnboardingDecision(ctx context.Context, a *model.OnboardingApplication) error
	UpsertKyc(ctx context.Context, rec *model.KycRecord) (*model.KycRecord, error)
	CreateEvent(ctx context.Context, evt *model.ComplianceEvent) (*model.ComplianceEvent, error)
}

// OnboardingService implements the A2 self-service onboarding flow:
// verify with the configured eKYC provider, risk-tier the result, approve
// low-risk applications straight through and refer the rest.
type OnboardingService struct {
	repo     OnboardingRepository
	provider ekyc.Provider
	logger   *zap.Logger
}

// NewOnboarding builds an OnboardingService.
func NewOnboarding(repo OnboardingRepository, provider ekyc.Provider, logger *zap.Logger) *OnboardingService {
	return &OnboardingService{repo: repo, provider: provider, logger: logger}
}

// Submit runs one onboarding application end-to-end. A provider error is a
// REFERRED application, never an approval (fail-closed): the customer gets
// "in review" rather than a false decline or a silent pass.
func (s *OnboardingService) Submit(ctx context.Context, req model.SubmitOnboardingRequest, tenantID string) (*model.OnboardingApplication, error) {
	switch {
	case strings.TrimSpace(req.Phone) == "":
		return nil, errors.BadRequest("phone is required")
	case strings.TrimSpace(req.FullName) == "":
		return nil, errors.BadRequest("fullName is required")
	case strings.TrimSpace(req.NationalID) == "":
		return nil, errors.BadRequest("nationalId is required")
	}

	res, verr := s.provider.Verify(ctx, ekyc.Request{
		FullName:    req.FullName,
		NationalID:  req.NationalID,
		Phone:       req.Phone,
		DateOfBirth: deref(req.DateOfBirth),
		DocumentRef: deref(req.DocumentRef),
		SelfieRef:   deref(req.SelfieRef),
	})

	tier, status, reasons := tierDecision(res, verr)
	now := time.Now()
	providerName := s.provider.Name()
	reasonsStr := strings.Join(reasons, "; ")

	app := &model.OnboardingApplication{
		TenantID:        tenantID,
		Phone:           req.Phone,
		FullName:        req.FullName,
		NationalID:      req.NationalID,
		DateOfBirth:     req.DateOfBirth,
		DocumentRef:     req.DocumentRef,
		SelfieRef:       req.SelfieRef,
		Status:          status,
		RiskTier:        &tier,
		Provider:        &providerName,
		DecisionReasons: &reasonsStr,
	}
	if verr == nil {
		app.ProviderRef = &res.ProviderRef
	}
	if status == model.OnboardingAutoApproved {
		decider := "ekyc:" + providerName
		app.DecidedBy = &decider
		app.DecidedAt = &now
		app.CustomerID = req.CustomerID
	}

	app, err := s.repo.CreateOnboarding(ctx, app)
	if err != nil {
		if strings.Contains(err.Error(), "uq_onboarding_open") || strings.Contains(err.Error(), "23505") {
			return nil, errors.NewBusinessError("An open onboarding application already exists for this identity")
		}
		return nil, err
	}

	// Auto-approval materializes the KYC record with the provider as checker.
	if status == model.OnboardingAutoApproved {
		if err := s.passKycFor(ctx, app, "ekyc:"+providerName); err != nil {
			return nil, err
		}
	}

	s.logComplianceEvent(ctx, app, "onboarding."+strings.ToLower(string(status)))
	s.logger.Info("Onboarding application decided",
		zap.String("tenant", tenantID),
		zap.String("status", string(status)),
		zap.String("tier", string(tier)),
		zap.String("provider", providerName))
	return app, nil
}

// tierDecision maps a provider result to (tier, status, reasons) — the A2 v1
// policy. Screening hits and verification failures always refer to a human;
// v1 never auto-rejects.
func tierDecision(res ekyc.Result, verr error) (model.RiskTier, model.OnboardingStatus, []string) {
	if verr != nil {
		return model.TierHigh, model.OnboardingReferred, []string{"PROVIDER_ERROR: " + verr.Error()}
	}
	var reasons []string
	if res.SanctionsHit {
		reasons = append(reasons, "SANCTIONS_HIT")
	}
	if res.PEPHit {
		reasons = append(reasons, "PEP_HIT")
	}
	if len(reasons) > 0 {
		return model.TierHigh, model.OnboardingReferred, reasons
	}
	if !res.DocumentVerified {
		reasons = append(reasons, "DOCUMENT_UNVERIFIED")
	}
	if !res.LivenessPassed {
		reasons = append(reasons, "LIVENESS_FAILED")
	}
	if res.FaceMatchScore < faceMatchThreshold {
		reasons = append(reasons, fmt.Sprintf("FACE_MATCH_BELOW_THRESHOLD (%.2f < %.2f)", res.FaceMatchScore, faceMatchThreshold))
	}
	if len(reasons) > 0 {
		return model.TierMedium, model.OnboardingReferred, reasons
	}
	return model.TierLow, model.OnboardingAutoApproved, []string{"ALL_CHECKS_PASSED"}
}

// Get returns one application.
func (s *OnboardingService) Get(ctx context.Context, id uuid.UUID, tenantID string) (*model.OnboardingApplication, error) {
	app, err := s.repo.GetOnboardingByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, errors.NotFoundResource("Onboarding application", id)
	}
	return app, nil
}

// List pages applications, optionally filtered by status (the officer queue
// is List with status=REFERRED).
func (s *OnboardingService) List(ctx context.Context, tenantID string, status *model.OnboardingStatus, page, size int) (dto.PageResponse, error) {
	apps, total, err := s.repo.ListOnboarding(ctx, tenantID, status, page, size)
	if err != nil {
		return dto.PageResponse{}, err
	}
	return dto.NewPageResponse(apps, page, size, total), nil
}

// Decide applies an officer decision to a REFERRED application.
func (s *OnboardingService) Decide(ctx context.Context, id uuid.UUID, approve bool, req model.OnboardingDecisionRequest, tenantID, officer string) (*model.OnboardingApplication, error) {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if app.Status != model.OnboardingReferred {
		return nil, errors.NewBusinessError("Only REFERRED applications can be decided; application is " + string(app.Status))
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, errors.BadRequest("reason is required")
	}

	now := time.Now()
	reasons := deref(app.DecisionReasons) + "; OFFICER: " + req.Reason
	app.DecisionReasons = &reasons
	app.DecidedBy = &officer
	app.DecidedAt = &now
	if req.CustomerID != nil {
		app.CustomerID = req.CustomerID
	}
	if approve {
		app.Status = model.OnboardingApproved
	} else {
		app.Status = model.OnboardingRejected
	}

	if err := s.repo.UpdateOnboardingDecision(ctx, app); err != nil {
		return nil, err
	}
	if approve {
		if err := s.passKycFor(ctx, app, officer); err != nil {
			return nil, err
		}
	}
	s.logComplianceEvent(ctx, app, "onboarding."+strings.ToLower(string(app.Status)))
	return app, nil
}

// passKycFor writes the PASSED kyc_record for an approved application.
// Customer id falls back to the application id, so the KYC trail exists even
// before the customer record is materialized downstream.
func (s *OnboardingService) passKycFor(ctx context.Context, app *model.OnboardingApplication, checkedBy string) error {
	customerID := deref(app.CustomerID)
	if customerID == "" {
		customerID = "onboarding:" + app.ID.String()
	}
	now := time.Now()
	tier := model.RiskLevelLow
	if app.RiskTier != nil && *app.RiskTier != model.TierLow {
		tier = model.RiskLevelMedium
	}
	_, err := s.repo.UpsertKyc(ctx, &model.KycRecord{
		TenantID:   app.TenantID,
		CustomerID: customerID,
		Status:     model.KycStatusPassed,
		CheckType:  "EKYC_ONBOARDING",
		NationalID: &app.NationalID,
		FullName:   &app.FullName,
		Phone:      &app.Phone,
		RiskLevel:  tier,
		CheckedBy:  &checkedBy,
		CheckedAt:  &now,
	})
	if err != nil {
		return fmt.Errorf("materialize KYC record: %w", err)
	}
	return nil
}

func (s *OnboardingService) logComplianceEvent(ctx context.Context, app *model.OnboardingApplication, eventType string) {
	source := "compliance-service"
	subject := app.ID.String()
	payload := fmt.Sprintf(`{"riskTier":%q,"provider":%q}`, derefTier(app.RiskTier), deref(app.Provider))
	_, err := s.repo.CreateEvent(ctx, &model.ComplianceEvent{
		TenantID:      app.TenantID,
		EventType:     eventType,
		SourceService: &source,
		SubjectID:     &subject,
		Payload:       &payload,
	})
	if err != nil {
		// The decision stands even if the trail write fails; log loudly.
		s.logger.Error("Failed to log onboarding compliance event", zap.Error(err))
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefTier(t *model.RiskTier) string {
	if t == nil {
		return ""
	}
	return string(*t)
}
