package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type LoanProxyService struct {
	productClient     *client.ProductClient
	originationClient *client.LoanOriginationClient
	managementClient  *client.LoanManagementClient
	authService       *AuthService
}

func NewLoanProxyService(
	productClient *client.ProductClient,
	originationClient *client.LoanOriginationClient,
	managementClient *client.LoanManagementClient,
	authService *AuthService,
) *LoanProxyService {
	return &LoanProxyService{
		productClient:     productClient,
		originationClient: originationClient,
		managementClient:  managementClient,
		authService:       authService,
	}
}

func (s *LoanProxyService) GetLoanProducts(ctx context.Context) ([]map[string]any, error) {
	products, err := s.productClient.GetLoanProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get loan products: %w", err)
	}
	if products == nil {
		products = []map[string]any{}
	}
	return products, nil
}

type LoanApplicationRequest struct {
	ProductID       string  `json:"productId"`
	RequestedAmount float64 `json:"requestedAmount"`
	TenorMonths     int     `json:"tenorMonths"`
	Purpose         string  `json:"purpose"`
}

func (s *LoanProxyService) ApplyForLoan(ctx context.Context, customerID string, req LoanApplicationRequest) (map[string]any, error) {
	if req.ProductID == "" {
		return nil, apperrors.BadRequest("productId is required")
	}
	if req.RequestedAmount <= 0 {
		return nil, apperrors.BadRequest("requestedAmount must be positive")
	}

	result, err := s.originationClient.ApplyForLoan(ctx, map[string]any{
		"productId":       req.ProductID,
		"customerId":      customerID,
		"requestedAmount": req.RequestedAmount,
		"tenorMonths":     req.TenorMonths,
		"purpose":         req.Purpose,
	})
	if err != nil {
		return nil, fmt.Errorf("apply for loan: %w", err)
	}
	return result, nil
}

func (s *LoanProxyService) GetActiveLoans(ctx context.Context, customerID string) ([]map[string]any, error) {
	loans, err := s.managementClient.GetActiveLoans(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("get active loans: %w", err)
	}
	if loans == nil {
		loans = []map[string]any{}
	}
	return loans, nil
}

func (s *LoanProxyService) GetLoanSchedule(ctx context.Context, loanID string) (map[string]any, error) {
	schedule, err := s.managementClient.GetLoanSchedule(ctx, loanID)
	if err != nil {
		return nil, fmt.Errorf("get loan schedule: %w", err)
	}
	return schedule, nil
}

type LoanRepaymentRequest struct {
	LoanID string  `json:"loanId"`
	Amount float64 `json:"amount"`
	Pin    string  `json:"pin"`
}

func (s *LoanProxyService) MakeRepayment(ctx context.Context, userID uuid.UUID, customerID string, req LoanRepaymentRequest) (map[string]any, error) {
	if req.LoanID == "" {
		return nil, apperrors.BadRequest("loanId is required")
	}
	if req.Amount <= 0 {
		return nil, apperrors.BadRequest("amount must be positive")
	}

	// Verify PIN
	if err := s.authService.VerifyPINForUser(ctx, userID, req.Pin); err != nil {
		return nil, err
	}

	result, err := s.managementClient.MakeRepayment(ctx, map[string]any{
		"loanId":     req.LoanID,
		"customerId": customerID,
		"amount":     req.Amount,
	})
	if err != nil {
		return nil, fmt.Errorf("make repayment: %w", err)
	}
	return result, nil
}
