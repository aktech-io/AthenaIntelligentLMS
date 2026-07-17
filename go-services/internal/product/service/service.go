package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/product/event"
	"github.com/athena-lms/go-services/internal/product/model"
	"github.com/athena-lms/go-services/internal/product/repository"
)

// Service contains all product and charge business logic.
type Service struct {
	repo      *repository.Repository
	publisher *event.Publisher
	logger    *zap.Logger
	auditor   *audit.Logger
}

// New creates a new Service.
func New(repo *repository.Repository, publisher *event.Publisher, logger *zap.Logger) *Service {
	return &Service{repo: repo, publisher: publisher, logger: logger, auditor: audit.New(repo, logger)}
}

// ListAuditLog returns the product-service audit trail, optionally filtered by
// entityType and entityId.
func (s *Service) ListAuditLog(ctx context.Context, tenantID, entityType, entityID string, limit, offset int) ([]*repository.AuditRecord, error) {
	return s.repo.ListAuditLog(ctx, tenantID, entityType, entityID, limit, offset)
}

// VerifyAuditChain reports whether the tamper-evident audit trail is intact, or
// the seq of the first altered/missing entry.
func (s *Service) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.repo.VerifyAuditChain(ctx, tenantID)
}

// ─── Product Operations ─────────────────────────────────────────────────────

// CreateProduct creates a new loan product.
func (s *Service) CreateProduct(ctx context.Context, req model.CreateProductRequest, tenantID, createdBy string) (*model.ProductResponse, error) {
	// Validate required fields
	if req.ProductCode == "" {
		return nil, &BusinessError{Status: 400, Msg: "productCode is required"}
	}
	if req.Name == "" {
		return nil, &BusinessError{Status: 400, Msg: "name is required"}
	}
	if req.ProductType == "" {
		return nil, &BusinessError{Status: 400, Msg: "productType is required"}
	}
	if req.NominalRate == nil {
		return nil, &BusinessError{Status: 400, Msg: "nominalRate is required"}
	}
	if req.MaxAmount == nil {
		return nil, &BusinessError{Status: 400, Msg: "maxAmount is required"}
	}
	if req.MaxTenorDays == nil {
		return nil, &BusinessError{Status: 400, Msg: "maxTenorDays is required"}
	}

	exists, err := s.repo.ExistsProductByCodeAndTenant(ctx, req.ProductCode, tenantID)
	if err != nil {
		return nil, fmt.Errorf("check product code: %w", err)
	}
	if exists {
		return nil, &ConflictError{Msg: "Product code already exists: " + req.ProductCode}
	}

	if req.MinAmount != nil && req.MaxAmount != nil && req.MinAmount.GreaterThan(*req.MaxAmount) {
		return nil, &BusinessError{Status: 422, Msg: fmt.Sprintf("minAmount (%s) must not exceed maxAmount (%s)", req.MinAmount, req.MaxAmount)}
	}
	if req.MinTenorDays != nil && req.MaxTenorDays != nil && *req.MinTenorDays > *req.MaxTenorDays {
		return nil, &BusinessError{Status: 422, Msg: fmt.Sprintf("minTenorDays (%d) must not exceed maxTenorDays (%d)", *req.MinTenorDays, *req.MaxTenorDays)}
	}

	product, err := s.buildProduct(req, tenantID, createdBy)
	if err != nil {
		return nil, err
	}

	if err := s.repo.CreateProduct(ctx, product); err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}

	s.saveVersionSnapshot(ctx, product, createdBy, "Initial creation")

	resp := model.ProductToResponse(product)

	s.auditor.Record(ctx, "PRODUCT_CREATE", "PRODUCT", product.ID.String(),
		nil, resp, map[string]any{"productCode": product.ProductCode, "name": product.Name})

	if s.publisher != nil {
		s.publisher.PublishProductCreated(ctx, tenantID, resp)
	}

	return &resp, nil
}

// GetProduct returns a product by ID and tenant.
func (s *Service) GetProduct(ctx context.Context, id uuid.UUID, tenantID string) (*model.ProductResponse, error) {
	product, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	resp := model.ProductToResponse(product)
	return &resp, nil
}

// ListProducts returns a paginated list of products for a tenant.
func (s *Service) ListProducts(ctx context.Context, tenantID string, page, size int) ([]model.ProductResponse, int64, error) {
	products, total, err := s.repo.ListProductsByTenant(ctx, tenantID, page, size)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	responses := make([]model.ProductResponse, len(products))
	for i, p := range products {
		responses[i] = model.ProductToResponse(&p)
	}
	return responses, total, nil
}

