package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type OverdraftProxyService struct {
	overdraftClient *client.OverdraftClient
	authService     *AuthService
}

func NewOverdraftProxyService(
	overdraftClient *client.OverdraftClient,
	authService *AuthService,
) *OverdraftProxyService {
	return &OverdraftProxyService{
		overdraftClient: overdraftClient,
		authService:     authService,
	}
}

func (s *OverdraftProxyService) GetOverdraftStatus(ctx context.Context, customerID string) (map[string]any, error) {
	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		slog.Warn("no wallet found for customer", "customerId", customerID, "error", err)
		return map[string]any{
			"hasWallet":   false,
			"hasFacility": false,
		}, nil
	}

	walletID, _ := wallet["id"].(string)
	result := map[string]any{
		"hasWallet": true,
		"balance":   wallet["balance"],
		"currency":  wallet["currency"],
	}

	// Try to get facility
	facility, err := s.overdraftClient.GetOverdraftFacility(ctx, walletID)
	if err != nil {
		result["hasFacility"] = false
		return result, nil
	}

	result["hasFacility"] = true
	result["overdraftLimit"] = facility["creditLimit"]
	result["usedAmount"] = facility["usedAmount"]
	result["availableAmount"] = facility["availableAmount"]
	result["apr"] = facility["apr"]
	result["dailyRate"] = facility["dailyRate"]
	result["accruedInterest"] = facility["accruedInterest"]
	result["scoreBand"] = facility["scoreBand"]

	return result, nil
}

func (s *OverdraftProxyService) SetupOverdraft(ctx context.Context, customerID, tenantID string) (map[string]any, error) {
	// Create wallet if not exists
	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		// Create wallet
		wallet, err = s.overdraftClient.CreateWallet(ctx, map[string]any{
			"customerId": customerID,
			"tenantId":   tenantID,
			"currency":   "KES",
		})
		if err != nil {
			return nil, fmt.Errorf("create wallet: %w", err)
		}
	}

	walletID, _ := wallet["id"].(string)

	// Apply for overdraft facility
	facility, err := s.overdraftClient.ApplyOverdraft(ctx, map[string]any{
		"walletId":   walletID,
		"customerId": customerID,
		"tenantId":   tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("apply overdraft: %w", err)
	}

	return facility, nil
}

type OverdraftDepositRequest struct {
	Amount      float64 `json:"amount"`
	Reference   string  `json:"reference"`
	Description string  `json:"description"`
	Pin         string  `json:"pin"`
}

func (s *OverdraftProxyService) Deposit(ctx context.Context, userID uuid.UUID, customerID string, req OverdraftDepositRequest) (map[string]any, error) {
	if req.Amount < 1 {
		return nil, apperrors.BadRequest("amount must be at least 1")
	}

	// Verify PIN
	if err := s.authService.VerifyPINForUser(ctx, userID, req.Pin); err != nil {
		return nil, err
	}

	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		return nil, apperrors.BadRequest("wallet not found")
	}

	walletID, _ := wallet["id"].(string)
	return s.overdraftClient.Deposit(ctx, walletID, map[string]any{
		"amount":      req.Amount,
		"reference":   req.Reference,
		"description": req.Description,
	})
}

type OverdraftWithdrawRequest struct {
	Amount      float64 `json:"amount"`
	Reference   string  `json:"reference"`
	Description string  `json:"description"`
	Pin         string  `json:"pin"`
}

func (s *OverdraftProxyService) Withdraw(ctx context.Context, userID uuid.UUID, customerID string, req OverdraftWithdrawRequest) (map[string]any, error) {
	if req.Amount <= 0 {
		return nil, apperrors.BadRequest("amount must be positive")
	}

	// Verify PIN
	if err := s.authService.VerifyPINForUser(ctx, userID, req.Pin); err != nil {
		return nil, err
	}

	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		return nil, apperrors.BadRequest("wallet not found")
	}

	walletID, _ := wallet["id"].(string)
	return s.overdraftClient.Withdraw(ctx, walletID, map[string]any{
		"amount":      req.Amount,
		"reference":   req.Reference,
		"description": req.Description,
	})
}

func (s *OverdraftProxyService) GetTransactions(ctx context.Context, customerID string, page, size int) (map[string]any, error) {
	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		return nil, apperrors.BadRequest("wallet not found")
	}

	walletID, _ := wallet["id"].(string)
	return s.overdraftClient.GetTransactions(ctx, walletID, page, size)
}

func (s *OverdraftProxyService) SuspendOverdraft(ctx context.Context, customerID string) (map[string]any, error) {
	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		return nil, apperrors.BadRequest("wallet not found")
	}

	walletID, _ := wallet["id"].(string)
	facility, err := s.overdraftClient.GetOverdraftFacility(ctx, walletID)
	if err != nil {
		return nil, apperrors.BadRequest("no overdraft facility found")
	}

	facilityID, _ := facility["id"].(string)
	return s.overdraftClient.SuspendOverdraft(ctx, facilityID)
}

func (s *OverdraftProxyService) GetCharges(ctx context.Context, customerID string) ([]map[string]any, error) {
	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		return nil, apperrors.BadRequest("wallet not found")
	}

	walletID, _ := wallet["id"].(string)
	facility, err := s.overdraftClient.GetOverdraftFacility(ctx, walletID)
	if err != nil {
		return nil, apperrors.BadRequest("no overdraft facility found")
	}

	facilityID, _ := facility["id"].(string)
	return s.overdraftClient.GetCharges(ctx, facilityID)
}
