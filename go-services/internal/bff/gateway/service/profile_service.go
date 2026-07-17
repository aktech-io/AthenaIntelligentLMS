package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
	"github.com/athena-lms/go-services/internal/bff/gateway/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type ProfileService struct {
	userRepo       *repository.UserRepo
	preferenceRepo *repository.PreferenceRepo
	employmentRepo *repository.EmploymentRepo
}

func NewProfileService(
	userRepo *repository.UserRepo,
	preferenceRepo *repository.PreferenceRepo,
	employmentRepo *repository.EmploymentRepo,
) *ProfileService {
	return &ProfileService{
		userRepo:       userRepo,
		preferenceRepo: preferenceRepo,
		employmentRepo: employmentRepo,
	}
}

func (s *ProfileService) GetProfile(ctx context.Context, userID uuid.UUID) (*model.ProfileResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil, apperrors.NotFoundResource("User", userID.String())
	}
	resp := user.ToProfileResponse()
	return &resp, nil
}

type UpdateProfileRequest struct {
	FullName        *string `json:"fullName"`
	Email           *string `json:"email"`
	ProfileImageURL *string `json:"profileImageUrl"`
	DateOfBirth     *string `json:"dateOfBirth"`
}

func (s *ProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateProfileRequest) (*model.ProfileResponse, error) {
	if err := s.userRepo.UpdateProfile(ctx, userID, req.FullName, req.Email, req.ProfileImageURL, req.DateOfBirth); err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return s.GetProfile(ctx, userID)
}

func (s *ProfileService) GetPreferences(ctx context.Context, userID uuid.UUID) (*model.UserPreference, error) {
	pref, err := s.preferenceRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find preferences: %w", err)
	}
	if pref == nil {
		// Return defaults
		return &model.UserPreference{
			UserID:         userID,
			PushEnabled:    true,
			SMSEnabled:     true,
			EmailEnabled:   true,
			Theme:          "LIGHT",
			BalanceVisible: true,
		}, nil
	}
	return pref, nil
}

type UpdatePreferencesRequest struct {
	PushEnabled    *bool   `json:"pushEnabled"`
	SMSEnabled     *bool   `json:"smsEnabled"`
	EmailEnabled   *bool   `json:"emailEnabled"`
	Theme          *string `json:"theme"`
	BalanceVisible *bool   `json:"balanceVisible"`
}

func (s *ProfileService) UpdatePreferences(ctx context.Context, userID uuid.UUID, tenantID string, req UpdatePreferencesRequest) (*model.UserPreference, error) {
	existing, err := s.preferenceRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find preferences: %w", err)
	}

	pref := &model.UserPreference{
		TenantID:       tenantID,
		UserID:         userID,
		PushEnabled:    true,
		SMSEnabled:     true,
		EmailEnabled:   true,
		Theme:          "LIGHT",
		BalanceVisible: true,
	}
	if existing != nil {
		pref = existing
	}

	if req.PushEnabled != nil {
		pref.PushEnabled = *req.PushEnabled
	}
	if req.SMSEnabled != nil {
		pref.SMSEnabled = *req.SMSEnabled
	}
	if req.EmailEnabled != nil {
		pref.EmailEnabled = *req.EmailEnabled
	}
	if req.Theme != nil {
		pref.Theme = *req.Theme
	}
	if req.BalanceVisible != nil {
		pref.BalanceVisible = *req.BalanceVisible
	}

	if err := s.preferenceRepo.Upsert(ctx, pref); err != nil {
		return nil, fmt.Errorf("upsert preferences: %w", err)
	}

	return s.GetPreferences(ctx, userID)
}

func (s *ProfileService) GetEmployment(ctx context.Context, userID uuid.UUID) (*model.UserEmployment, error) {
	emp, err := s.employmentRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find employment: %w", err)
	}
	if emp == nil {
		return &model.UserEmployment{UserID: userID}, nil
	}
	return emp, nil
}

type UpdateEmploymentRequest struct {
	EmployerName     *string  `json:"employerName"`
	JobTitle         *string  `json:"jobTitle"`
	MonthlyIncome    *float64 `json:"monthlyIncome"`
	EmploymentStatus *string  `json:"employmentStatus"`
}

func (s *ProfileService) UpdateEmployment(ctx context.Context, userID uuid.UUID, tenantID string, req UpdateEmploymentRequest) (*model.UserEmployment, error) {
	existing, err := s.employmentRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find employment: %w", err)
	}

	emp := &model.UserEmployment{
		TenantID: tenantID,
		UserID:   userID,
	}
	if existing != nil {
		emp = existing
	}

	if req.EmployerName != nil {
		emp.EmployerName = req.EmployerName
	}
	if req.JobTitle != nil {
		emp.JobTitle = req.JobTitle
	}
	if req.MonthlyIncome != nil {
		emp.MonthlyIncome = req.MonthlyIncome
	}
	if req.EmploymentStatus != nil {
		emp.EmploymentStatus = req.EmploymentStatus
	}

	if err := s.employmentRepo.Upsert(ctx, emp); err != nil {
		return nil, fmt.Errorf("upsert employment: %w", err)
	}

	return s.GetEmployment(ctx, userID)
}