// UpdateProduct updates a product and saves a version snapshot.
func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, req model.CreateProductRequest, tenantID, changedBy, changeReason string) (*model.ProductResponse, error) {
	product, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Save snapshot before changes
	before := model.ProductToResponse(product)
	s.saveVersionSnapshot(ctx, product, changedBy, changeReason)

	// Apply changes
	if req.Name != "" {
		product.Name = req.Name
	}
	if req.Description != nil {
		product.Description = req.Description
	}
	if req.MaxAmount != nil {
		product.MaxAmount = req.MaxAmount
	}
	if req.MinAmount != nil {
		product.MinAmount = req.MinAmount
	}
	if req.MaxTenorDays != nil {
		product.MaxTenorDays = req.MaxTenorDays
	}
	if req.MinTenorDays != nil {
		product.MinTenorDays = req.MinTenorDays
	}
	if req.NominalRate != nil {
		product.NominalRate = *req.NominalRate
	}
	if req.PenaltyRate != nil {
		product.PenaltyRate = *req.PenaltyRate
	}
	if req.ScheduleType != nil {
		st, err := model.ParseScheduleType(*req.ScheduleType)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		product.ScheduleType = st
	}
	if req.RepaymentFrequency != nil {
		rf, err := model.ParseRepaymentFrequency(*req.RepaymentFrequency)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		product.RepaymentFrequency = rf
	}

	product.Version++

	// Two-person auth gate
	if product.RequiresTwoPersonAuth && product.AuthThresholdAmount != nil &&
		req.MaxAmount != nil && req.MaxAmount.GreaterThan(*product.AuthThresholdAmount) {
		product.PendingAuthorization = true
		product.Status = model.ProductStatusDraft
	}

	if err := s.repo.UpdateProduct(ctx, product); err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}

	resp := model.ProductToResponse(product)

	s.auditor.Record(ctx, "PRODUCT_UPDATE", "PRODUCT", product.ID.String(),
		before, resp, map[string]any{"version": product.Version, "changeReason": changeReason})

	if s.publisher != nil {
		s.publisher.PublishProductUpdated(ctx, tenantID, resp)
	}

	return &resp, nil
}

// ActivateProduct sets a product status to ACTIVE.
func (s *Service) ActivateProduct(ctx context.Context, id uuid.UUID, tenantID, approvedBy string) (*model.ProductResponse, error) {
	product, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	before := model.ProductToResponse(product)
	product.Status = model.ProductStatusActive
	product.PendingAuthorization = false
	s.logger.Info("Product activated", zap.String("code", product.ProductCode), zap.String("by", approvedBy))

	if err := s.repo.UpdateProduct(ctx, product); err != nil {
		return nil, fmt.Errorf("activate product: %w", err)
	}
	resp := model.ProductToResponse(product)

	s.auditor.Record(ctx, "PRODUCT_ACTIVATE", "PRODUCT", product.ID.String(),
		before, resp, map[string]any{"approvedBy": approvedBy})

	if s.publisher != nil {
		s.publisher.PublishProductActivated(ctx, tenantID, resp)
	}

	return &resp, nil
}

// DeactivateProduct sets a product status to INACTIVE.
func (s *Service) DeactivateProduct(ctx context.Context, id uuid.UUID, tenantID string) (*model.ProductResponse, error) {
	product, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	before := model.ProductToResponse(product)
	product.Status = model.ProductStatusInactive
	if err := s.repo.UpdateProduct(ctx, product); err != nil {
		return nil, fmt.Errorf("deactivate product: %w", err)
	}
	resp := model.ProductToResponse(product)
	s.auditor.Record(ctx, "PRODUCT_DEACTIVATE", "PRODUCT", product.ID.String(), before, resp, nil)
	return &resp, nil
}

// PauseProduct sets a product status to PAUSED (must be ACTIVE).
func (s *Service) PauseProduct(ctx context.Context, id uuid.UUID, tenantID, pausedBy string) (*model.ProductResponse, error) {
	product, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if product.Status != model.ProductStatusActive {
		return nil, &ConflictError{Msg: "Product must be ACTIVE to pause; current status: " + string(product.Status)}
	}
	product.Status = model.ProductStatusPaused
	s.logger.Info("Product paused", zap.String("code", product.ProductCode), zap.String("by", pausedBy))

	if err := s.repo.UpdateProduct(ctx, product); err != nil {
		return nil, fmt.Errorf("pause product: %w", err)
	}
	resp := model.ProductToResponse(product)
	return &resp, nil
}

// SimulateSchedule generates a repayment schedule for a product.
func (s *Service) SimulateSchedule(ctx context.Context, id uuid.UUID, req model.SimulateScheduleRequest, tenantID string) (*model.ScheduleResponse, error) {
	_, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	return simulate(req), nil
}

