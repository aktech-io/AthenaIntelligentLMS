package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/origination/client"
)

func d(s string) decimal.Decimal {
	v, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return v
}

func dp(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

func TestComputeDisbursementFees_NilConfig(t *testing.T) {
	fees, total := ComputeDisbursementFees(d("10000"), nil)
	assert.Empty(t, fees)
	assert.True(t, total.IsZero())
}

func TestComputeDisbursementFees_ZeroFeeProduct(t *testing.T) {
	cfg := &client.FeeConfig{} // rate 0, min 0, no fee lines
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	assert.Empty(t, fees, "zero-fee product must produce no fee lines (and hence no events)")
	assert.True(t, total.IsZero())
}

func TestComputeDisbursementFees_ProcessingFeeRateOnly(t *testing.T) {
	cfg := &client.FeeConfig{ProcessingFeeRate: d("2.5")}
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "Processing Fee", fees[0].FeeName)
	assert.Equal(t, ProcessingFeeType, fees[0].FeeType)
	assert.Equal(t, CalcTypePercentage, fees[0].CalculationType)
	assert.Equal(t, "250.00", fees[0].Amount.StringFixed(2))
	assert.Equal(t, "250.00", total.StringFixed(2))
}

func TestComputeDisbursementFees_ProcessingFeeClampedToMin(t *testing.T) {
	cfg := &client.FeeConfig{ProcessingFeeRate: d("1"), ProcessingFeeMin: d("50")}
	fees, total := ComputeDisbursementFees(d("1000"), cfg) // 1% = 10 < min 50
	require.Len(t, fees, 1)
	assert.Equal(t, "50.00", fees[0].Amount.StringFixed(2))
	assert.Equal(t, "50.00", total.StringFixed(2))
}

func TestComputeDisbursementFees_ProcessingFeeClampedToMax(t *testing.T) {
	cfg := &client.FeeConfig{ProcessingFeeRate: d("5"), ProcessingFeeMax: dp("1000")}
	fees, _ := ComputeDisbursementFees(d("100000"), cfg) // 5% = 5000 > max 1000
	require.Len(t, fees, 1)
	assert.Equal(t, "1000.00", fees[0].Amount.StringFixed(2))
}

