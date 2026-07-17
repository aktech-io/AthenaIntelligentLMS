package model

import (
	"time"

	"github.com/google/uuid"
)

type BNPLPlan struct {
	ID                uuid.UUID `db:"id" json:"id"`
	TenantID          string    `db:"tenant_id" json:"tenantId"`
	PlanName          string    `db:"plan_name" json:"planName"`
	DurationMonths    int       `db:"duration_months" json:"durationMonths"`
	InterestRate      float64   `db:"interest_rate" json:"interestRate"`
	ProcessingFeeRate float64   `db:"processing_fee_rate" json:"processingFeeRate"`
	MinAmount         float64   `db:"min_amount" json:"minAmount"`
	MaxAmount         float64   `db:"max_amount" json:"maxAmount"`
	MinCreditScore    int       `db:"min_credit_score" json:"minCreditScore"`
	DepositPercentage float64   `db:"deposit_percentage" json:"depositPercentage"`
	Active            bool      `db:"active" json:"active"`
	CreatedAt         time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt         time.Time `db:"updated_at" json:"updatedAt"`
}

type BnplPlanResponse struct {
	ID                uuid.UUID `json:"id"`
	PlanName          string    `json:"planName"`
	DurationMonths    int       `json:"durationMonths"`
	InterestRate      float64   `json:"interestRate"`
	ProcessingFeeRate float64   `json:"processingFeeRate"`
	MinAmount         float64   `json:"minAmount"`
	MaxAmount         float64   `json:"maxAmount"`
	MinCreditScore    int       `json:"minCreditScore"`
	DepositPercentage float64   `json:"depositPercentage"`
}

func (p *BNPLPlan) ToResponse() BnplPlanResponse {
	return BnplPlanResponse{
		ID:                p.ID,
		PlanName:          p.PlanName,
		DurationMonths:    p.DurationMonths,
		InterestRate:      p.InterestRate,
		ProcessingFeeRate: p.ProcessingFeeRate,
		MinAmount:         p.MinAmount,
		MaxAmount:         p.MaxAmount,
		MinCreditScore:    p.MinCreditScore,
		DepositPercentage: p.DepositPercentage,
	}
}

type BnplCalculationResponse struct {
	PlanName          string  `json:"planName"`
	Amount            float64 `json:"amount"`
	Deposit           float64 `json:"deposit"`
	AmountFinanced    float64 `json:"amountFinanced"`
	InterestRate      float64 `json:"interestRate"`
	Interest          float64 `json:"interest"`
	ProcessingFeeRate float64 `json:"processingFeeRate"`
	ProcessingFee     float64 `json:"processingFee"`
	TotalPayable      float64 `json:"totalPayable"`
	MonthlyPayment    float64 `json:"monthlyPayment"`
	DurationMonths    int     `json:"durationMonths"`
}

type BnplEligibilityResponse struct {
	Eligible       bool               `json:"eligible"`
	CreditScore    int                `json:"creditScore"`
	AvailablePlans []BnplPlanResponse `json:"availablePlans"`
	Message        string             `json:"message"`
}