// GetProductVersions returns the version history for a product.
func (s *Service) GetProductVersions(ctx context.Context, id uuid.UUID, tenantID string) ([]model.ProductVersion, error) {
	_, err := s.loadProduct(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListVersionsByProduct(ctx, id)
}

// CreateFromTemplate creates a product from a template.
func (s *Service) CreateFromTemplate(ctx context.Context, templateCode, tenantID, createdBy string) (*model.ProductResponse, error) {
	tmpl, err := s.repo.GetTemplateByCode(ctx, templateCode)
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	if tmpl == nil {
		return nil, &NotFoundError{Msg: "Product template not found: " + templateCode}
	}

	var req model.CreateProductRequest
	if err := json.Unmarshal([]byte(tmpl.Configuration), &req); err != nil {
		return nil, fmt.Errorf("failed to parse template configuration: %w", err)
	}
	tc := templateCode
	req.TemplateID = &tc
	return s.CreateProduct(ctx, req, tenantID, createdBy)
}

// ListTemplates returns all active product templates.
func (s *Service) ListTemplates(ctx context.Context) ([]model.ProductTemplate, error) {
	return s.repo.ListActiveTemplates(ctx)
}

// GetTemplate returns a product template by code.
func (s *Service) GetTemplate(ctx context.Context, code string) (*model.ProductTemplate, error) {
	tmpl, err := s.repo.GetTemplateByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	if tmpl == nil {
		return nil, &NotFoundError{Msg: "ProductTemplate not found: " + code}
	}
	return tmpl, nil
}

// ─── Charge Operations ──────────────────────────────────────────────────────

// CreateCharge creates a new transaction charge.
func (s *Service) CreateCharge(ctx context.Context, req model.CreateChargeRequest, tenantID string) (*model.TransactionChargeResponse, error) {
	if req.ChargeCode == "" {
		return nil, &BusinessError{Status: 400, Msg: "chargeCode is required"}
	}
	if req.ChargeName == "" {
		return nil, &BusinessError{Status: 400, Msg: "chargeName is required"}
	}

	exists, err := s.repo.ExistsChargeByCodeAndTenant(ctx, req.ChargeCode, tenantID)
	if err != nil {
		return nil, fmt.Errorf("check charge code: %w", err)
	}
	if exists {
		return nil, &BusinessError{Status: 400, Msg: "Charge code already exists: " + req.ChargeCode}
	}

	txnType, err := model.ParseChargeTransactionType(req.TransactionType)
	if err != nil {
		return nil, &BusinessError{Status: 400, Msg: err.Error()}
	}
	calcType, err := model.ParseChargeCalculationType(req.CalculationType)
	if err != nil {
		return nil, &BusinessError{Status: 400, Msg: err.Error()}
	}

	currency := market.Currency()
	if req.Currency != nil {
		currency = *req.Currency
	}

	charge := &model.TransactionCharge{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ChargeCode:      req.ChargeCode,
		ChargeName:      req.ChargeName,
		TransactionType: txnType,
		CalculationType: calcType,
		FlatAmount:      req.FlatAmount,
		PercentageRate:  req.PercentageRate,
		MinAmount:       req.MinAmount,
		MaxAmount:       req.MaxAmount,
		Currency:        currency,
		IsActive:        true,
	}

	for _, tr := range req.Tiers {
		charge.Tiers = append(charge.Tiers, model.ChargeTier{
			FromAmount:     tr.FromAmount,
			ToAmount:       tr.ToAmount,
			FlatAmount:     tr.FlatAmount,
			PercentageRate: tr.PercentageRate,
		})
	}
	if charge.Tiers == nil {
		charge.Tiers = []model.ChargeTier{}
	}

	if err := s.repo.CreateCharge(ctx, charge); err != nil {
		return nil, fmt.Errorf("create charge: %w", err)
	}

	s.logger.Info("Created charge config",
		zap.String("code", charge.ChargeCode),
		zap.String("id", charge.ID.String()),
		zap.String("tenant", tenantID),
	)

	resp := model.ChargeToResponse(charge)
	return &resp, nil
}

// GetCharge returns a charge by ID and tenant.
func (s *Service) GetCharge(ctx context.Context, id uuid.UUID, tenantID string) (*model.TransactionChargeResponse, error) {
	c, err := s.repo.GetChargeByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get charge: %w", err)
	}
	if c == nil {
		return nil, &NotFoundError{Msg: fmt.Sprintf("Charge not found with id: %s", id)}
	}
	resp := model.ChargeToResponse(c)
	return &resp, nil
}

// ListCharges returns a paginated list of charges for a tenant.
func (s *Service) ListCharges(ctx context.Context, tenantID string, page, size int) ([]model.TransactionChargeResponse, int64, error) {
	charges, total, err := s.repo.ListChargesByTenant(ctx, tenantID, page, size)
	if err != nil {
		return nil, 0, fmt.Errorf("list charges: %w", err)
	}
	responses := make([]model.TransactionChargeResponse, len(charges))
	for i, c := range charges {
		responses[i] = model.ChargeToResponse(&c)
	}
	return responses, total, nil
}

