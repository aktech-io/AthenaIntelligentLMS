package service

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/client"
	"github.com/athena-lms/go-services/internal/bff/shop/model"
	"github.com/athena-lms/go-services/internal/bff/shop/repository"
	"github.com/athena-lms/go-services/internal/common/errors"
)

type BNPLService struct {
	bnplRepo      *repository.BNPLRepo
	scoringClient *client.ScoringClient
}

func NewBNPLService(bnplRepo *repository.BNPLRepo, scoringClient *client.ScoringClient) *BNPLService {
	return &BNPLService{
		bnplRepo:      bnplRepo,
		scoringClient: scoringClient,
	}
}

func (s *BNPLService) ListPlans(ctx context.Context, tenantID string) ([]model.BnplPlanResponse, error) {
	plans, err := s.bnplRepo.FindAllActive(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	resp := make([]model.BnplPlanResponse, len(plans))
	for i := range plans {
		resp[i] = plans[i].ToResponse()
	}
	return resp, nil
}

func (s *BNPLService) CheckEligibility(ctx context.Context, tenantID, customerID string) (*model.BnplEligibilityResponse, error) {
	creditScore, err := s.scoringClient.GetCreditScore(ctx, customerID)
	if err != nil {
		return nil, err
	}

	plans, err := s.bnplRepo.FindActiveByMinCreditScore(ctx, tenantID, creditScore)
	if err != nil {
		return nil, err
	}

	planResponses := make([]model.BnplPlanResponse, len(plans))
	for i := range plans {
		planResponses[i] = plans[i].ToResponse()
	}

	eligible := len(plans) > 0
	message := "You are eligible for BNPL plans"
	if !eligible {
		message = "You are not eligible for BNPL plans at this time"
	}

	return &model.BnplEligibilityResponse{
		Eligible:       eligible,
		CreditScore:    creditScore,
		AvailablePlans: planResponses,
		Message:        message,
	}, nil
}

func (s *BNPLService) Calculate(ctx context.Context, planID uuid.UUID, amount float64) (*model.BnplCalculationResponse, error) {
	plan, err := s.bnplRepo.FindByID(ctx, planID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("BNPL Plan", planID.String())
		}
		return nil, err
	}

	deposit := amount * plan.DepositPercentage / 100
	financed := amount - deposit
	interest := financed * plan.InterestRate / 100
	fee := financed * plan.ProcessingFeeRate / 100
	totalPayable := financed + interest + fee
	monthly := 0.0
	if plan.DurationMonths > 0 {
		monthly = totalPayable / float64(plan.DurationMonths)
	}

	return &model.BnplCalculationResponse{
		PlanName:          plan.PlanName,
		Amount:            amount,
		Deposit:           deposit,
		AmountFinanced:    financed,
		InterestRate:      plan.InterestRate,
		Interest:          interest,
		ProcessingFeeRate: plan.ProcessingFeeRate,
		ProcessingFee:     fee,
		TotalPayable:      totalPayable,
		MonthlyPayment:    monthly,
		DurationMonths:    plan.DurationMonths,
	}, nil
}
