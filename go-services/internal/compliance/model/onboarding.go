package model

import (
	"time"

	"github.com/google/uuid"
)

// OnboardingStatus is the lifecycle of a self-service onboarding application.
type OnboardingStatus string

const (
	OnboardingReceived     OnboardingStatus = "RECEIVED"      // transient, pre-verification
	OnboardingAutoApproved OnboardingStatus = "AUTO_APPROVED" // low-risk straight-through
	OnboardingReferred     OnboardingStatus = "REFERRED"      // officer queue
	OnboardingApproved     OnboardingStatus = "APPROVED"      // officer approved a referral
	OnboardingRejected     OnboardingStatus = "REJECTED"      // officer rejected a referral
)

// RiskTier is the onboarding risk classification driving auto-approval.
type RiskTier string

const (
	TierLow    RiskTier = "LOW"
	TierMedium RiskTier = "MEDIUM"
	TierHigh   RiskTier = "HIGH"
)

// OnboardingApplication is a row in onboarding_applications.
type OnboardingApplication struct {
	ID              uuid.UUID        `json:"id"`
	TenantID        string           `json:"tenantId"`
	Phone           string           `json:"phone"`
	FullName        string           `json:"fullName"`
	NationalID      string           `json:"nationalId"`
	DateOfBirth     *string          `json:"dateOfBirth,omitempty"`
	DocumentRef     *string          `json:"documentRef,omitempty"`
	SelfieRef       *string          `json:"selfieRef,omitempty"`
	Status          OnboardingStatus `json:"status"`
	RiskTier        *RiskTier        `json:"riskTier,omitempty"`
	Provider        *string          `json:"provider,omitempty"`
	ProviderRef     *string          `json:"providerRef,omitempty"`
	DecisionReasons *string          `json:"decisionReasons,omitempty"`
	CustomerID      *string          `json:"customerId,omitempty"`
	DecidedBy       *string          `json:"decidedBy,omitempty"`
	DecidedAt       *time.Time       `json:"decidedAt,omitempty"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

// SubmitOnboardingRequest is the customer-channel submission (via the BFF).
type SubmitOnboardingRequest struct {
	Phone       string  `json:"phone"`
	FullName    string  `json:"fullName"`
	NationalID  string  `json:"nationalId"`
	DateOfBirth *string `json:"dateOfBirth,omitempty"`
	DocumentRef *string `json:"documentRef,omitempty"`
	SelfieRef   *string `json:"selfieRef,omitempty"`
	// CustomerID lets the caller bind the application to an already-created
	// customer identity; empty means the caller assigns it on approval.
	CustomerID *string `json:"customerId,omitempty"`
}

// OnboardingDecisionRequest is an officer decision on a REFERRED application.
type OnboardingDecisionRequest struct {
	Reason     string  `json:"reason"`
	CustomerID *string `json:"customerId,omitempty"` // approve: bind customer
}