// UpdateCharge updates a transaction charge.
func (s *Service) UpdateCharge(ctx context.Context, id uuid.UUID, req model.CreateChargeRequest, tenantID string) (*model.TransactionChargeResponse, error) {
	charge, err := s.repo.GetChargeByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get charge: %w", err)
	}
	if charge == nil {
		return nil, &NotFoundError{Msg: fmt.Sprintf("Charge not found with id: %s", id)}
	}

	if req.ChargeName != "" {
		charge.ChargeName = req.ChargeName
	}
	if req.TransactionType != "" {
		txnType, err := model.ParseChargeTransactionType(req.TransactionType)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		charge.TransactionType = txnType
	}
	if req.CalculationType != "" {
		calcType, err := model.ParseChargeCalculationType(req.CalculationType)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		charge.CalculationType = calcType
	}
	if req.FlatAmount != nil {
		charge.FlatAmount = req.FlatAmount
	}
	if req.PercentageRate != nil {
		charge.PercentageRate = req.PercentageRate
	}
	if req.MinAmount != nil {
		charge.MinAmount = req.MinAmount
	}
	if req.MaxAmount != nil {
		charge.MaxAmount = req.MaxAmount
	}
	if req.Currency != nil {
		charge.Currency = *req.Currency
	}

	if req.Tiers != nil {
		charge.Tiers = nil
		for _, tr := range req.Tiers {
			charge.Tiers = append(charge.Tiers, model.ChargeTier{
				FromAmount:     tr.FromAmount,
				ToAmount:       tr.ToAmount,
				FlatAmount:     tr.FlatAmount,
				PercentageRate: tr.PercentageRate,
			})
		}
		if charge.Tiers == nil {
			charge.Tiers = []model.ChargeTier{}
		}
	}

	if err := s.repo.UpdateCharge(ctx, charge); err != nil {
		return nil, fmt.Errorf("update charge: %w", err)
	}
	resp := model.ChargeToResponse(charge)
	return &resp, nil
}

// DeleteCharge deletes a transaction charge.
func (s *Service) DeleteCharge(ctx context.Context, id uuid.UUID, tenantID string) error {
	charge, err := s.repo.GetChargeByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("get charge: %w", err)
	}
	if charge == nil {
		return &NotFoundError{Msg: fmt.Sprintf("Charge not found with id: %s", id)}
	}
	return s.repo.DeleteCharge(ctx, id, tenantID)
}

// CalculateCharge calculates the charge for a given transaction type and amount.
func (s *Service) CalculateCharge(ctx context.Context, transactionType string, amount decimal.Decimal, tenantID string) (*model.ChargeCalculationResponse, error) {
	txnType, err := model.ParseChargeTransactionType(transactionType)
	if err != nil {
		return nil, &BusinessError{Status: 400, Msg: err.Error()}
	}

	charges, err := s.repo.FindActiveChargesByTransactionType(ctx, tenantID, txnType)
	if err != nil {
		return nil, fmt.Errorf("find charges: %w", err)
	}

	if len(charges) == 0 {
		return &model.ChargeCalculationResponse{
			TransactionType:   transactionType,
			TransactionAmount: amount,
			ChargeAmount:      decimal.Zero,
			Currency:          market.Currency(),
		}, nil
	}

	charge := charges[0]
	chargeAmount := doCalculate(&charge, amount)

	return &model.ChargeCalculationResponse{
		ChargeCode:        charge.ChargeCode,
		ChargeName:        charge.ChargeName,
		TransactionType:   transactionType,
		TransactionAmount: amount,
		ChargeAmount:      chargeAmount,
		Currency:          charge.Currency,
	}, nil
}

// ─── Internal helpers ───────────────────────────────────────────────────────

func (s *Service) loadProduct(ctx context.Context, id uuid.UUID, tenantID string) (*model.Product, error) {
	product, err := s.repo.GetProductByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load product: %w", err)
	}
	if product == nil {
		return nil, &NotFoundError{Msg: fmt.Sprintf("Product not found with id: %s", id)}
	}
	return product, nil
}

