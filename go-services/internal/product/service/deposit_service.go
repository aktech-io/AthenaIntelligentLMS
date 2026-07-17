package service

import (
	"context"
	"github.com/athena-lms/go-services/internal/common/market"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/product/model"
	"github.com/athena-lms/go-services/internal/product/repository"
)

// DepositService handles deposit product business logic.
type DepositService struct {
	repo   *repository.Repository
	logger *zap.Logger
}

// NewDepositService creates a new DepositService.
func NewDepositService(repo *repository.Repository, logger *zap.Logger) *DepositService {
	return &DepositService{repo: repo, logger: logger}
}

// CreateDepositProduct creates a new deposit product.
func (s *DepositService) CreateDepositProduct(ctx context.Context, req model.CreateDepositProductRequest, tenantID, username string) (*model.DepositProductResponse, error) {
	if req.ProductCode == "" {
		return nil, &BusinessError{Status: 400, Msg: "productCode is required"}
	}
	if req.Name == "" {
		return nil, &BusinessError{Status: 400, Msg: "name is required"}
	}

	category, err := model.ParseDepositProductCategory(req.ProductCategory)
	if err != nil {
		return nil, &BusinessError{Status: 400, Msg: err.Error()}
	}

	exists, err := s.repo.ExistsDepositProductByCodeAndTenant(ctx, req.ProductCode, tenantID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &ConflictError{Msg: "deposit product with code " + req.ProductCode + " already exists"}
	}

	currency := market.Currency()
	if req.Currency != nil {
		currency = *req.Currency
	}

	interestRate := decimal.Zero
	if req.InterestRate != nil {
		interestRate = *req.InterestRate
	}

	calcMethod := model.InterestCalcDailyBalance
	if req.InterestCalcMethod != nil {
		calcMethod = model.InterestCalcMethod(*req.InterestCalcMethod)
	}
	postingFreq := model.PostingFreqMonthly
	if req.InterestPostingFreq != nil {
		postingFreq = model.InterestPostingFreq(*req.InterestPostingFreq)
	}
	compoundFreq := model.CompoundFreqMonthly
	if req.InterestCompoundFreq != nil {
		compoundFreq = model.InterestCompoundFreq(*req.InterestCompoundFreq)
	}
	accrualFreq := model.AccrualFreqDaily
	if req.AccrualFrequency != nil {
		accrualFreq = model.AccrualFrequency(*req.AccrualFrequency)
	}

	dormancyDays := 365
	if req.DormancyDaysThreshold != nil {
		dormancyDays = *req.DormancyDaysThreshold
	}

	p := &model.DepositProduct{
		ID:                         uuid.New(),
		TenantID:                   tenantID,
		ProductCode:                req.ProductCode,
		Name:                       req.Name,
		Description:                req.Description,
		ProductCategory:            category,
		Status:                     model.DepositStatusDraft,
		Currency:                   currency,
		InterestRate:               interestRate,
		InterestCalcMethod:         calcMethod,
		InterestPostingFreq:        postingFreq,
		InterestCompoundFreq:       compoundFreq,
		AccrualFrequency:           accrualFreq,
		MinOpeningBalance:          decimalOrZero(req.MinOpeningBalance),
		MinOperatingBalance:        decimalOrZero(req.MinOperatingBalance),
		MinBalanceForInterest:      decimalOrZero(req.MinBalanceForInterest),
		MinTermDays:                req.MinTermDays,
		MaxTermDays:                req.MaxTermDays,
		EarlyWithdrawalPenaltyRate: req.EarlyWithdrawalPenaltyRate,
		AutoRenew:                  req.AutoRenew,
		DormancyDaysThreshold:      dormancyDays,
		DormancyChargeAmount:       req.DormancyChargeAmount,
		MonthlyMaintenanceFee:      req.MonthlyMaintenanceFee,
		MaxWithdrawalsPerMonth:     req.MaxWithdrawalsPerMonth,
		Version:                    1,
		CreatedBy:                  &username,
	}

	for _, t := range req.InterestTiers {
		p.InterestTiers = append(p.InterestTiers, model.DepositInterestTier{
			FromAmount: t.FromAmount,
			ToAmount:   t.ToAmount,
			Rate:       t.Rate,
		})
	}

	if err := s.repo.CreateDepositProduct(ctx, p); err != nil {
		return nil, err
	}

	resp := model.DepositProductToResponse(p)
	return &resp, nil
}

// GetDepositProduct fetches a deposit product.
func (s *DepositService) GetDepositProduct(ctx context.Context, id uuid.UUID, tenantID string) (*model.DepositProductResponse, error) {
	p, err := s.repo.GetDepositProductByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &NotFoundError{Msg: "deposit product not found"}
	}
	resp := model.DepositProductToResponse(p)
	return &resp, nil
}

