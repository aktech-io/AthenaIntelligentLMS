package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

type LoanStatus string

const (
	LoanStatusActive       LoanStatus = "ACTIVE"
	LoanStatusRestructured LoanStatus = "RESTRUCTURED"
	LoanStatusClosed       LoanStatus = "CLOSED"
	LoanStatusWrittenOff   LoanStatus = "WRITTEN_OFF"
)

type LoanStage string

const (
	LoanStagePerforming  LoanStage = "PERFORMING"
	LoanStageWatch       LoanStage = "WATCH"
	LoanStageSubstandard LoanStage = "SUBSTANDARD"
	LoanStageDoubtful    LoanStage = "DOUBTFUL"
	LoanStageLoss        LoanStage = "LOSS"
)

type RepaymentFrequency string

const (
	FrequencyDaily    RepaymentFrequency = "DAILY"
	FrequencyWeekly   RepaymentFrequency = "WEEKLY"
	FrequencyBiweekly RepaymentFrequency = "BIWEEKLY"
	FrequencyMonthly  RepaymentFrequency = "MONTHLY"
)

type ScheduleType string

const (
	ScheduleTypeEMI      ScheduleType = "EMI"
	ScheduleTypeFlatRate ScheduleType = "FLAT_RATE"
)

type InstallmentStatus string

const (
	InstallmentPending InstallmentStatus = "PENDING"
	InstallmentPartial InstallmentStatus = "PARTIAL"
	InstallmentPaid    InstallmentStatus = "PAID"
)

// ---------------------------------------------------------------------------
// Entities
// ---------------------------------------------------------------------------

// Loan maps to the "loans" table.
type Loan struct {
	ID                   uuid.UUID          `json:"id"`
	TenantID             string             `json:"tenantId"`
	ApplicationID        uuid.UUID          `json:"applicationId"`
	CustomerID           string             `json:"customerId"`
	ProductID            uuid.UUID          `json:"productId"`
	DisbursedAmount      decimal.Decimal    `json:"disbursedAmount"`
	OutstandingPrincipal decimal.Decimal    `json:"outstandingPrincipal"`
	OutstandingInterest  decimal.Decimal    `json:"outstandingInterest"`
	OutstandingFees      decimal.Decimal    `json:"outstandingFees"`
	OutstandingPenalty   decimal.Decimal    `json:"outstandingPenalty"`
	Currency             string             `json:"currency"`
	InterestRate         decimal.Decimal    `json:"interestRate"`
	TenorMonths          int                `json:"tenorMonths"`
	RepaymentFrequency   RepaymentFrequency `json:"repaymentFrequency"`
	ScheduleType         ScheduleType       `json:"scheduleType"`
	DisbursedAt          time.Time          `json:"disbursedAt"`
	FirstRepaymentDate   time.Time          `json:"firstRepaymentDate"`
	MaturityDate         time.Time          `json:"maturityDate"`
	Status               LoanStatus         `json:"status"`
	Stage                LoanStage          `json:"stage"`
	DPD                  int                `json:"dpd"`
	LastRepaymentDate    *time.Time         `json:"lastRepaymentDate,omitempty"`
	LastRepaymentAmount  *decimal.Decimal   `json:"lastRepaymentAmount,omitempty"`
	ClosedAt             *time.Time         `json:"closedAt,omitempty"`
	CreatedAt            time.Time          `json:"createdAt"`
	UpdatedAt            time.Time          `json:"updatedAt"`
}

