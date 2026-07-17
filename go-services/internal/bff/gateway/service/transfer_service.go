package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	"github.com/athena-lms/go-services/internal/bff/gateway/publisher"
	"github.com/athena-lms/go-services/internal/bff/gateway/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type TransferService struct {
	userRepo       *repository.UserRepo
	accountClient  *client.AccountClient
	authService    *AuthService
	eventPublisher *publisher.EventPublisher
}

func NewTransferService(
	userRepo *repository.UserRepo,
	accountClient *client.AccountClient,
	authService *AuthService,
	eventPublisher *publisher.EventPublisher,
) *TransferService {
	return &TransferService{
		userRepo:       userRepo,
		accountClient:  accountClient,
		authService:    authService,
		eventPublisher: eventPublisher,
	}
}

type SendTransferRequest struct {
	RecipientPhone string  `json:"recipientPhone"`
	Amount         float64 `json:"amount"`
	Pin            string  `json:"pin"`
	Description    string  `json:"description"`
}

type TransferResponse struct {
	TransactionID  string  `json:"transactionId"`
	Status         string  `json:"status"`
	Amount         float64 `json:"amount"`
	RecipientPhone string  `json:"recipientPhone"`
	RecipientName  string  `json:"recipientName"`
	Reference      string  `json:"reference"`
	Message        string  `json:"message"`
}

func (s *TransferService) SendMoney(ctx context.Context, senderUserID uuid.UUID, senderCustomerID, tenantID string, req SendTransferRequest) (*TransferResponse, error) {
	if req.RecipientPhone == "" {
		return nil, apperrors.BadRequest("recipientPhone is required")
	}
	if req.Amount <= 0 {
		return nil, apperrors.BadRequest("amount must be positive")
	}
	if req.Pin == "" {
		return nil, apperrors.BadRequest("pin is required")
	}

	// Verify PIN
	if err := s.authService.VerifyPINForUser(ctx, senderUserID, req.Pin); err != nil {
		return nil, err
	}

	// Look up sender
	sender, err := s.userRepo.FindByID(ctx, senderUserID)
	if err != nil {
		return nil, fmt.Errorf("find sender: %w", err)
	}
	if sender == nil {
		return nil, apperrors.NotFoundResource("Sender", senderUserID.String())
	}

	// Look up recipient
	recipient, err := s.userRepo.FindByPhoneAndTenant(ctx, req.RecipientPhone, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find recipient: %w", err)
	}
	if recipient == nil {
		return nil, apperrors.BadRequest("recipient not found")
	}

	reference := generateReference("TXF")

	// Resolve account IDs
	senderAccountID, err := s.accountClient.ResolveAccountID(ctx, sender.CustomerID)
	if err != nil {
		slog.Error("failed to resolve sender account", "customerId", sender.CustomerID, "error", err)
		s.publishTransferFailed(tenantID, senderUserID, req.RecipientPhone, req.Amount, reference, "failed to resolve sender account")
		return nil, apperrors.BadRequest("could not resolve sender account")
	}

	recipientAccountID, err := s.accountClient.ResolveAccountID(ctx, recipient.CustomerID)
	if err != nil {
		slog.Error("failed to resolve recipient account", "customerId", recipient.CustomerID, "error", err)
		s.publishTransferFailed(tenantID, senderUserID, req.RecipientPhone, req.Amount, reference, "failed to resolve recipient account")
		return nil, apperrors.BadRequest("could not resolve recipient account")
	}

	// Debit sender
	_, err = s.accountClient.DebitAccount(ctx, senderAccountID, map[string]any{
		"amount":      req.Amount,
		"reference":   reference,
		"description": req.Description,
		"type":        "TRANSFER",
	})
	if err != nil {
		slog.Error("debit failed", "accountId", senderAccountID, "error", err)
		s.publishTransferFailed(tenantID, senderUserID, req.RecipientPhone, req.Amount, reference, "debit failed: "+err.Error())
		return nil, apperrors.BadRequest("transfer failed: insufficient funds or account error")
	}

	// Credit recipient
	_, err = s.accountClient.CreditAccount(ctx, recipientAccountID, map[string]any{
		"amount":      req.Amount,
		"reference":   reference,
		"description": fmt.Sprintf("Transfer from %s", sender.PhoneNumber),
		"type":        "TRANSFER",
	})
	if err != nil {
		slog.Error("credit failed", "accountId", recipientAccountID, "error", err)
		// Attempt to reverse debit
		_, reverseErr := s.accountClient.CreditAccount(ctx, senderAccountID, map[string]any{
			"amount":      req.Amount,
			"reference":   reference + "-REVERSAL",
			"description": "Transfer reversal",
			"type":        "REVERSAL",
		})
		if reverseErr != nil {
			slog.Error("reversal failed", "error", reverseErr)
		}
		s.publishTransferFailed(tenantID, senderUserID, req.RecipientPhone, req.Amount, reference, "credit failed")
		return nil, apperrors.BadRequest("transfer failed during credit")
	}

	// Publish success event
	s.eventPublisher.PublishTransferCompleted(tenantID, map[string]any{
		"senderUserId":        senderUserID.String(),
		"senderCustomerId":    sender.CustomerID,
		"recipientUserId":     recipient.ID.String(),
		"recipientCustomerId": recipient.CustomerID,
		"amount":              req.Amount,
		"reference":           reference,
	})

	recipientName := ""
	if recipient.FullName != nil {
		recipientName = *recipient.FullName
	}

	return &TransferResponse{
		TransactionID:  uuid.New().String(),
		Status:         "COMPLETED",
		Amount:         req.Amount,
		RecipientPhone: req.RecipientPhone,
		RecipientName:  recipientName,
		Reference:      reference,
		Message:        "Transfer successful",
	}, nil
}

func (s *TransferService) publishTransferFailed(tenantID string, senderUserID uuid.UUID, recipientPhone string, amount float64, reference, reason string) {
	s.eventPublisher.PublishTransferFailed(tenantID, map[string]any{
		"senderUserId":   senderUserID.String(),
		"recipientPhone": recipientPhone,
		"amount":         amount,
		"reference":      reference,
		"reason":         reason,
	})
}

func generateReference(prefix string) string {
	id := uuid.New().String()
	return prefix + "-" + strings.ToUpper(strings.ReplaceAll(id[:8], "-", ""))
}
