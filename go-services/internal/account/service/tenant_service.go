package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	errors2 "errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
	commonevent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/market"
)

// TenantRepository is the data-access surface the tenant service needs
// (satisfied by *repository.Repository). Declared here so the service is
// unit-testable with a fake.
type TenantRepository interface {
	GetTenant(ctx context.Context, id string) (*model.Tenant, error)
	ListTenants(ctx context.Context) ([]*model.Tenant, error)
	ProvisionTenant(ctx context.Context, t *model.Tenant, s *model.TenantSettings, u *model.User, passwordHash string, evt *commonevent.DomainEvent) error
	UpdateTenantStatus(ctx context.Context, id string, status model.TenantStatus, evt *commonevent.DomainEvent) error
	GetTenantBrand(ctx context.Context, id string) (raw []byte, found bool, err error)
	SetTenantBrand(ctx context.Context, id string, raw []byte) error
}

// TenantService implements tenant provisioning (Nemo gap C1): the "create
// neobank" API. One call registers a tenant, seeds its org settings from the
// market pack, creates its initial admin user (one-time password returned
// exactly once) and emits tenant.provisioned through the transactional outbox
// — all atomically.
//
// Downstream seeding is deliberately lazy, so no consumer is required for v1:
//   - regulatory: profile is seeded from the market pack on first access
//     (regulatory service GetOrCreateForTenant);
//   - accounting: GL postings resolve account codes against the shared
//     'system' chart of accounts (resolveAccountID falls back to tenant
//     "system"), so a new tenant books correctly with zero per-tenant seed
//     until it customises its chart.
//
// tenant.provisioned exists so future consumers (per-tenant GL charts, brand
// packs, product-catalogue seeding) can react without touching this flow.
type TenantService struct {
	repo    TenantRepository
	auditor *audit.Logger
	logger  *zap.Logger
}

// NewTenantService builds a TenantService. auditIns is the audit sink (the
// repository); a nil sink yields a no-op audit logger (safe for tests).
func NewTenantService(repo TenantRepository, auditIns audit.Inserter, logger *zap.Logger) *TenantService {
	return &TenantService{repo: repo, auditor: audit.New(auditIns, logger), logger: logger}
}

// ProvisionTenantRequest is the DTO for POST /api/v1/tenants.
type ProvisionTenantRequest struct {
	ID            string `json:"id"` // tenant slug, e.g. "acme-bank"
	DisplayName   string `json:"displayName"`
	MarketCode    string `json:"marketCode"`    // ISO 3166-1 alpha-2; defaults to the active market pack
	AdminUsername string `json:"adminUsername"` // defaults to "admin"
	AdminName     string `json:"adminName"`     // defaults to "Tenant Administrator"
	AdminEmail    string `json:"adminEmail"`
}

// ProvisionedTenant is the one-shot provisioning result. OneTimePassword is
// returned exactly once and stored only as a bcrypt hash.
type ProvisionedTenant struct {
	Tenant          *model.Tenant         `json:"tenant"`
	Settings        *model.TenantSettings `json:"settings"`
	AdminUser       *model.User           `json:"adminUser"`
	OneTimePassword string                `json:"oneTimePassword"`
}

// tenantSlugRe: lowercase DNS-label style, 3–63 chars, no leading/trailing
// hyphen — safe to use in URLs, queues and (later) subdomains.
var tenantSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// reservedTenantIDs cannot be provisioned: "system" holds the shared chart of
// accounts in the accounting service; "admin" is the built-in default tenant.
var reservedTenantIDs = map[string]bool{"system": true, "admin": true}

// Provision executes the "create neobank" flow. Idempotency: an existing slug
// returns 409 Conflict (repo convention), including races resolved via the
// unique constraint.
func (s *TenantService) Provision(ctx context.Context, req ProvisionTenantRequest) (*ProvisionedTenant, error) {
	slug := strings.ToLower(strings.TrimSpace(req.ID))
	switch {
	case !tenantSlugRe.MatchString(slug):
		return nil, errors.BadRequest("id must be a 3-63 char lowercase slug (a-z, 0-9, '-', no leading/trailing '-')")
	case reservedTenantIDs[slug]:
		return nil, errors.BadRequest("tenant id '" + slug + "' is reserved")
	case strings.TrimSpace(req.DisplayName) == "":
		return nil, errors.BadRequest("displayName is required")
	}
	adminEmail := strings.TrimSpace(req.AdminEmail)
	if adminEmail == "" || !strings.Contains(adminEmail, "@") {
		return nil, errors.BadRequest("adminEmail is required and must be an email address")
	}

	// Resolve the market pack: currency/timezone/locale come from data, not code.
	code := strings.ToUpper(strings.TrimSpace(req.MarketCode))
	if code == "" {
		code = market.Current().Code
	}
	pack, ok := market.Get(code)
	if !ok {
		return nil, errors.BadRequest("unknown market code '" + code + "' — no market pack registered")
	}

	if existing, err := s.repo.GetTenant(ctx, slug); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, errors.Conflict("Tenant '" + slug + "' already exists")
	}

	displayName := strings.TrimSpace(req.DisplayName)
	var createdBy *string
	if actor := auth.UserIDFromContext(ctx); actor != "" {
		createdBy = &actor
	}
	tenant := &model.Tenant{
		ID:          slug,
		DisplayName: displayName,
		MarketCode:  pack.Code,
		Status:      model.TenantStatusProvisioning,
		CreatedBy:   createdBy,
	}

	countryCode := pack.Code
	settings := &model.TenantSettings{
		TenantID:              slug,
		Currency:              pack.Currency,
		OrgName:               &displayName,
		CountryCode:           &countryCode,
		Timezone:              pack.Timezone,
		SessionTimeoutMinutes: 30,
		AuditTrailEnabled:     true,
	}

	adminUsername := strings.TrimSpace(req.AdminUsername)
	if adminUsername == "" {
		adminUsername = "admin"
	}
	adminName := strings.TrimSpace(req.AdminName)
	if adminName == "" {
		adminName = "Tenant Administrator"
	}
	adminUser := &model.User{
		TenantID: slug,
		Username: adminUsername,
		Name:     adminName,
		Email:    adminEmail,
		Role:     "ADMIN",
		Status:   "ACTIVE",
	}

	oneTimePassword, err := generateOneTimePassword()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(oneTimePassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	evt, err := commonevent.NewDomainEvent(commonevent.TenantProvisioned, "account-service", slug, "", map[string]any{
		"tenantId":      slug,
		"displayName":   displayName,
		"marketCode":    pack.Code,
		"currency":      pack.Currency,
		"timezone":      pack.Timezone,
		"adminUsername": adminUsername,
	})
	if err != nil {
		return nil, err
	}

	if err := s.repo.ProvisionTenant(ctx, tenant, settings, adminUser, string(hash), evt); err != nil {
		if repository.IsUniqueViolation(err) {
			// Concurrent create won the primary key — same outcome as the
			// pre-check above.
			return nil, errors.Conflict("Tenant '" + slug + "' already exists")
		}
		return nil, err
	}

	s.auditor.Record(ctx, "TENANT_PROVISION", "TENANT", slug, nil, tenant, map[string]any{
		"marketCode":    pack.Code,
		"currency":      pack.Currency,
		"adminUsername": adminUsername,
	})
	s.logger.Info("Tenant provisioned",
		zap.String("tenantId", slug),
		zap.String("marketCode", pack.Code))

	return &ProvisionedTenant{
		Tenant:          tenant,
		Settings:        settings,
		AdminUser:       adminUser,
		OneTimePassword: oneTimePassword,
	}, nil
}

