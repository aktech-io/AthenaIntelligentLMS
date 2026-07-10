package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ApplicationStatus represents the status of a loan application.
type ApplicationStatus string

const (
	StatusDraft       ApplicationStatus = "DRAFT"
	StatusSubmitted   ApplicationStatus = "SUBMITTED"
	StatusUnderReview ApplicationStatus = "UNDER_REVIEW"
	StatusApproved    ApplicationStatus = "APPROVED"
	StatusRejected    ApplicationStatus = "REJECTED"
	StatusDisbursed   ApplicationStatus = "DISBURSED"
	StatusCancelled   ApplicationStatus = "CANCELLED"
)

// ValidApplicationStatuses lists all valid statuses.
var ValidApplicationStatuses = map[ApplicationStatus]bool{
	StatusDraft:       true,
	StatusSubmitted:   true,
	StatusUnderReview: true,
	StatusApproved:    true,
	StatusRejected:    true,
	StatusDisbursed:   true,
	StatusCancelled:   true,
}

// RiskGrade represents the risk classification of an application.
type RiskGrade string

const (
	RiskGradeAPlus RiskGrade = "A_PLUS"
	RiskGradeA     RiskGrade = "A"
	RiskGradeBPlus RiskGrade = "B_PLUS"
	RiskGradeB     RiskGrade = "B"
	RiskGradeC     RiskGrade = "C"
	RiskGradeD     RiskGrade = "D"
	RiskGradeE     RiskGrade = "E"
)

// ValidRiskGrades lists all valid risk grades.
var ValidRiskGrades = map[RiskGrade]bool{
	RiskGradeAPlus: true,
	RiskGradeA:     true,
	RiskGradeBPlus: true,
	RiskGradeB:     true,
	RiskGradeC:     true,
	RiskGradeD:     true,
	RiskGradeE:     true,
}

// CollateralType represents the type of collateral.
type CollateralType string

const (
	CollateralRealEstate      CollateralType = "REAL_ESTATE"
	CollateralVehicle         CollateralType = "VEHICLE"
	CollateralEquipment       CollateralType = "EQUIPMENT"
	CollateralCashDeposit     CollateralType = "CASH_DEPOSIT"
	CollateralShares          CollateralType = "SHARES"
	CollateralInsurancePolicy CollateralType = "INSURANCE_POLICY"
	CollateralOther           CollateralType = "OTHER"
)

// ValidCollateralTypes lists all valid collateral types.
var ValidCollateralTypes = map[CollateralType]bool{
	CollateralRealEstate:      true,
	CollateralVehicle:         true,
	CollateralEquipment:       true,
	CollateralCashDeposit:     true,
	CollateralShares:          true,
	CollateralInsurancePolicy: true,
	CollateralOther:           true,
}

// LoanApplication represents a loan application entity.
type LoanApplication struct {
	ID                  uuid.UUID         `json:"id"`
	TenantID            string            `json:"tenantId"`
	CustomerID          string            `json:"customerId"`
	ProductID           uuid.UUID         `json:"productId"`
	RequestedAmount     decimal.Decimal   `json:"requestedAmount"`
	ApprovedAmount      *decimal.Decimal  `json:"approvedAmount"`
	Currency            string            `json:"currency"`
	TenorMonths         int               `json:"tenorMonths"`
	Purpose             *string           `json:"purpose"`
	Status              ApplicationStatus `json:"status"`
	RiskGrade           *RiskGrade        `json:"riskGrade"`
	CreditScore         *int              `json:"creditScore"`
	InterestRate        *decimal.Decimal  `json:"interestRate"`
	DepositAmount       decimal.Decimal   `json:"depositAmount"`
	DisbursedAmount     *decimal.Decimal  `json:"disbursedAmount"`
	DisbursedAt         *time.Time        `json:"disbursedAt"`
	DisbursementAccount *string           `json:"disbursementAccount"`
	ReviewerID          *string           `json:"reviewerId,omitempty"`
	ReviewedAt          *time.Time        `json:"reviewedAt,omitempty"`
	ReviewNotes         *string           `json:"reviewNotes"`
	CreatedAt           *time.Time        `json:"createdAt"`
	UpdatedAt           *time.Time        `json:"updatedAt"`
	CreatedBy           *string           `json:"createdBy,omitempty"`
	UpdatedBy           *string           `json:"updatedBy,omitempty"`
}

// ApplicationCollateral represents collateral attached to a loan application.
type ApplicationCollateral struct {
	ID             uuid.UUID       `json:"id"`
	ApplicationID  uuid.UUID       `json:"applicationId"`
	TenantID       string          `json:"tenantId"`
	CollateralType CollateralType  `json:"collateralType"`
	Description    string          `json:"description"`
	EstimatedValue decimal.Decimal `json:"estimatedValue"`
	Currency       string          `json:"currency"`
	DocumentRef    *string         `json:"documentRef"`
	CreatedAt      *time.Time      `json:"createdAt"`
}