// LoanSchedule maps to the "loan_schedules" table.
type LoanSchedule struct {
	ID            uuid.UUID         `json:"id"`
	LoanID        uuid.UUID         `json:"loanId"`
	TenantID      string            `json:"tenantId"`
	InstallmentNo int               `json:"installmentNo"`
	DueDate       time.Time         `json:"dueDate"`
	PrincipalDue  decimal.Decimal   `json:"principalDue"`
	InterestDue   decimal.Decimal   `json:"interestDue"`
	FeeDue        decimal.Decimal   `json:"feeDue"`
	PenaltyDue    decimal.Decimal   `json:"penaltyDue"`
	TotalDue      decimal.Decimal   `json:"totalDue"`
	PrincipalPaid decimal.Decimal   `json:"principalPaid"`
	InterestPaid  decimal.Decimal   `json:"interestPaid"`
	FeePaid       decimal.Decimal   `json:"feePaid"`
	PenaltyPaid   decimal.Decimal   `json:"penaltyPaid"`
	TotalPaid     decimal.Decimal   `json:"totalPaid"`
	Status        InstallmentStatus `json:"status"`
	PaidDate      *time.Time        `json:"paidDate,omitempty"`
}

// LoanRepayment maps to the "loan_repayments" table.
type LoanRepayment struct {
	ID               uuid.UUID       `json:"id"`
	LoanID           uuid.UUID       `json:"loanId"`
	TenantID         string          `json:"tenantId"`
	Amount           decimal.Decimal `json:"amount"`
	Currency         string          `json:"currency"`
	PenaltyApplied   decimal.Decimal `json:"penaltyApplied"`
	FeeApplied       decimal.Decimal `json:"feeApplied"`
	InterestApplied  decimal.Decimal `json:"interestApplied"`
	PrincipalApplied decimal.Decimal `json:"principalApplied"`
	// UnallocatedAmount is any surplus left after allocating to every
	// installment and to loan-level outstanding principal (overpayment).
	UnallocatedAmount decimal.Decimal `json:"unallocatedAmount"`
	PaymentReference  sql.NullString  `json:"paymentReference"`
	PaymentMethod     sql.NullString  `json:"paymentMethod"`
	PaymentDate       time.Time       `json:"paymentDate"`
	CreatedAt         time.Time       `json:"createdAt"`
	CreatedBy         sql.NullString  `json:"createdBy"`
}

