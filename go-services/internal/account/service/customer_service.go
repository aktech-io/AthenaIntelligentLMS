package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/event"
	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// CustomerService provides customer business logic.
// Port of Java CustomerService.java.
type CustomerService struct {
	repo      *repository.Repository
	publisher *event.Publisher
	logger    *zap.Logger
	auditor   *audit.Logger
}

// NewCustomerService creates a new CustomerService.
func NewCustomerService(repo *repository.Repository, publisher *event.Publisher, logger *zap.Logger) *CustomerService {
	return &CustomerService{repo: repo, publisher: publisher, logger: logger, auditor: audit.New(repo, logger)}
}

// CreateCustomerRequest is the DTO for customer creation.
type CreateCustomerRequest struct {
	CustomerID   string  `json:"customerId"`
	FirstName    string  `json:"firstName"`
	LastName     string  `json:"lastName"`
	Email        *string `json:"email,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	DateOfBirth  *string `json:"dateOfBirth,omitempty"`
	NationalID   *string `json:"nationalId,omitempty"`
	Gender       *string `json:"gender,omitempty"`
	Address      *string `json:"address,omitempty"`
	CustomerType *string `json:"customerType,omitempty"`
	Source       *string `json:"source,omitempty"`
}

// CreateCustomer creates a new customer record.
func (s *CustomerService) CreateCustomer(ctx context.Context, req CreateCustomerRequest, tenantID string) (*model.Customer, error) {
	if req.CustomerID == "" {
		return nil, errors.BadRequest("customerId is required")
	}
	if req.FirstName == "" {
		return nil, errors.BadRequest("firstName is required")
	}
	if req.LastName == "" {
		return nil, errors.BadRequest("lastName is required")
	}

	exists, err := s.repo.CustomerExistsByCustomerIDAndTenant(ctx, req.CustomerID, tenantID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.BadRequest("Customer ID already exists: " + req.CustomerID)
	}

	custType := model.CustomerTypeIndividual
	if req.CustomerType != nil {
		upper := strings.ToUpper(*req.CustomerType)
		if !model.ValidCustomerType(upper) {
			return nil, errors.BadRequest("Invalid customer type: " + *req.CustomerType)
		}
		custType = model.CustomerType(upper)
	}

	source := "BRANCH"
	if req.Source != nil {
		source = *req.Source
	}

	var dob *time.Time
	if req.DateOfBirth != nil && *req.DateOfBirth != "" {
		t, err := time.Parse("2006-01-02", *req.DateOfBirth)
		if err != nil {
			return nil, errors.BadRequest("Invalid dateOfBirth format, expected YYYY-MM-DD")
		}
		dob = &t
	}

	customer := &model.Customer{
		TenantID:     tenantID,
		CustomerID:   req.CustomerID,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		Phone:        req.Phone,
		DateOfBirth:  dob,
		NationalID:   req.NationalID,
		Gender:       req.Gender,
		Address:      req.Address,
		CustomerType: custType,
		Status:       model.CustomerStatusActive,
		KycStatus:    "PENDING",
		Source:       source,
	}

	if err := s.repo.CreateCustomer(ctx, customer); err != nil {
		return nil, err
	}

	s.publisher.PublishCustomerCreated(ctx, customer.ID, customer.CustomerID, tenantID)
	s.logger.Info("Created customer",
		zap.String("customerId", customer.CustomerID),
		zap.String("id", customer.ID.String()),
		zap.String("tenantId", tenantID))

	return customer, nil
}

// GetCustomer fetches a customer by PK.
func (s *CustomerService) GetCustomer(ctx context.Context, id uuid.UUID, tenantID string) (*model.Customer, error) {
	c, err := s.repo.GetCustomerByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Customer", id)
		}
		return nil, err
	}
	return c, nil
}

// ListCustomers returns paginated customers.
func (s *CustomerService) ListCustomers(ctx context.Context, tenantID string, page, size int) (dto.PageResponse, error) {
	customers, total, err := s.repo.ListCustomersByTenant(ctx, tenantID, size, page*size)
	if err != nil {
		return dto.PageResponse{}, err
	}
	return dto.NewPageResponse(customers, page, size, total), nil
}

// UpdateCustomerRequest is the DTO for partial customer updates.
type UpdateCustomerRequest struct {
	FirstName    *string `json:"firstName,omitempty"`
	LastName     *string `json:"lastName,omitempty"`
	Email        *string `json:"email,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	DateOfBirth  *string `json:"dateOfBirth,omitempty"`
	NationalID   *string `json:"nationalId,omitempty"`
	Gender       *string `json:"gender,omitempty"`
	Address      *string `json:"address,omitempty"`
	CustomerType *string `json:"customerType,omitempty"`
}

