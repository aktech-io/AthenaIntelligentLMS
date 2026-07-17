package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
	"github.com/athena-lms/go-services/internal/bff/gateway/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type ContactService struct {
	contactRepo *repository.ContactRepo
	userRepo    *repository.UserRepo
}

func NewContactService(contactRepo *repository.ContactRepo, userRepo *repository.UserRepo) *ContactService {
	return &ContactService{contactRepo: contactRepo, userRepo: userRepo}
}

func (s *ContactService) GetRecentContacts(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.ContactResponse, int64, error) {
	contacts, err := s.contactRepo.FindRecent(ctx, tenantID, userID, page, size)
	if err != nil {
		return nil, 0, fmt.Errorf("find recent contacts: %w", err)
	}
	total, err := s.contactRepo.CountByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count contacts: %w", err)
	}
	responses := make([]model.ContactResponse, len(contacts))
	for i := range contacts {
		responses[i] = contacts[i].ToResponse()
	}
	return responses, total, nil
}

type CreateContactRequest struct {
	ContactName string `json:"contactName"`
	PhoneNumber string `json:"phoneNumber"`
	IsFavorite  bool   `json:"isFavorite"`
}

func (s *ContactService) CreateContact(ctx context.Context, tenantID string, userID uuid.UUID, req CreateContactRequest) (*model.ContactResponse, error) {
	if req.ContactName == "" || req.PhoneNumber == "" {
		return nil, apperrors.BadRequest("contactName and phoneNumber are required")
	}

	// Check if the contact is an Athena user
	isAthena := false
	existingUser, err := s.userRepo.FindByPhoneAndTenant(ctx, req.PhoneNumber, tenantID)
	if err == nil && existingUser != nil {
		isAthena = true
	}

	contact := &model.UserContact{
		TenantID:     tenantID,
		UserID:       userID,
		ContactName:  req.ContactName,
		PhoneNumber:  req.PhoneNumber,
		IsAthenaUser: isAthena,
		IsFavorite:   req.IsFavorite,
	}

	if err := s.contactRepo.Create(ctx, contact); err != nil {
		return nil, fmt.Errorf("create contact: %w", err)
	}
	resp := contact.ToResponse()
	return &resp, nil
}

func (s *ContactService) SearchContacts(ctx context.Context, tenantID string, userID uuid.UUID, query string, page, size int) ([]model.ContactResponse, int64, error) {
	contacts, err := s.contactRepo.Search(ctx, tenantID, userID, query, page, size)
	if err != nil {
		return nil, 0, fmt.Errorf("search contacts: %w", err)
	}
	total, err := s.contactRepo.CountSearch(ctx, tenantID, userID, query)
	if err != nil {
		return nil, 0, fmt.Errorf("count search: %w", err)
	}
	responses := make([]model.ContactResponse, len(contacts))
	for i := range contacts {
		responses[i] = contacts[i].ToResponse()
	}
	return responses, total, nil
}