func (s *Service) buildProduct(req model.CreateProductRequest, tenantID, createdBy string) (*model.Product, error) {
	pt, err := model.ParseProductType(req.ProductType)
	if err != nil {
		return nil, &BusinessError{Status: 400, Msg: err.Error()}
	}

	schedType := model.ScheduleTypeEMI
	if req.ScheduleType != nil {
		schedType, err = model.ParseScheduleType(*req.ScheduleType)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
	}

	repFreq := model.RepaymentFrequencyMonthly
	if req.RepaymentFrequency != nil {
		repFreq, err = model.ParseRepaymentFrequency(*req.RepaymentFrequency)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
	}

	currency := market.Currency()
	if req.Currency != nil {
		currency = *req.Currency
	}

	penaltyRate := decimal.Zero
	if req.PenaltyRate != nil {
		penaltyRate = *req.PenaltyRate
	}

	penaltyGraceDays := 1
	if req.PenaltyGraceDays != nil {
		penaltyGraceDays = *req.PenaltyGraceDays
	}

	gracePeriodDays := 0
	if req.GracePeriodDays != nil {
		gracePeriodDays = *req.GracePeriodDays
	}

	processingFeeRate := decimal.Zero
	if req.ProcessingFeeRate != nil {
		processingFeeRate = *req.ProcessingFeeRate
	}

	processingFeeMin := decimal.Zero
	if req.ProcessingFeeMin != nil {
		processingFeeMin = *req.ProcessingFeeMin
	}

	maxDtir := decimal.NewFromFloat(100.0)
	if req.MaxDtir != nil {
		maxDtir = *req.MaxDtir
	}

	var fees []model.ProductFee
	for _, f := range req.Fees {
		ft, err := model.ParseFeeType(f.FeeType)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		ct, err := model.ParseCalculationType(f.CalculationType)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		fees = append(fees, model.ProductFee{
			FeeName:         f.FeeName,
			FeeType:         ft,
			CalculationType: ct,
			Amount:          f.Amount,
			Rate:            f.Rate,
			IsMandatory:     f.IsMandatory,
		})
	}
	if fees == nil {
		fees = []model.ProductFee{}
	}

	return &model.Product{
		ID:                    uuid.New(),
		TenantID:              tenantID,
		ProductCode:           req.ProductCode,
		Name:                  req.Name,
		ProductType:           pt,
		Status:                model.ProductStatusDraft,
		Description:           req.Description,
		Currency:              currency,
		MinAmount:             req.MinAmount,
		MaxAmount:             req.MaxAmount,
		MinTenorDays:          req.MinTenorDays,
		MaxTenorDays:          req.MaxTenorDays,
		ScheduleType:          schedType,
		RepaymentFrequency:    repFreq,
		NominalRate:           *req.NominalRate,
		PenaltyRate:           penaltyRate,
		PenaltyGraceDays:      penaltyGraceDays,
		GracePeriodDays:       gracePeriodDays,
		ProcessingFeeRate:     processingFeeRate,
		ProcessingFeeMin:      processingFeeMin,
		ProcessingFeeMax:      req.ProcessingFeeMax,
		RequiresCollateral:    req.RequiresCollateral,
		MinCreditScore:        req.MinCreditScore,
		MaxDtir:               maxDtir,
		Version:               1,
		TemplateID:            req.TemplateID,
		RequiresTwoPersonAuth: req.RequiresTwoPersonAuth,
		AuthThresholdAmount:   req.AuthThresholdAmount,
		PendingAuthorization:  req.RequiresTwoPersonAuth,
		CreatedBy:             &createdBy,
		Fees:                  fees,
	}, nil
}

func (s *Service) saveVersionSnapshot(ctx context.Context, product *model.Product, changedBy, reason string) {
	resp := model.ProductToResponse(product)
	snapshot, err := json.Marshal(resp)
	if err != nil {
		s.logger.Warn("Failed to marshal version snapshot",
			zap.String("product", product.ID.String()),
			zap.Error(err),
		)
		return
	}
	v := &model.ProductVersion{
		ID:            uuid.New(),
		ProductID:     product.ID,
		VersionNumber: product.Version,
		Snapshot:      snapshot,
		ChangedBy:     &changedBy,
		ChangeReason:  &reason,
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		s.logger.Warn("Failed to save version snapshot",
			zap.String("product", product.ID.String()),
			zap.Error(err),
		)
	}
}

func doCalculate(charge *model.TransactionCharge, amount decimal.Decimal) decimal.Decimal {
	hundred := decimal.NewFromInt(100)
	switch charge.CalculationType {
	case model.ChargeCalculationTypeFlat:
		if charge.FlatAmount != nil {
			return *charge.FlatAmount
		}
		return decimal.Zero
	case model.ChargeCalculationTypePercentage:
		if charge.PercentageRate == nil {
			return decimal.Zero
		}
		calculated := amount.Mul(*charge.PercentageRate).Div(hundred).Round(2)
		if charge.MinAmount != nil && calculated.LessThan(*charge.MinAmount) {
			calculated = *charge.MinAmount
		}
		if charge.MaxAmount != nil && calculated.GreaterThan(*charge.MaxAmount) {
			calculated = *charge.MaxAmount
		}
		return calculated
	case model.ChargeCalculationTypeTiered:
		for _, tier := range charge.Tiers {
			if amount.GreaterThanOrEqual(tier.FromAmount) && amount.LessThanOrEqual(tier.ToAmount) {
				if tier.FlatAmount != nil {
					return *tier.FlatAmount
				}
				if tier.PercentageRate != nil {
					return amount.Mul(*tier.PercentageRate).Div(hundred).Round(2)
				}
			}
		}
		return decimal.Zero
	}
	return decimal.Zero
}

// ─── Schedule Simulator ─────────────────────────────────────────────────────

