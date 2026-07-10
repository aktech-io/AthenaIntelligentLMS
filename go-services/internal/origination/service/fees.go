package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/origination/client"
)

// Fee/calculation type literals for disbursement-time charges. The product
// fee types mirror product-service's FeeType enum; ProcessingFeeType marks the
// product-level processing fee (ProcessingFeeRate/Min/Max), which is not a
// ProductFee row.
const (
	FeeTypeUpfront      = "UPFRONT"
	FeeTypeDisbursement = "DISBURSEMENT"
	ProcessingFeeType   = "PROCESSING"

	CalcTypeFlat       = "FLAT"
	CalcTypePercentage = "PERCENTAGE"

	processingFeeName = "Processing Fee"
)

var oneHundred = decimal.NewFromInt(100)

// ComputedFee is one fee resolved to a concrete amount at disbursement time.
type ComputedFee struct {
	FeeName         string
	FeeType         string
	CalculationType string
	Amount          decimal.Decimal
}

// ComputeDisbursementFees resolves all fees due at disbursement:
//
//   - the product processing fee: disbursedAmount × processingFeeRate/100,
//     clamped to [processingFeeMin, processingFeeMax] (max only when set);
//     skipped entirely when rate and min are both zero;
//   - every MANDATORY ProductFee with feeType UPFRONT or DISBURSEMENT:
//     FLAT → amount; PERCENTAGE → disbursedAmount × rate/100.
//
// Percentage results are rounded to 2dp, half up. Fees that resolve to a
// non-positive amount are dropped. Returns the fee lines and their total.
func ComputeDisbursementFees(disbursedAmount decimal.Decimal, cfg *client.FeeConfig) ([]ComputedFee, decimal.Decimal) {
	total := decimal.Zero
	if cfg == nil {
		return nil, total
	}

	var fees []ComputedFee

	// Product-level processing fee.
	if !cfg.ProcessingFeeRate.IsZero() || !cfg.ProcessingFeeMin.IsZero() {
		fee := percentOf(disbursedAmount, cfg.ProcessingFeeRate)
		if fee.LessThan(cfg.ProcessingFeeMin) {
			fee = cfg.ProcessingFeeMin
		}
		if cfg.ProcessingFeeMax != nil && fee.GreaterThan(*cfg.ProcessingFeeMax) {
			fee = *cfg.ProcessingFeeMax
		}
		if fee.IsPositive() {
			fees = append(fees, ComputedFee{
				FeeName:         processingFeeName,
				FeeType:         ProcessingFeeType,
				CalculationType: CalcTypePercentage,
				Amount:          fee,
			})
			total = total.Add(fee)
		}
	}

	// Mandatory UPFRONT / DISBURSEMENT product fee lines.
	for _, pf := range cfg.Fees {
		if !pf.IsMandatory {
			continue
		}
		feeType := strings.ToUpper(pf.FeeType)
		if feeType != FeeTypeUpfront && feeType != FeeTypeDisbursement {
			continue
		}

		var amount decimal.Decimal
		switch strings.ToUpper(pf.CalculationType) {
		case CalcTypeFlat:
			if pf.Amount != nil {
				amount = *pf.Amount
			}
		case CalcTypePercentage:
			if pf.Rate != nil {
				amount = percentOf(disbursedAmount, *pf.Rate)
			}
		}
		if !amount.IsPositive() {
			continue
		}

		fees = append(fees, ComputedFee{
			FeeName:         pf.FeeName,
			FeeType:         feeType,
			CalculationType: strings.ToUpper(pf.CalculationType),
			Amount:          amount,
		})
		total = total.Add(amount)
	}

	return fees, total
}

// ValidateFeeTotal enforces the net-off guard: the fees must leave a positive
// amount to credit the borrower.
func ValidateFeeTotal(disbursedAmount, totalFees decimal.Decimal) error {
	if totalFees.GreaterThanOrEqual(disbursedAmount) {
		return fmt.Errorf("total upfront fees %s equal or exceed the disbursed amount %s; nothing would be credited to the borrower",
			totalFees.StringFixed(2), disbursedAmount.StringFixed(2))
	}
	return nil
}

// percentOf computes base × rate/100 rounded to 2dp (half up — shopspring
// Round is half-away-from-zero, which is half up for positive amounts).
func percentOf(base, rate decimal.Decimal) decimal.Decimal {
	return base.Mul(rate).Div(oneHundred).Round(2)
}
