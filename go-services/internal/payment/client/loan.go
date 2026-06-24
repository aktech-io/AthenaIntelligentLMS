package client

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/google/uuid"
)

// LoanManagementClient validates loans before accepting payments.
// Fails open on infrastructure errors so payments aren't blocked by
// loan-management downtime.
type LoanManagementClient struct {
	client  *httputil.ServiceClient
	baseURL string
	logger  *zap.Logger
}

// NewLoanManagementClient creates a new loan management client.
func NewLoanManagementClient(serviceKey string, logger *zap.Logger) *LoanManagementClient {
	baseURL := os.Getenv("LOAN_MANAGEMENT_URL")
	if baseURL == "" {
		baseURL = "http://loan-management-service.lms.svc.cluster.local:8089"
	}
	return &LoanManagementClient{
		client:  httputil.NewServiceClient(serviceKey),
		baseURL: baseURL,
		logger:  logger,
	}
}

// ValidateLoanExists checks that a loan exists and is eligible for payment.
// Returns nil if loanID is nil (loanId is optional on payments).
func (c *LoanManagementClient) ValidateLoanExists(ctx context.Context, loanID *uuid.UUID) error {
	if loanID == nil {
		return nil
	}

	url := fmt.Sprintf("%s/api/v1/loans/%s", c.baseURL, loanID.String())
	var result map[string]any
	if err := c.client.Get(ctx, url, &result); err != nil {
		// Fail open: log and proceed
		c.logger.Warn("Loan management unavailable, skipping loanId validation",
			zap.String("loanId", loanID.String()),
			zap.Error(err),
		)
		return nil
	}

	status, _ := result["status"].(string)
	if status == "CLOSED" || status == "WRITTEN_OFF" {
		return errors.NewBusinessError(
			fmt.Sprintf("Loan %s is not eligible for payment (status=%s)", loanID, status),
		)
	}

	return nil
}