func simulate(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	switch req.ScheduleType {
	case model.ScheduleTypeEMI:
		return simulateEmi(req)
	case model.ScheduleTypeFlat, model.ScheduleTypeFlatRate:
		return simulateFlat(req)
	case model.ScheduleTypeActuarial:
		return simulateActuarial(req)
	case model.ScheduleTypeDailySimple:
		return simulateDailySimple(req)
	case model.ScheduleTypeBalloon:
		return simulateBalloon(req)
	case model.ScheduleTypeSeasonal:
		return simulateSeasonal(req)
	case model.ScheduleTypeGraduated:
		return simulateGraduated(req)
	default:
		return simulateEmi(req)
	}
}

func parseDisbursementDate(req model.SimulateScheduleRequest) time.Time {
	if req.DisbursementDate != nil {
		t, err := time.Parse("2006-01-02", *req.DisbursementDate)
		if err == nil {
			return t
		}
	}
	return time.Now().Truncate(24 * time.Hour)
}

func computePeriods(req model.SimulateScheduleRequest) int {
	daysInPeriod := req.RepaymentFrequency.DaysInPeriod()
	if daysInPeriod == 0 {
		return 1 // BULLET
	}
	p := int(math.Ceil(float64(req.TenorDays) / float64(daysInPeriod)))
	if p < 1 {
		return 1
	}
	return p
}

func simulateEmi(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	P := req.Principal
	periods := computePeriods(req)

	hundred := decimal.NewFromInt(100)
	annualRate := req.NominalRate.Div(hundred)
	ppy := decimal.NewFromInt(int64(req.RepaymentFrequency.PeriodsPerYear()))
	periodRate := annualRate.Div(ppy)

	installments := make([]model.InstallmentResponse, 0, periods)
	outstanding := P
	totalInterest := decimal.Zero
	dueDate := parseDisbursementDate(req)

	var emi decimal.Decimal
	if periodRate.IsZero() {
		emi = P.Div(decimal.NewFromInt(int64(periods))).Round(2)
	} else {
		// EMI = P * r * (1+r)^n / ((1+r)^n - 1)
		onePlusR := decimal.NewFromInt(1).Add(periodRate)
		onePlusRn := decPow(onePlusR, periods)
		emi = P.Mul(periodRate).Mul(onePlusRn).Div(onePlusRn.Sub(decimal.NewFromInt(1))).Round(2)
	}

	for i := 1; i <= periods; i++ {
		days := req.RepaymentFrequency.DaysInPeriod()
		if days <= 0 {
			days = req.TenorDays
		}
		dueDate = dueDate.AddDate(0, 0, days)

		interest := outstanding.Mul(periodRate).Round(2)
		var principal decimal.Decimal
		if i == periods {
			principal = outstanding
		} else {
			principal = emi.Sub(interest).Round(2)
		}
		if principal.GreaterThan(outstanding) {
			principal = outstanding
		}
		outstanding = outstanding.Sub(principal).Round(2)
		totalInterest = totalInterest.Add(interest)

		installments = append(installments, model.InstallmentResponse{
			InstallmentNumber:  i,
			DueDate:            dueDate.Format("2006-01-02"),
			Principal:          principal,
			Interest:           interest,
			TotalPayment:       principal.Add(interest),
			OutstandingBalance: outstanding,
		})
	}

	totalPayable := P.Add(totalInterest).Round(2)
	effectiveRate := decimal.Zero
	if !P.IsZero() {
		effectiveRate = totalInterest.Div(P).Mul(hundred).Round(4)
	}

	return &model.ScheduleResponse{
		ScheduleType:         "EMI",
		Principal:            P,
		TotalInterest:        totalInterest.Round(2),
		TotalPayable:         totalPayable,
		EffectiveRate:        effectiveRate,
		NumberOfInstallments: periods,
		Installments:         installments,
	}
}

func simulateFlat(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	P := req.Principal
	periods := computePeriods(req)

	hundred := decimal.NewFromInt(100)
	d365 := decimal.NewFromInt(365)
	annualRate := req.NominalRate.Div(hundred)
	totalInterest := P.Mul(annualRate).Mul(decimal.NewFromInt(int64(req.TenorDays))).Div(d365).Round(2)

	principalPerPeriod := P.Div(decimal.NewFromInt(int64(periods))).Round(2)
	interestPerPeriod := totalInterest.Div(decimal.NewFromInt(int64(periods))).Round(2)

	installments := make([]model.InstallmentResponse, 0, periods)
	dueDate := parseDisbursementDate(req)
	outstanding := P

	for i := 1; i <= periods; i++ {
		days := req.RepaymentFrequency.DaysInPeriod()
		if days <= 0 {
			days = req.TenorDays / periods
		}
		dueDate = dueDate.AddDate(0, 0, days)

		principal := principalPerPeriod
		if i == periods {
			principal = outstanding
		}
		outstanding = outstanding.Sub(principal).Round(2)

		installments = append(installments, model.InstallmentResponse{
			InstallmentNumber:  i,
			DueDate:            dueDate.Format("2006-01-02"),
			Principal:          principal,
			Interest:           interestPerPeriod,
			TotalPayment:       principal.Add(interestPerPeriod),
			OutstandingBalance: outstanding,
		})
	}

	return &model.ScheduleResponse{
		ScheduleType:         "FLAT",
		Principal:            P,
		TotalInterest:        totalInterest,
		TotalPayable:         P.Add(totalInterest),
		EffectiveRate:        req.NominalRate,
		NumberOfInstallments: periods,
		Installments:         installments,
	}
}