// LoanDpdHistory maps to the "loan_dpd_history" table.
type LoanDpdHistory struct {
	ID           uuid.UUID `json:"id"`
	LoanID       uuid.UUID `json:"loanId"`
	TenantID     string    `json:"tenantId"`
	DPD          int       `json:"dpd"`
	Stage        string    `json:"stage"`
	SnapshotDate time.Time `json:"snapshotDate"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ---------------------------------------------------------------------------
// DTOs (request / response)
// ---------------------------------------------------------------------------

// RepaymentRequest is the inbound DTO for applying a repayment.
type RepaymentRequest struct {
	Amount           decimal.Decimal `json:"amount"`
	PaymentDate      *string         `json:"paymentDate,omitempty"` // YYYY-MM-DD, defaults to today
	PaymentReference string          `json:"paymentReference,omitempty"`
	PaymentMethod    string          `json:"paymentMethod,omitempty"`
	Currency         string          `json:"currency,omitempty"`
}

// RestructureRequest is the inbound DTO for restructuring a loan.
type RestructureRequest struct {
	NewTenorMonths  int             `json:"newTenorMonths"`
	NewInterestRate decimal.Decimal `json:"newInterestRate"`
	NewFrequency    *string         `json:"newFrequency,omitempty"` // optional override
	Reason          string          `json:"reason"`
}

// LoanResponse is the outbound DTO for a loan.
type LoanResponse struct {
	ID                   uuid.UUID          `json:"id"`
	TenantID             string             `json:"tenantId"`
	ApplicationID        uuid.UUID          `json:"applicationId"`
	CustomerID           string             `json:"customerId"`
	ProductID            uuid.UUID          `json:"productId"`
	DisbursedAmount      decimal.Decimal    `json:"disbursedAmount"`
	OutstandingPrincipal decimal.Decimal    `json:"outstandingPrincipal"`
	OutstandingInterest  decimal.Decimal    `json:"outstandingInterest"`
	OutstandingFees      decimal.Decimal    `json:"outstandingFees"`
	OutstandingPenalty   decimal.Decimal    `json:"outstandingPenalty"`
	TotalOutstanding     decimal.Decimal    `json:"totalOutstanding"`
	Currency             string             `json:"currency"`
	InterestRate         decimal.Decimal    `json:"interestRate"`
	TenorMonths          int                `json:"tenorMonths"`
	RepaymentFrequency   RepaymentFrequency `json:"repaymentFrequency"`
	ScheduleType         ScheduleType       `json:"scheduleType"`
	DisbursedAt          time.Time          `json:"disbursedAt"`
	FirstRepaymentDate   string             `json:"firstRepaymentDate"`
	MaturityDate         string             `json:"maturityDate"`
	Status               LoanStatus         `json:"status"`
	Stage                LoanStage          `json:"stage"`
	DPD                  int                `json:"dpd"`
	LastRepaymentDate    *string            `json:"lastRepaymentDate,omitempty"`
	LastRepaymentAmount  *decimal.Decimal   `json:"lastRepaymentAmount,omitempty"`
	CreatedAt            time.Time          `json:"createdAt"`
}

// InstallmentResponse is the outbound DTO for a schedule installment.
type InstallmentResponse struct {
	ID            uuid.UUID       `json:"id"`
	InstallmentNo int             `json:"installmentNo"`
	DueDate       string          `json:"dueDate"`
	PrincipalDue  decimal.Decimal `json:"principalDue"`
	InterestDue   decimal.Decimal `json:"interestDue"`
	FeeDue        decimal.Decimal `json:"feeDue"`
	PenaltyDue    decimal.Decimal `json:"penaltyDue"`
	TotalDue      decimal.Decimal `json:"totalDue"`
	PrincipalPaid decimal.Decimal `json:"principalPaid"`
	InterestPaid  decimal.Decimal `json:"interestPaid"`
	FeePaid       decimal.Decimal `json:"feePaid"`
	PenaltyPaid   decimal.Decimal `json:"penaltyPaid"`
	TotalPaid     decimal.Decimal `json:"totalPaid"`
	Balance       decimal.Decimal `json:"balance"`
	Status        string          `json:"status"`
	PaidDate      *string         `json:"paidDate,omitempty"`
}

// RepaymentResponse is the outbound DTO for a repayment.
type RepaymentResponse struct {
	ID               uuid.UUID       `json:"id"`
	Status           string          `json:"status"`
	Amount           decimal.Decimal `json:"amount"`
	Currency         string          `json:"currency"`
	PenaltyApplied   decimal.Decimal `json:"penaltyApplied"`
	FeeApplied       decimal.Decimal `json:"feeApplied"`
	InterestApplied  decimal.Decimal `json:"interestApplied"`
	PrincipalApplied decimal.Decimal `json:"principalApplied"`
	// UnallocatedAmount surfaces any overpayment surplus that could not be
	// allocated to the loan, so it is visible instead of silently swallowed.
	UnallocatedAmount decimal.Decimal `json:"unallocatedAmount"`
	PaymentReference  string          `json:"paymentReference"`
	PaymentMethod     string          `json:"paymentMethod"`
	PaymentDate       string          `json:"paymentDate"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// DpdResponse is the outbound DTO for DPD info.
type DpdResponse struct {
	LoanID      uuid.UUID `json:"loanId"`
	DPD         int       `json:"dpd"`
	Stage       LoanStage `json:"stage"`
	Description string    `json:"description"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ClassifyStage returns the loan stage for a given DPD value.
func ClassifyStage(dpd int) LoanStage {
	switch {
	case dpd == 0:
		return LoanStagePerforming
	case dpd <= 30:
		return LoanStageWatch
	case dpd <= 60:
		return LoanStageSubstandard
	case dpd <= 90:
		return LoanStageDoubtful
	default:
		return LoanStageLoss
	}
}

// StageDescription returns a human-readable description for a loan stage.
func StageDescription(stage LoanStage) string {
	switch stage {
	case LoanStagePerforming:
		return "Current - DPD 0"
	case LoanStageWatch:
		return "Watch - DPD 1-30"
	case LoanStageSubstandard:
		return "Substandard - DPD 31-60"
	case LoanStageDoubtful:
		return "Doubtful - DPD 61-90"
	case LoanStageLoss:
		return "Loss - DPD > 90"
	default:
		return "Unknown"
	}
}

// ToLoanResponse converts a Loan entity to LoanResponse DTO.
func ToLoanResponse(loan *Loan) LoanResponse {
	totalOutstanding := loan.OutstandingPrincipal.
		Add(loan.OutstandingInterest).
		Add(loan.OutstandingFees).
		Add(loan.OutstandingPenalty)

	resp := LoanResponse{
		ID:                   loan.ID,
		TenantID:             loan.TenantID,
		ApplicationID:        loan.ApplicationID,
		CustomerID:           loan.CustomerID,
		ProductID:            loan.ProductID,
		DisbursedAmount:      loan.DisbursedAmount,
		OutstandingPrincipal: loan.OutstandingPrincipal,
		OutstandingInterest:  loan.OutstandingInterest,
		OutstandingFees:      loan.OutstandingFees,
		OutstandingPenalty:   loan.OutstandingPenalty,
		TotalOutstanding:     totalOutstanding,
		Currency:             loan.Currency,
		InterestRate:         loan.InterestRate,
		TenorMonths:          loan.TenorMonths,
		RepaymentFrequency:   loan.RepaymentFrequency,
		ScheduleType:         loan.ScheduleType,
		DisbursedAt:          loan.DisbursedAt,
		FirstRepaymentDate:   loan.FirstRepaymentDate.Format("2006-01-02"),
		MaturityDate:         loan.MaturityDate.Format("2006-01-02"),
		Status:               loan.Status,
		Stage:                loan.Stage,
		DPD:                  loan.DPD,
		LastRepaymentAmount:  loan.LastRepaymentAmount,
		CreatedAt:            loan.CreatedAt,
	}
	if loan.LastRepaymentDate != nil {
		s := loan.LastRepaymentDate.Format("2006-01-02")
		resp.LastRepaymentDate = &s
	}
	return resp
}

// ToInstallmentResponse converts a LoanSchedule entity to InstallmentResponse DTO.
func ToInstallmentResponse(s *LoanSchedule) InstallmentResponse {
	resp := InstallmentResponse{
		ID:            s.ID,
		InstallmentNo: s.InstallmentNo,
		DueDate:       s.DueDate.Format("2006-01-02"),
		PrincipalDue:  s.PrincipalDue,
		InterestDue:   s.InterestDue,
		FeeDue:        s.FeeDue,
		PenaltyDue:    s.PenaltyDue,
		TotalDue:      s.TotalDue,
		PrincipalPaid: s.PrincipalPaid,
		InterestPaid:  s.InterestPaid,
		FeePaid:       s.FeePaid,
		PenaltyPaid:   s.PenaltyPaid,
		TotalPaid:     s.TotalPaid,
		Balance:       s.TotalDue.Sub(s.TotalPaid),
		Status:        string(s.Status),
	}
	if s.PaidDate != nil {
		pd := s.PaidDate.Format("2006-01-02")
		resp.PaidDate = &pd
	}
	return resp
}

// ToRepaymentResponse converts a LoanRepayment entity to RepaymentResponse DTO.
func ToRepaymentResponse(r *LoanRepayment) RepaymentResponse {
	return RepaymentResponse{
		ID:                r.ID,
		Status:            "COMPLETED",
		Amount:            r.Amount,
		Currency:          r.Currency,
		PenaltyApplied:    r.PenaltyApplied,
		FeeApplied:        r.FeeApplied,
		InterestApplied:   r.InterestApplied,
		PrincipalApplied:  r.PrincipalApplied,
		UnallocatedAmount: r.UnallocatedAmount,
		PaymentReference:  r.PaymentReference.String,
		PaymentMethod:     r.PaymentMethod.String,
		PaymentDate:       r.PaymentDate.Format("2006-01-02"),
		CreatedAt:         r.CreatedAt,
	}
}
