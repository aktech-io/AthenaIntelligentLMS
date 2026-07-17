package service

import (
	"context"
	"fmt"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type TopUpService struct {
	paymentClient *client.PaymentClient
	accountClient *client.AccountClient
}

func NewTopUpService(paymentClient *client.PaymentClient, accountClient *client.AccountClient) *TopUpService {
	return &TopUpService{
		paymentClient: paymentClient,
		accountClient: accountClient,
	}
}

type TopUpRequest struct {
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"paymentMethod"`
	PhoneNumber   string  `json:"phoneNumber"`
	CardToken     string  `json:"cardToken"`
	BankAccountID string  `json:"bankAccountId"`
}

func (s *TopUpService) TopUp(ctx context.Context, customerID string, req TopUpRequest) (map[string]any, error) {
	if req.Amount < 1 {
		return nil, apperrors.BadRequest("amount must be at least 1")
	}

	reference := generateReference("TOPUP")

	// Initiate payment — LMS expects paymentType + paymentChannel
	paymentResp, err := s.paymentClient.InitiatePayment(ctx, map[string]any{
		"amount":         req.Amount,
		"paymentType":    "FLOAT_TRANSFER",
		"paymentChannel": req.PaymentMethod,
		"phoneNumber":    req.PhoneNumber,
		"cardToken":      req.CardToken,
		"bankAccountId":  req.BankAccountID,
		"reference":      reference,
		"type":           "TOP_UP",
		"customerId":     customerID,
	})
	if err != nil {
		return nil, fmt.Errorf("initiate payment: %w", err)
	}

	// Credit account
	accountID, err := s.accountClient.ResolveAccountID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("resolve account: %w", err)
	}

	_, err = s.accountClient.CreditAccount(ctx, accountID, map[string]any{
		"amount":      req.Amount,
		"reference":   reference,
		"description": "Top-up via " + req.PaymentMethod,
		"type":        "TOP_UP",
	})
	if err != nil {
		return nil, fmt.Errorf("credit account: %w", err)
	}

	result := map[string]any{
		"status":    "COMPLETED",
		"reference": reference,
		"amount":    req.Amount,
		"message":   "Top-up successful",
	}
	if paymentResp != nil {
		for k, v := range paymentResp {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result, nil
}