func simulateActuarial(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	r := simulateEmi(req)
	r.ScheduleType = "ACTUARIAL"
	return r
}

func simulateDailySimple(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	P := req.Principal
	periods := computePeriods(req)

	hundred := decimal.NewFromInt(100)
	d365 := decimal.NewFromInt(365)
	dailyRate := req.NominalRate.Div(hundred).Div(d365)

	daysPerPeriod := req.RepaymentFrequency.DaysInPeriod()
	if daysPerPeriod <= 0 {
		daysPerPeriod = req.TenorDays / periods
	}

	principalPerPeriod := P.Div(decimal.NewFromInt(int64(periods))).Round(2)
	outstanding := P
	totalInterest := decimal.Zero
	installments := make([]model.InstallmentResponse, 0, periods)
	dueDate := parseDisbursementDate(req)

	for i := 1; i <= periods; i++ {
		dueDate = dueDate.AddDate(0, 0, daysPerPeriod)
		interest := outstanding.Mul(dailyRate).Mul(decimal.NewFromInt(int64(daysPerPeriod))).Round(2)
		principal := principalPerPeriod
		if i == periods {
			principal = outstanding
		}
		outstanding = outstanding.Sub(principal).Round(2)
		totalInterest = totalInterest.Add(interest)

		installments = append(installments, model.InstallmentResponse{
			InstallmentNumber:  i,
			DueDate:            dueDate.Format("2006-01-02"),
			Principal:          principal,
			Interest:           interest,
			TotalPayment:       principal.Add(interest),
			OutstandingBalance: outstanding,
		})
	}

	return &model.ScheduleResponse{
		ScheduleType:         "DAILY_SIMPLE",
		Principal:            P,
		TotalInterest:        totalInterest.Round(2),
		TotalPayable:         P.Add(totalInterest).Round(2),
		EffectiveRate:        req.NominalRate,
		NumberOfInstallments: periods,
		Installments:         installments,
	}
}

func simulateBalloon(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	P := req.Principal
	periods := computePeriods(req)

	hundred := decimal.NewFromInt(100)
	annualRate := req.NominalRate.Div(hundred)
	ppy := decimal.NewFromInt(int64(req.RepaymentFrequency.PeriodsPerYear()))
	periodRate := annualRate.Div(ppy)
	periodInterest := P.Mul(periodRate).Round(2)

	installments := make([]model.InstallmentResponse, 0, periods)
	dueDate := parseDisbursementDate(req)
	totalInterest := decimal.Zero

	for i := 1; i <= periods; i++ {
		days := req.RepaymentFrequency.DaysInPeriod()
		if days <= 0 {
			days = req.TenorDays / periods
		}
		dueDate = dueDate.AddDate(0, 0, days)

		isLast := i == periods
		principal := decimal.Zero
		if isLast {
			principal = P
		}
		total := principal.Add(periodInterest)
		totalInterest = totalInterest.Add(periodInterest)

		bal := P
		if isLast {
			bal = decimal.Zero
		}

		installments = append(installments, model.InstallmentResponse{
			InstallmentNumber:  i,
			DueDate:            dueDate.Format("2006-01-02"),
			Principal:          principal,
			Interest:           periodInterest,
			TotalPayment:       total,
			OutstandingBalance: bal,
		})
	}

	return &model.ScheduleResponse{
		ScheduleType:         "BALLOON",
		Principal:            P,
		TotalInterest:        totalInterest.Round(2),
		TotalPayable:         P.Add(totalInterest).Round(2),
		EffectiveRate:        req.NominalRate,
		NumberOfInstallments: periods,
		Installments:         installments,
	}
}

