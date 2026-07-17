// Package service implements the per-tenant regulatory profile business logic:
// seeding sensible defaults on first access and applying validated partial
// updates with an audited before/after trail. It owns no rates — the profile
// only points at which rule-set/bureau applies (see package model).
package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/regulatory/model"
)

// ErrValidation wraps a client-correctable validation failure (handler → 400).
var ErrValidation = errors.New("validation error")

func validationf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrValidation}, args...)...)
}

// Repository is the data-access surface the service needs (satisfied by
// *repository.Repository). Declared here so the service is unit-testable with a
// fake.
type Repository interface {
	GetActiveProfile(ctx context.Context, tenantID string) (*model.RegulatoryProfile, error)
	CreateProfile(ctx context.Context, p *model.RegulatoryProfile) (*model.RegulatoryProfile, error)
	UpdateProfile(ctx context.Context, p *model.RegulatoryProfile) (*model.RegulatoryProfile, error)
}

// Service is the regulatory-profile business logic.
type Service struct {
	repo   Repository
	audit  *audit.Logger
	logger *zap.Logger
}

// New builds a Service. auditIns is the audit sink (the repository); a nil sink
// yields a no-op audit logger (safe for tests).
func New(repo Repository, auditIns audit.Inserter, logger *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit.New(auditIns, logger), logger: logger}
}

// GetOrCreateForTenant returns the tenant's active profile, seeding a default
// DCP profile on first access. Concurrency-safe: if a racing request creates the
// profile first (partial unique index conflict), the create error is resolved by
// re-reading.
func (s *Service) GetOrCreateForTenant(ctx context.Context, tenantID string) (*model.RegulatoryProfile, error) {
	if tenantID == "" {
		return nil, validationf("tenant is required")
	}
	p, err := s.repo.GetActiveProfile(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	created, err := s.repo.CreateProfile(ctx, defaultProfile(tenantID))
	if err != nil {
		// Likely a concurrent create won the unique index — re-read and use it.
		if existing, gerr := s.repo.GetActiveProfile(ctx, tenantID); gerr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	s.audit.Record(ctx, "REGULATORY_PROFILE_CREATE", "REGULATORY_PROFILE", created.ID, nil, created, nil)
	return created, nil
}

func defaultProfile(tenantID string) *model.RegulatoryProfile {
	// Seed defaults come from the active market pack; fall back to the
	// legacy Kenya constants if the pack omits a value.
	pack := market.Current()
	provisioningKey := pack.Regulatory.ProvisioningKey
	if !model.ValidProvisioningKey(provisioningKey) {
		provisioningKey = model.DefaultProvisioningKey
	}
	reportingCurrency := pack.Regulatory.ReportingCurrency
	if reportingCurrency == "" {
		reportingCurrency = pack.Currency
	}
	licenseType := model.LicenseType(pack.Regulatory.DefaultLicenseType)
	if !model.ValidLicenseType(string(licenseType)) {
		licenseType = model.LicenseDCP
	}
	return &model.RegulatoryProfile{
		TenantID:               tenantID,
		LicenseType:            licenseType,
		Country:                pack.Code,
		ReportingCurrency:      reportingCurrency,
		ProvisioningTableKey:   provisioningKey,
		CrbEnabled:             false,
		CrbSubmissionFrequency: model.FrequencyMonthly,
		ReportSet:              model.DefaultReportSetFor(licenseType),
		Active:                 true,
	}
}

// UpdateForTenant applies a validated partial update to the tenant's active
// profile and records an audited before/after entry. actor (the user id) may be
// empty. Only the supplied (non-nil) request fields are changed.
func (s *Service) UpdateForTenant(ctx context.Context, tenantID, actor string, req *model.UpdateProfileRequest) (*model.RegulatoryProfile, error) {
	current, err := s.GetOrCreateForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	before := *current

	if req.LicenseType != nil {
		if !model.ValidLicenseType(string(*req.LicenseType)) {
			return nil, validationf("unknown licenseType %q", *req.LicenseType)
		}
		current.LicenseType = *req.LicenseType
	}
	if req.Country != nil {
		if len(*req.Country) != 2 {
			return nil, validationf("country must be ISO 3166-1 alpha-2")
		}
		current.Country = *req.Country
	}
	if req.ReportingCurrency != nil {
		if len(*req.ReportingCurrency) != 3 {
			return nil, validationf("reportingCurrency must be ISO 4217")
		}
		current.ReportingCurrency = *req.ReportingCurrency
	}
	if req.ProvisioningTableKey != nil {
		if !model.ValidProvisioningKey(*req.ProvisioningTableKey) {
			return nil, validationf("unknown provisioningTableKey %q", *req.ProvisioningTableKey)
		}
		current.ProvisioningTableKey = *req.ProvisioningTableKey
	}
	if req.CrbEnabled != nil {
		current.CrbEnabled = *req.CrbEnabled
	}
	if req.CrbBureau != nil {
		switch {
		case *req.CrbBureau == "":
			current.CrbBureau = nil // explicit unset
		case !model.ValidCrbBureau(string(*req.CrbBureau)):
			return nil, validationf("unknown crbBureau %q", *req.CrbBureau)
		default:
			b := *req.CrbBureau
			current.CrbBureau = &b
		}
	}
	if req.CrbSubmissionFrequency != nil {
		if !model.ValidSubmissionFrequency(string(*req.CrbSubmissionFrequency)) {
			return nil, validationf("unknown crbSubmissionFrequency %q", *req.CrbSubmissionFrequency)
		}
		current.CrbSubmissionFrequency = *req.CrbSubmissionFrequency
	}
	if req.ReportSet != nil {
		for _, c := range *req.ReportSet {
			if !model.ValidReportCode(string(c)) {
				return nil, validationf("unknown report code %q", c)
			}
		}
		current.ReportSet = *req.ReportSet
	}
	if req.Notes != nil {
		current.Notes = req.Notes
	}

	// Consistency guard: a CRB feed cannot be enabled without a target bureau.
	if current.CrbEnabled && current.CrbBureau == nil {
		return nil, validationf("crbBureau is required when crbEnabled is true")
	}

	if actor != "" {
		current.UpdatedBy = &actor
	}
	updated, err := s.repo.UpdateProfile(ctx, current)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("active regulatory profile not found during update for tenant %s", tenantID)
	}
	s.audit.Record(ctx, "REGULATORY_PROFILE_UPDATE", "REGULATORY_PROFILE", updated.ID, &before, updated, nil)
	return updated, nil
}
