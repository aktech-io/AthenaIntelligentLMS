package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/common/auth"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/invoice/model"
)

// Seller-facing helpers (SPEC §6 "Auth modes"). A seller is the SME customer
// that uploaded the invoice; it is identified by the account-service customer
// business id (Invoice.SellerCustomerID), which the mobile BFF stamps into the
// token's `customerId` claim.

// Owner returns the tenant-scoped invoice for an ownership check (NotFound
// when it does not exist in the caller's tenant).
func (s *Service) Owner(ctx context.Context, id uuid.UUID) (*model.Invoice, error) {
	return s.repo.GetInvoice(ctx, nil, auth.TenantIDOrDefault(ctx), id)
}

// SellerProfileExists reports whether account-service knows the seller's
// customer record. A mobile-only user whose mobile.user.registered event has
// not (yet) created the customer gets false → the API answers 409
// SELLER_PROFILE_MISSING so the client app can start onboarding.
func (s *Service) SellerProfileExists(ctx context.Context, sellerID string) (bool, error) {
	sellerID = strings.TrimSpace(sellerID)
	if sellerID == "" {
		return false, nil
	}
	c, err := s.clients.Accounts.GetCustomer(ctx, sellerID)
	if err != nil {
		return false, fmt.Errorf("account-service lookup: %w", err)
	}
	return c != nil, nil
}

// SellerSummary is the seller dashboard aggregate.
func (s *Service) SellerSummary(ctx context.Context, sellerID string) (*model.SellerSummary, error) {
	sum, err := s.repo.SellerSummary(ctx, auth.TenantIDOrDefault(ctx), sellerID)
	if err != nil {
		return nil, err
	}
	sum.Currency = s.cfg.Currency
	return sum, nil
}

// Document returns the stored invoice file (memory cache, else media-service).
func (s *Service) Document(ctx context.Context, id uuid.UUID) (fileName string, data []byte, err error) {
	inv, err := s.Owner(ctx, id)
	if err != nil {
		return "", nil, err
	}
	data, err = s.fileBytes(ctx, inv)
	if err != nil {
		if errors.Is(err, errNoMedia) {
			return "", nil, cerrors.NotFound("invoice document not available")
		}
		return "", nil, err
	}
	return inv.FileName, data, nil
}