// UpdateCustomer applies partial updates to a customer.
func (s *CustomerService) UpdateCustomer(ctx context.Context, id uuid.UUID, req UpdateCustomerRequest, tenantID string) (*model.Customer, error) {
	customer, err := s.repo.GetCustomerByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Customer", id)
		}
		return nil, err
	}

	if req.FirstName != nil {
		customer.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		customer.LastName = *req.LastName
	}
	if req.Email != nil {
		customer.Email = req.Email
	}
	if req.Phone != nil {
		customer.Phone = req.Phone
	}
	if req.DateOfBirth != nil {
		if *req.DateOfBirth != "" {
			t, err := time.Parse("2006-01-02", *req.DateOfBirth)
			if err != nil {
				return nil, errors.BadRequest("Invalid dateOfBirth format")
			}
			customer.DateOfBirth = &t
		}
	}
	if req.NationalID != nil {
		customer.NationalID = req.NationalID
	}
	if req.Gender != nil {
		customer.Gender = req.Gender
	}
	if req.Address != nil {
		customer.Address = req.Address
	}
	if req.CustomerType != nil {
		upper := strings.ToUpper(*req.CustomerType)
		if !model.ValidCustomerType(upper) {
			return nil, errors.BadRequest("Invalid customer type: " + *req.CustomerType)
		}
		customer.CustomerType = model.CustomerType(upper)
	}

	if err := s.repo.UpdateCustomer(ctx, customer); err != nil {
		return nil, err
	}

	s.publisher.PublishCustomerUpdated(ctx, customer.ID, customer.CustomerID, tenantID)
	return customer, nil
}

// UpdateCustomerStatus changes a customer's status.
func (s *CustomerService) UpdateCustomerStatus(ctx context.Context, id uuid.UUID, status, tenantID string) (*model.Customer, error) {
	customer, err := s.repo.GetCustomerByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Customer", id)
		}
		return nil, err
	}

	upper := strings.ToUpper(status)
	if !model.ValidCustomerStatus(upper) {
		return nil, errors.BadRequest("Invalid customer status: " + status)
	}

	customer.Status = model.CustomerStatus(upper)
	if err := s.repo.UpdateCustomer(ctx, customer); err != nil {
		return nil, err
	}
	return customer, nil
}

// UpdateKycStatus sets a customer's KYC status (e.g. PENDING -> VERIFIED).
func (s *CustomerService) UpdateKycStatus(ctx context.Context, id uuid.UUID, kycStatus, tenantID string) (*model.Customer, error) {
	customer, err := s.repo.GetCustomerByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Customer", id)
		}
		return nil, err
	}
	upper := strings.ToUpper(strings.TrimSpace(kycStatus))
	switch upper {
	case "PENDING", "VERIFIED", "REJECTED":
	default:
		return nil, errors.BadRequest("Invalid KYC status: " + kycStatus + " (expected PENDING, VERIFIED or REJECTED)")
	}
	prev := customer.KycStatus
	customer.KycStatus = upper
	if err := s.repo.UpdateCustomer(ctx, customer); err != nil {
		return nil, err
	}
	s.auditor.Record(ctx, "CUSTOMER_KYC_UPDATE", "CUSTOMER", id.String(),
		map[string]any{"kycStatus": prev}, map[string]any{"kycStatus": upper}, nil)
	return customer, nil
}

// GetByCustomerID fetches a customer by business customer_id.
func (s *CustomerService) GetByCustomerID(ctx context.Context, customerID, tenantID string) (*model.Customer, error) {
	c, err := s.repo.GetCustomerByCustomerIDAndTenant(ctx, customerID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Customer", customerID)
		}
		return nil, err
	}
	return c, nil
}

// SearchCustomers searches customers by name, phone, email, or customer_id.
func (s *CustomerService) SearchCustomers(ctx context.Context, q, tenantID string, limit int) ([]*model.Customer, error) {
	return s.repo.SearchCustomers(ctx, tenantID, q, limit)
}