func TestComputeDisbursementFees_NoMaxMeansUnclamped(t *testing.T) {
	cfg := &client.FeeConfig{ProcessingFeeRate: d("5")} // max nil
	fees, _ := ComputeDisbursementFees(d("100000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "5000.00", fees[0].Amount.StringFixed(2))
}

func TestComputeDisbursementFees_ZeroRatePositiveMinStillCharges(t *testing.T) {
	// The skip rule applies only when rate AND min are BOTH zero.
	cfg := &client.FeeConfig{ProcessingFeeMin: d("100")}
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "100.00", fees[0].Amount.StringFixed(2))
	assert.Equal(t, "100.00", total.StringFixed(2))
}

func TestComputeDisbursementFees_PercentageRounding2dpHalfUp(t *testing.T) {
	// 0.5555% of 1000 = 5.555 → 5.56 (half up)
	cfg := &client.FeeConfig{ProcessingFeeRate: d("0.5555")}
	fees, _ := ComputeDisbursementFees(d("1000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "5.56", fees[0].Amount.StringFixed(2))

	// 0.25% of 2002 = 5.005 → 5.01 (exact half rounds up)
	cfg = &client.FeeConfig{ProcessingFeeRate: d("0.25")}
	fees, _ = ComputeDisbursementFees(d("2002"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "5.01", fees[0].Amount.StringFixed(2))
}

func TestComputeDisbursementFees_MandatoryFlatUpfrontFee(t *testing.T) {
	cfg := &client.FeeConfig{Fees: []client.ProductFee{
		{FeeName: "Application Fee", FeeType: "UPFRONT", CalculationType: "FLAT", Amount: dp("300"), IsMandatory: true},
	}}
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "Application Fee", fees[0].FeeName)
	assert.Equal(t, "UPFRONT", fees[0].FeeType)
	assert.Equal(t, "FLAT", fees[0].CalculationType)
	assert.Equal(t, "300.00", fees[0].Amount.StringFixed(2))
	assert.Equal(t, "300.00", total.StringFixed(2))
}

func TestComputeDisbursementFees_MandatoryPercentageDisbursementFee(t *testing.T) {
	cfg := &client.FeeConfig{Fees: []client.ProductFee{
		{FeeName: "Insurance Levy", FeeType: "DISBURSEMENT", CalculationType: "PERCENTAGE", Rate: dp("1.5"), IsMandatory: true},
	}}
	fees, total := ComputeDisbursementFees(d("20000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "DISBURSEMENT", fees[0].FeeType)
	assert.Equal(t, "300.00", fees[0].Amount.StringFixed(2))
	assert.Equal(t, "300.00", total.StringFixed(2))
}

func TestComputeDisbursementFees_SkipsNonMandatoryAndPeriodicFees(t *testing.T) {
	cfg := &client.FeeConfig{Fees: []client.ProductFee{
		{FeeName: "Optional Fee", FeeType: "UPFRONT", CalculationType: "FLAT", Amount: dp("100"), IsMandatory: false},
		{FeeName: "Monthly Service", FeeType: "MONTHLY", CalculationType: "FLAT", Amount: dp("50"), IsMandatory: true},
		{FeeName: "Annual Fee", FeeType: "ANNUAL", CalculationType: "FLAT", Amount: dp("500"), IsMandatory: true},
		{FeeName: "Exit Fee", FeeType: "EXIT", CalculationType: "PERCENTAGE", Rate: dp("2"), IsMandatory: true},
	}}
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	assert.Empty(t, fees)
	assert.True(t, total.IsZero())
}

func TestComputeDisbursementFees_SkipsNilOrZeroAmounts(t *testing.T) {
	cfg := &client.FeeConfig{Fees: []client.ProductFee{
		{FeeName: "Broken Flat", FeeType: "UPFRONT", CalculationType: "FLAT", Amount: nil, IsMandatory: true},
		{FeeName: "Broken Pct", FeeType: "DISBURSEMENT", CalculationType: "PERCENTAGE", Rate: nil, IsMandatory: true},
		{FeeName: "Zero Flat", FeeType: "UPFRONT", CalculationType: "FLAT", Amount: dp("0"), IsMandatory: true},
	}}
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	assert.Empty(t, fees)
	assert.True(t, total.IsZero())
}

func TestComputeDisbursementFees_MixedProcessingFlatAndPercentage(t *testing.T) {
	cfg := &client.FeeConfig{
		ProcessingFeeRate: d("2"),
		ProcessingFeeMin:  d("100"),
		ProcessingFeeMax:  dp("500"),
		Fees: []client.ProductFee{
			{FeeName: "Application Fee", FeeType: "UPFRONT", CalculationType: "FLAT", Amount: dp("250"), IsMandatory: true},
			{FeeName: "Insurance Levy", FeeType: "DISBURSEMENT", CalculationType: "PERCENTAGE", Rate: dp("0.75"), IsMandatory: true},
			{FeeName: "Optional Extra", FeeType: "UPFRONT", CalculationType: "FLAT", Amount: dp("999"), IsMandatory: false},
		},
	}
	// Processing: 2% of 30000 = 600 → clamped to max 500
	// Flat: 250; Percentage: 0.75% of 30000 = 225
	fees, total := ComputeDisbursementFees(d("30000"), cfg)
	require.Len(t, fees, 3)
	assert.Equal(t, "500.00", fees[0].Amount.StringFixed(2))
	assert.Equal(t, "250.00", fees[1].Amount.StringFixed(2))
	assert.Equal(t, "225.00", fees[2].Amount.StringFixed(2))
	assert.Equal(t, "975.00", total.StringFixed(2))
}

func TestComputeDisbursementFees_LowercaseTypesAccepted(t *testing.T) {
	cfg := &client.FeeConfig{Fees: []client.ProductFee{
		{FeeName: "App Fee", FeeType: "upfront", CalculationType: "flat", Amount: dp("100"), IsMandatory: true},
	}}
	fees, total := ComputeDisbursementFees(d("10000"), cfg)
	require.Len(t, fees, 1)
	assert.Equal(t, "UPFRONT", fees[0].FeeType)
	assert.Equal(t, "FLAT", fees[0].CalculationType)
	assert.Equal(t, "100.00", total.StringFixed(2))
}

func TestValidateFeeTotal(t *testing.T) {
	assert.NoError(t, ValidateFeeTotal(d("10000"), d("0")))
	assert.NoError(t, ValidateFeeTotal(d("10000"), d("9999.99")))
	assert.Error(t, ValidateFeeTotal(d("10000"), d("10000")), "fees equal to the disbursed amount must be rejected")
	assert.Error(t, ValidateFeeTotal(d("10000"), d("10000.01")), "fees above the disbursed amount must be rejected")
}
