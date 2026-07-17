package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	"github.com/athena-lms/go-services/internal/bff/gateway/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type DashboardService struct {
	userRepo        *repository.UserRepo
	accountClient   *client.AccountClient
	notifClient     *client.NotificationClient
	overdraftClient *client.OverdraftClient
}

func NewDashboardService(
	userRepo *repository.UserRepo,
	accountClient *client.AccountClient,
	notifClient *client.NotificationClient,
	overdraftClient *client.OverdraftClient,
) *DashboardService {
	return &DashboardService{
		userRepo:        userRepo,
		accountClient:   accountClient,
		notifClient:     notifClient,
		overdraftClient: overdraftClient,
	}
}

type DashboardResponse struct {
	Balance             map[string]any   `json:"balance"`
	RecentTransactions  []map[string]any `json:"recentTransactions"`
	UnreadNotifications int64            `json:"unreadNotifications"`
	OverdraftStatus     map[string]any   `json:"overdraftStatus"`
	User                map[string]any   `json:"user"`
}

func (s *DashboardService) GetDashboard(ctx context.Context, userID uuid.UUID, customerID string) (*DashboardResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil, apperrors.NotFoundResource("User", userID.String())
	}

	resp := &DashboardResponse{
		User: map[string]any{
			"id":          user.ID,
			"phoneNumber": user.PhoneNumber,
			"fullName":    user.FullName,
			"status":      user.Status,
			"kycTier":     user.KYCTier,
		},
	}

	// Fetch balance (best effort)
	balance, err := s.accountClient.GetBalance(ctx, customerID)
	if err != nil {
		slog.Warn("failed to fetch balance for dashboard", "customerId", customerID, "error", err)
		// Leave resp.Balance as nil so JSON encodes to null, not {}
	} else {
		resp.Balance = balance
	}

	// Fetch recent transactions (best effort)
	txns, err := s.accountClient.GetTransactions(ctx, customerID, 0, 5)
	if err != nil {
		slog.Warn("failed to fetch transactions for dashboard", "customerId", customerID, "error", err)
		resp.RecentTransactions = []map[string]any{}
	} else {
		// Extract the content list from the paginated response
		if content, ok := txns["content"]; ok {
			if list, ok := content.([]any); ok {
				items := make([]map[string]any, 0, len(list))
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						items = append(items, m)
					}
				}
				resp.RecentTransactions = items
			} else {
				resp.RecentTransactions = []map[string]any{}
			}
		} else {
			resp.RecentTransactions = []map[string]any{}
		}
	}

	// Fetch unread notification count (best effort)
	unread, err := s.notifClient.GetUnreadCount(ctx, userID.String())
	if err != nil {
		slog.Warn("failed to fetch unread count for dashboard", "userId", userID, "error", err)
	}
	resp.UnreadNotifications = unread

	// Fetch overdraft status (best effort)
	wallet, err := s.overdraftClient.GetWalletByCustomerID(ctx, customerID)
	if err != nil {
		slog.Warn("failed to fetch overdraft for dashboard", "customerId", customerID, "error", err)
		// Leave resp.OverdraftStatus as nil so JSON encodes to null, not {}
	} else {
		resp.OverdraftStatus = wallet
	}

	return resp, nil
}