// Get returns a tenant by slug or a NotFoundError.
func (s *TenantService) Get(ctx context.Context, id string) (*model.Tenant, error) {
	t, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errors.NotFoundResource("Tenant", id)
	}
	return t, nil
}

// List returns every registered tenant.
func (s *TenantService) List(ctx context.Context) ([]*model.Tenant, error) {
	return s.repo.ListTenants(ctx)
}

// Activate transitions PROVISIONING/SUSPENDED -> ACTIVE. Activating an already
// ACTIVE tenant is an idempotent no-op.
func (s *TenantService) Activate(ctx context.Context, id string) (*model.Tenant, error) {
	return s.transition(ctx, id, model.TenantStatusActive,
		commonevent.TenantActivated, "TENANT_ACTIVATE",
		model.TenantStatusProvisioning, model.TenantStatusSuspended)
}

// Suspend transitions PROVISIONING/ACTIVE -> SUSPENDED. Suspending an already
// SUSPENDED tenant is an idempotent no-op.
func (s *TenantService) Suspend(ctx context.Context, id string) (*model.Tenant, error) {
	return s.transition(ctx, id, model.TenantStatusSuspended,
		commonevent.TenantSuspended, "TENANT_SUSPEND",
		model.TenantStatusProvisioning, model.TenantStatusActive)
}

func (s *TenantService) transition(ctx context.Context, id string, target model.TenantStatus, eventType, auditAction string, from ...model.TenantStatus) (*model.Tenant, error) {
	t, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Status == target {
		return t, nil // idempotent
	}
	allowed := false
	for _, f := range from {
		if t.Status == f {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errors.Conflict("Cannot move tenant '" + id + "' from " + string(t.Status) + " to " + string(target))
	}

	evt, err := commonevent.NewDomainEvent(eventType, "account-service", id, "", map[string]any{
		"tenantId": id,
		"from":     t.Status,
		"to":       target,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateTenantStatus(ctx, id, target, evt); err != nil {
		return nil, err
	}

	before := *t
	t.Status = target
	s.auditor.Record(ctx, auditAction, "TENANT", id, before, t, nil)
	s.logger.Info("Tenant status changed",
		zap.String("tenantId", id),
		zap.String("from", string(before.Status)),
		zap.String("to", string(target)))
	return t, nil
}

// otpAlphabet excludes visually ambiguous characters (0/O, 1/l/I).
const otpAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// otpLength gives ~94 bits of entropy over otpAlphabet.
const otpLength = 16

func generateOneTimePassword() (string, error) {
	out := make([]byte, otpLength)
	max := big.NewInt(int64(len(otpAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = otpAlphabet[n.Int64()]
	}
	return string(out), nil
}

// GetBrand returns the tenant's brand pack, falling back to the platform
// default (Nemo deep-water) when the tenant has none. The tenant must exist.
func (s *TenantService) GetBrand(ctx context.Context, tenantID string) (*model.BrandPack, error) {
	raw, found, err := s.repo.GetTenantBrand(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.NotFoundResource("Tenant", tenantID)
	}
	if len(raw) == 0 {
		def := model.DefaultBrand()
		return &def, nil
	}
	var b model.BrandPack
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("stored brand pack is corrupt for tenant %s: %w", tenantID, err)
	}
	return &b, nil
}

// SetBrand validates and stores the tenant's brand pack.
func (s *TenantService) SetBrand(ctx context.Context, tenantID string, b model.BrandPack) (*model.BrandPack, error) {
	if err := b.Validate(); err != nil {
		return nil, errors.BadRequest(err.Error())
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetTenantBrand(ctx, tenantID, raw); err != nil {
		if errors2.Is(err, pgx.ErrNoRows) {
			return nil, errors.NotFoundResource("Tenant", tenantID)
		}
		return nil, err
	}
	s.logger.Info("Brand pack updated", zap.String("tenant", tenantID))
	return &b, nil
}