// ListDepositProducts lists deposit products for a tenant.
func (s *DepositService) ListDepositProducts(ctx context.Context, tenantID string, page, size int) ([]model.DepositProductResponse, int64, error) {
	products, total, err := s.repo.ListDepositProductsByTenant(ctx, tenantID, page, size)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]model.DepositProductResponse, 0, len(products))
	for i := range products {
		resp = append(resp, model.DepositProductToResponse(&products[i]))
	}
	return resp, total, nil
}

// UpdateDepositProduct updates a deposit product.
func (s *DepositService) UpdateDepositProduct(ctx context.Context, id uuid.UUID, req model.CreateDepositProductRequest, tenantID string) (*model.DepositProductResponse, error) {
	existing, err := s.repo.GetDepositProductByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &NotFoundError{Msg: "deposit product not found"}
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.ProductCategory != "" {
		cat, err := model.ParseDepositProductCategory(req.ProductCategory)
		if err != nil {
			return nil, &BusinessError{Status: 400, Msg: err.Error()}
		}
		existing.ProductCategory = cat
	}
	if req.Currency != nil {
		existing.Currency = *req.Currency
	}
	if req.InterestRate != nil {
		existing.InterestRate = *req.InterestRate
	}
	if req.InterestCalcMethod != nil {
		existing.InterestCalcMethod = model.InterestCalcMethod(*req.InterestCalcMethod)
	}
	if req.InterestPostingFreq != nil {
		existing.InterestPostingFreq = model.InterestPostingFreq(*req.InterestPostingFreq)
	}
	if req.InterestCompoundFreq != nil {
		existing.InterestCompoundFreq = model.InterestCompoundFreq(*req.InterestCompoundFreq)
	}
	if req.AccrualFrequency != nil {
		existing.AccrualFrequency = model.AccrualFrequency(*req.AccrualFrequency)
	}
	if req.MinOpeningBalance != nil {
		existing.MinOpeningBalance = *req.MinOpeningBalance
	}
	if req.MinOperatingBalance != nil {
		existing.MinOperatingBalance = *req.MinOperatingBalance
	}
	if req.MinBalanceForInterest != nil {
		existing.MinBalanceForInterest = *req.MinBalanceForInterest
	}
	existing.MinTermDays = req.MinTermDays
	existing.MaxTermDays = req.MaxTermDays
	existing.EarlyWithdrawalPenaltyRate = req.EarlyWithdrawalPenaltyRate
	existing.AutoRenew = req.AutoRenew
	if req.DormancyDaysThreshold != nil {
		existing.DormancyDaysThreshold = *req.DormancyDaysThreshold
	}
	existing.DormancyChargeAmount = req.DormancyChargeAmount
	existing.MonthlyMaintenanceFee = req.MonthlyMaintenanceFee
	existing.MaxWithdrawalsPerMonth = req.MaxWithdrawalsPerMonth
	existing.Version++

	existing.InterestTiers = nil
	for _, t := range req.InterestTiers {
		existing.InterestTiers = append(existing.InterestTiers, model.DepositInterestTier{
			FromAmount: t.FromAmount,
			ToAmount:   t.ToAmount,
			Rate:       t.Rate,
		})
	}

	if err := s.repo.UpdateDepositProduct(ctx, existing); err != nil {
		return nil, err
	}

	resp := model.DepositProductToResponse(existing)
	return &resp, nil
}

// ActivateDepositProduct activates a deposit product.
func (s *DepositService) ActivateDepositProduct(ctx context.Context, id uuid.UUID, tenantID string) (*model.DepositProductResponse, error) {
	p, err := s.repo.GetDepositProductByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &NotFoundError{Msg: "deposit product not found"}
	}
	p.Status = model.DepositStatusActive
	if err := s.repo.UpdateDepositProduct(ctx, p); err != nil {
		return nil, err
	}
	resp := model.DepositProductToResponse(p)
	return &resp, nil
}

// DeactivateDepositProduct sets a deposit product to archived.
func (s *DepositService) DeactivateDepositProduct(ctx context.Context, id uuid.UUID, tenantID string) (*model.DepositProductResponse, error) {
	p, err := s.repo.GetDepositProductByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &NotFoundError{Msg: "deposit product not found"}
	}
	p.Status = model.DepositStatusArchived
	if err := s.repo.UpdateDepositProduct(ctx, p); err != nil {
		return nil, err
	}
	resp := model.DepositProductToResponse(p)
	return &resp, nil
}

func decimalOrZero(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}