// ApplicationNote represents a note attached to a loan application.
type ApplicationNote struct {
	ID            uuid.UUID  `json:"id"`
	ApplicationID uuid.UUID  `json:"applicationId"`
	TenantID      string     `json:"tenantId"`
	NoteType      string     `json:"noteType"`
	Content       string     `json:"content"`
	AuthorID      *string    `json:"authorId"`
	CreatedAt     *time.Time `json:"createdAt"`
}

// ApplicationStatusHistory represents a status change record.
type ApplicationStatusHistory struct {
	ID            uuid.UUID  `json:"id"`
	ApplicationID uuid.UUID  `json:"applicationId"`
	TenantID      string     `json:"tenantId"`
	FromStatus    *string    `json:"fromStatus"`
	ToStatus      string     `json:"toStatus"`
	Reason        *string    `json:"reason"`
	ChangedBy     *string    `json:"changedBy"`
	ChangedAt     *time.Time `json:"changedAt"`
}

// DisbursementFee is one fee charged (netted off) at disbursement — the
// persisted breakdown behind the loan.fee.charged events (BLOCKER-3).
type DisbursementFee struct {
	ID              uuid.UUID       `json:"id"`
	ApplicationID   uuid.UUID       `json:"applicationId"`
	TenantID        string          `json:"tenantId"`
	FeeName         string          `json:"feeName"`
	FeeType         string          `json:"feeType"`
	CalculationType string          `json:"calculationType"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Reference       string          `json:"reference"`
	CreatedAt       *time.Time      `json:"createdAt"`
}

// ---- Request DTOs ----

// CreateApplicationRequest is the DTO for creating a new loan application.
type CreateApplicationRequest struct {
	CustomerID          string           `json:"customerId"`
	ProductID           uuid.UUID        `json:"productId"`
	RequestedAmount     decimal.Decimal  `json:"requestedAmount"`
	TenorMonths         int              `json:"tenorMonths"`
	Purpose             *string          `json:"purpose"`
	Currency            string           `json:"currency"`
	DisbursementAccount *string          `json:"disbursementAccount"`
	DepositAmount       *decimal.Decimal `json:"depositAmount"`
}

// ApproveApplicationRequest is the DTO for approving a loan application.
type ApproveApplicationRequest struct {
	ApprovedAmount decimal.Decimal `json:"approvedAmount"`
	InterestRate   decimal.Decimal `json:"interestRate"`
	RiskGrade      *string         `json:"riskGrade"`
	CreditScore    *int            `json:"creditScore"`
	ReviewNotes    *string         `json:"reviewNotes"`
}

// RejectApplicationRequest is the DTO for rejecting a loan application.
type RejectApplicationRequest struct {
	Reason string `json:"reason"`
}

// DisburseRequest is the DTO for disbursing a loan application.
type DisburseRequest struct {
	DisbursedAmount     decimal.Decimal `json:"disbursedAmount"`
	DisbursementAccount string          `json:"disbursementAccount"`
}

// AddCollateralRequest is the DTO for adding collateral to an application.
type AddCollateralRequest struct {
	CollateralType CollateralType  `json:"collateralType"`
	Description    string          `json:"description"`
	EstimatedValue decimal.Decimal `json:"estimatedValue"`
	Currency       string          `json:"currency"`
	DocumentRef    *string         `json:"documentRef"`
}

// AddNoteRequest is the DTO for adding a note to an application.
type AddNoteRequest struct {
	Content  string `json:"content"`
	NoteType string `json:"noteType"`
}

// ---- Response DTOs ----

// ApplicationResponse is the response DTO for a loan application.
type ApplicationResponse struct {
	ID                  uuid.UUID               `json:"id"`
	TenantID            string                  `json:"tenantId"`
	CustomerID          string                  `json:"customerId"`
	ProductID           uuid.UUID               `json:"productId"`
	RequestedAmount     decimal.Decimal         `json:"requestedAmount"`
	ApprovedAmount      *decimal.Decimal        `json:"approvedAmount"`
	Currency            string                  `json:"currency"`
	TenorMonths         int                     `json:"tenorMonths"`
	Purpose             *string                 `json:"purpose"`
	Status              ApplicationStatus       `json:"status"`
	RiskGrade           *RiskGrade              `json:"riskGrade"`
	CreditScore         *int                    `json:"creditScore"`
	InterestRate        *decimal.Decimal        `json:"interestRate"`
	DepositAmount       decimal.Decimal         `json:"depositAmount"`
	DisbursedAmount     *decimal.Decimal        `json:"disbursedAmount"`
	DisbursedAt         *time.Time              `json:"disbursedAt"`
	DisbursementAccount *string                 `json:"disbursementAccount"`
	ReviewNotes         *string                 `json:"reviewNotes"`
	CreatedAt           *time.Time              `json:"createdAt"`
	UpdatedAt           *time.Time              `json:"updatedAt"`
	Collaterals         []CollateralResponse    `json:"collaterals"`
	Notes               []NoteResponse          `json:"notes"`
	StatusHistory       []StatusHistoryResponse `json:"statusHistory"`
}

// CollateralResponse is the response DTO for application collateral.
type CollateralResponse struct {
	ID             uuid.UUID       `json:"id"`
	CollateralType CollateralType  `json:"collateralType"`
	Description    string          `json:"description"`
	EstimatedValue decimal.Decimal `json:"estimatedValue"`
	Currency       string          `json:"currency"`
	DocumentRef    *string         `json:"documentRef"`
	CreatedAt      *time.Time      `json:"createdAt"`
}

// NoteResponse is the response DTO for an application note.
type NoteResponse struct {
	ID        uuid.UUID  `json:"id"`
	NoteType  string     `json:"noteType"`
	Content   string     `json:"content"`
	AuthorID  *string    `json:"authorId"`
	CreatedAt *time.Time `json:"createdAt"`
}

// StatusHistoryResponse is the response DTO for a status change record.
type StatusHistoryResponse struct {
	ID         uuid.UUID  `json:"id"`
	FromStatus *string    `json:"fromStatus"`
	ToStatus   string     `json:"toStatus"`
	Reason     *string    `json:"reason"`
	ChangedBy  *string    `json:"changedBy"`
	ChangedAt  *time.Time `json:"changedAt"`
}

// PageResponse is a generic paginated response.
type PageResponse struct {
	Content       []ApplicationResponse `json:"content"`
	TotalElements int64                 `json:"totalElements"`
	TotalPages    int                   `json:"totalPages"`
	Page          int                   `json:"page"`
	Size          int                   `json:"size"`
}

// ToApplicationResponse converts a LoanApplication to an ApplicationResponse.
func ToApplicationResponse(app *LoanApplication, collaterals []ApplicationCollateral, notes []ApplicationNote, history []ApplicationStatusHistory) ApplicationResponse {
	resp := ApplicationResponse{
		ID:                  app.ID,
		TenantID:            app.TenantID,
		CustomerID:          app.CustomerID,
		ProductID:           app.ProductID,
		RequestedAmount:     app.RequestedAmount,
		ApprovedAmount:      app.ApprovedAmount,
		Currency:            app.Currency,
		TenorMonths:         app.TenorMonths,
		Purpose:             app.Purpose,
		Status:              app.Status,
		RiskGrade:           app.RiskGrade,
		CreditScore:         app.CreditScore,
		InterestRate:        app.InterestRate,
		DepositAmount:       app.DepositAmount,
		DisbursedAmount:     app.DisbursedAmount,
		DisbursedAt:         app.DisbursedAt,
		DisbursementAccount: app.DisbursementAccount,
		ReviewNotes:         app.ReviewNotes,
		CreatedAt:           app.CreatedAt,
		UpdatedAt:           app.UpdatedAt,
		Collaterals:         make([]CollateralResponse, 0, len(collaterals)),
		Notes:               make([]NoteResponse, 0, len(notes)),
		StatusHistory:       make([]StatusHistoryResponse, 0, len(history)),
	}
	for _, c := range collaterals {
		resp.Collaterals = append(resp.Collaterals, CollateralResponse{
			ID:             c.ID,
			CollateralType: c.CollateralType,
			Description:    c.Description,
			EstimatedValue: c.EstimatedValue,
			Currency:       c.Currency,
			DocumentRef:    c.DocumentRef,
			CreatedAt:      c.CreatedAt,
		})
	}
	for _, n := range notes {
		resp.Notes = append(resp.Notes, NoteResponse{
			ID:        n.ID,
			NoteType:  n.NoteType,
			Content:   n.Content,
			AuthorID:  n.AuthorID,
			CreatedAt: n.CreatedAt,
		})
	}
	for _, h := range history {
		resp.StatusHistory = append(resp.StatusHistory, StatusHistoryResponse{
			ID:         h.ID,
			FromStatus: h.FromStatus,
			ToStatus:   h.ToStatus,
			Reason:     h.Reason,
			ChangedBy:  h.ChangedBy,
			ChangedAt:  h.ChangedAt,
		})
	}
	return resp
}

// ToSimpleResponse converts a LoanApplication to an ApplicationResponse without related entities.
func ToSimpleResponse(app *LoanApplication) ApplicationResponse {
	return ToApplicationResponse(app, nil, nil, nil)
}