func simulateSeasonal(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	P := req.Principal
	tenorMonths := int(math.Ceil(float64(req.TenorDays) / 30.0))
	if tenorMonths < 2 {
		tenorMonths = 2
	}

	// Payment months: odd months only
	var paymentMonths []int
	for m := 1; m <= tenorMonths; m++ {
		if m%2 != 0 {
			paymentMonths = append(paymentMonths, m)
		}
	}
	n := len(paymentMonths)
	if n == 0 {
		n = 1
	}

	hundred := decimal.NewFromInt(100)
	d365 := decimal.NewFromInt(365)
	annualRate := req.NominalRate.Div(hundred)
	totalInterest := P.Mul(annualRate).Mul(decimal.NewFromInt(int64(req.TenorDays))).Div(d365).Round(2)

	paymentAmount := P.Add(totalInterest).Div(decimal.NewFromInt(int64(n))).Round(2)

	installments := make([]model.InstallmentResponse, 0, tenorMonths)
	outstanding := P
	outstandingInterest := totalInterest
	installNum := 0

	dueBase := parseDisbursementDate(req)
	for m := 1; m <= tenorMonths; m++ {
		dueDate := dueBase.AddDate(0, m, 0)
		isPaymentMonth := m%2 != 0
		installNum++

		principal := decimal.Zero
		interest := decimal.Zero

		if isPaymentMonth {
			interest = decimal.Min(paymentAmount, outstandingInterest).Round(2)
			principal = paymentAmount.Sub(interest).Round(2)
			if principal.GreaterThan(outstanding) {
				principal = outstanding
			}
			outstanding = outstanding.Sub(principal).Round(2)
			outstandingInterest = outstandingInterest.Sub(interest).Round(2)
		}

		installments = append(installments, model.InstallmentResponse{
			InstallmentNumber:  installNum,
			DueDate:            dueDate.Format("2006-01-02"),
			Principal:          principal,
			Interest:           interest,
			TotalPayment:       principal.Add(interest),
			OutstandingBalance: outstanding,
		})
	}

	return &model.ScheduleResponse{
		ScheduleType:         "SEASONAL",
		Principal:            P,
		TotalInterest:        totalInterest,
		TotalPayable:         P.Add(totalInterest).Round(2),
		EffectiveRate:        req.NominalRate,
		NumberOfInstallments: tenorMonths,
		Installments:         installments,
	}
}

func simulateGraduated(req model.SimulateScheduleRequest) *model.ScheduleResponse {
	P := req.Principal
	periods := computePeriods(req)

	hundred := decimal.NewFromInt(100)
	annualRate := req.NominalRate.Div(hundred)
	ppy := decimal.NewFromInt(int64(req.RepaymentFrequency.PeriodsPerYear()))
	periodRate := annualRate.Div(ppy)
	growthRate := decimal.NewFromFloat(0.05)

	// Solve for P1
	sum := decimal.Zero
	for i := 0; i < periods; i++ {
		numerator := decPow(decimal.NewFromInt(1).Add(growthRate), i)
		denominator := decPow(decimal.NewFromInt(1).Add(periodRate), i+1)
		sum = sum.Add(numerator.Div(denominator))
	}
	p1 := P
	if !sum.IsZero() {
		p1 = P.Div(sum)
	}

	installments := make([]model.InstallmentResponse, 0, periods)
	outstanding := P
	totalInterest := decimal.Zero
	dueDate := parseDisbursementDate(req)

	for i := 1; i <= periods; i++ {
		days := req.RepaymentFrequency.DaysInPeriod()
		if days <= 0 {
			days = req.TenorDays / periods
		}
		dueDate = dueDate.AddDate(0, 0, days)

		payment := p1.Mul(decPow(decimal.NewFromInt(1).Add(growthRate), i-1)).Round(2)
		interest := outstanding.Mul(periodRate).Round(2)
		principal := payment.Sub(interest).Round(2)

		if i == periods {
			principal = outstanding
			payment = principal.Add(interest)
		} else if principal.GreaterThan(outstanding) {
			principal = outstanding
			payment = principal.Add(interest)
		}

		outstanding = outstanding.Sub(principal).Round(2)
		totalInterest = totalInterest.Add(interest)

		installments = append(installments, model.InstallmentResponse{
			InstallmentNumber:  i,
			DueDate:            dueDate.Format("2006-01-02"),
			Principal:          principal,
			Interest:           interest,
			TotalPayment:       payment,
			OutstandingBalance: outstanding,
		})
	}

	return &model.ScheduleResponse{
		ScheduleType:         "GRADUATED",
		Principal:            P,
		TotalInterest:        totalInterest.Round(2),
		TotalPayable:         P.Add(totalInterest).Round(2),
		EffectiveRate:        req.NominalRate,
		NumberOfInstallments: periods,
		Installments:         installments,
	}
}

// decPow raises d to the power n (integer exponent).
func decPow(d decimal.Decimal, n int) decimal.Decimal {
	if n == 0 {
		return decimal.NewFromInt(1)
	}
	result := decimal.NewFromInt(1)
	base := d
	exp := n
	if exp < 0 {
		base = decimal.NewFromInt(1).Div(base)
		exp = -exp
	}
	for exp > 0 {
		if exp%2 == 1 {
			result = result.Mul(base)
		}
		base = base.Mul(base)
		exp /= 2
	}
	return result
}

// ─── Error Types ────────────────────────────────────────────────────────────

type BusinessError struct {
	Status int
	Msg    string
}

func (e *BusinessError) Error() string { return e.Msg }

type ConflictError struct {
	Msg string
}

func (e *ConflictError) Error() string { return e.Msg }

type NotFoundError struct {
	Msg string
}

func (e *NotFoundError) Error() string { return e.Msg }
