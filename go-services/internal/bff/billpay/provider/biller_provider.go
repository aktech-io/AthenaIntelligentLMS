package provider

import (
	"fmt"
	"log/slog"
	"regexp"

	"github.com/google/uuid"
)

// ValidationResult represents the result of biller account validation.
type ValidationResult struct {
	Valid       bool   `json:"valid"`
	AccountName string `json:"accountName,omitempty"`
	BillerName  string `json:"billerName"`
	BillerCode  string `json:"billerCode"`
	Message     string `json:"message,omitempty"`
}

// PaymentResult represents the result of a biller payment.
type PaymentResult struct {
	Success         bool   `json:"success"`
	BillerReference string `json:"billerReference,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
}

// BillerProvider simulates external biller API integrations.
type BillerProvider struct{}

func NewBillerProvider() *BillerProvider {
	return &BillerProvider{}
}

// ValidateAccount validates an account number with the biller.
// Uses regex validation if defined, then simulates a provider lookup.
func (p *BillerProvider) ValidateAccount(billerCode, billerName, accountNumber string, validationRegex *string) ValidationResult {
	// Check regex first if defined.
	if validationRegex != nil && *validationRegex != "" {
		re, err := regexp.Compile(*validationRegex)
		if err != nil {
			slog.Warn("invalid biller validation regex", "billerCode", billerCode, "regex", *validationRegex)
		} else if !re.MatchString(accountNumber) {
			return ValidationResult{
				Valid:      false,
				BillerName: billerName,
				BillerCode: billerCode,
				Message:    "invalid account number format",
			}
		}
	}

	// Simulate provider validation based on biller code.
	var accountName string
	switch billerCode {
	case "KPLC_PREPAID", "KPLC_POSTPAID":
		accountName = fmt.Sprintf("KPLC Customer %s", accountNumber[:4])
	case "DSTV":
		accountName = fmt.Sprintf("DSTV Subscriber %s", accountNumber[:4])
	default:
		accountName = fmt.Sprintf("Account Holder %s", accountNumber[:min(4, len(accountNumber))])
	}

	slog.Info("biller account validated", "billerCode", billerCode, "accountNumber", accountNumber)
	return ValidationResult{
		Valid:       true,
		AccountName: accountName,
		BillerName:  billerName,
		BillerCode:  billerCode,
		Message:     "account validated successfully",
	}
}

// ProcessPayment simulates a payment to the biller provider.
func (p *BillerProvider) ProcessPayment(billerCode, accountNumber string, amount float64) PaymentResult {
	// Simulate successful payment with a generated reference.
	ref := fmt.Sprintf("%s-%s", billerCode, uuid.New().String()[:8])

	slog.Info("biller payment processed",
		"billerCode", billerCode,
		"accountNumber", accountNumber,
		"amount", amount,
		"reference", ref,
	)
	return PaymentResult{
		Success:         true,
		BillerReference: ref,
	}
}
