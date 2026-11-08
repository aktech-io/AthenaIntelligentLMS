package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/policy"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func sampleInvoice(now time.Time, total string, days int) *model.Invoice {
	t := d(total)
	due := now.AddDate(0, 0, days)
	return &model.Invoice{ID: uuid.New(), Total: &t, DueDate: &due, Currency: "KES",
		OCR: model.OCRMeta{Confidence: map[string]float64{"total": 0.95, "dueDate": 0.9, "invoiceNumber": 0.93}}}
}

func TestComputeOfferBandA(t *testing.T) {
	now := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	inv := sampleInvoice(now, "100000", 45)
	o := ComputeOffer(inv, policy.Terms{AdvanceRate: d("0.90"), FeePerDay: d("0.0005")}, DefaultConfig(), now)
	assert.Equal(t, 45, o.TenorDays)
	assert.True(t, o.AdvanceAmount.Equal(d("90000")), o.AdvanceAmount.String())
	assert.True(t, o.FeeAmount.Equal(d("2025")), o.FeeAmount.String())        // 90000 × 0.0005 × 45
	assert.True(t, o.ProcessingFee.Equal(d("900")), o.ProcessingFee.String()) // 1% of advance (> 500 min)
	assert.True(t, o.NetDisbursement.Equal(d("87075")), o.NetDisbursement.String())
	assert.True(t, o.ReserveAmount.Equal(d("10000")), o.ReserveAmount.String())
	assert.Equal(t, model.OfferOffered, o.Status)
	assert.Equal(t, now.Add(7*24*time.Hour), o.ExpiresAt)
}

func TestComputeOfferProcessingFeeMinAndRequestedAmount(t *testing.T) {
	now := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	inv := sampleInvoice(now, "40000", 30)
	req := d("20000")
	inv.RequestedAmount = &req
	o := ComputeOffer(inv, policy.Terms{AdvanceRate: d("0.80"), FeePerDay: d("0.0007")}, DefaultConfig(), now)
	assert.True(t, o.AdvanceAmount.Equal(d("16000"))) // 80% of the requested 20k, not the 40k total
	assert.True(t, o.ProcessingFee.Equal(d("500")))   // 1% = 160 → floor 500
	assert.True(t, o.ReserveAmount.Equal(d("24000"))) // total − advance
	// requested above total is capped at total
	big := d("1000000")
	inv.RequestedAmount = &big
	o = ComputeOffer(inv, policy.Terms{AdvanceRate: d("0.80"), FeePerDay: d("0.0007")}, DefaultConfig(), now)
	assert.True(t, o.AdvanceAmount.Equal(d("32000")))
	// max_amount cap from the policy
	mx := d("10000")
	o = ComputeOffer(inv, policy.Terms{AdvanceRate: d("0.80"), FeePerDay: d("0.0007"), MaxAmount: &mx}, DefaultConfig(), now)
	assert.True(t, o.AdvanceAmount.Equal(mx))
}

func TestApplyPaymentWaterfall(t *testing.T) {
	adv := &model.Advance{Principal: d("90000"), FeesAccrued: d("2925"), ReserveAmount: d("10000")}
	fees, prin, rel, covered := ApplyPayment(adv, d("50000"))
	assert.True(t, fees.Equal(d("2925")))
	assert.True(t, prin.Equal(d("47075")))
	assert.True(t, rel.IsZero())
	assert.False(t, covered)

	adv.FeesRepaid, adv.Repaid = fees, prin
	fees, prin, rel, covered = ApplyPayment(adv, d("50000")) // buyer pays the full 100k invoice in two halves
	assert.True(t, fees.IsZero())
	assert.True(t, prin.Equal(d("42925")))
	assert.True(t, rel.Equal(d("7075"))) // surplus → reserve release to seller
	assert.True(t, covered)
	adv.Repaid = adv.Repaid.Add(prin)
	assert.True(t, adv.Outstanding().IsZero())
}

func TestLocalFraudFlags(t *testing.T) {
	now := time.Now()
	cfg := DefaultConfig()
	// Clean first invoice from a brand-new, unrated buyer: no flags (demo path).
	in := FraudInputs{SellerEmail: "ar@seller.co.ke", BuyerEmail: "ap@buyer.co.ke", BuyerCreatedAt: now.Add(-time.Hour), BuyerTier: "C", Amount: d("100000")}
	assert.Empty(t, LocalFraudFlags(in, cfg, now))

	in.BuyerEmail = "ap@seller.co.ke"
	assert.Contains(t, LocalFraudFlags(in, cfg, now), "SAME_PARTY_DOMAIN")
	in.BuyerEmail, in.SellerEmail = "a@gmail.com", "b@gmail.com"
	assert.Empty(t, LocalFraudFlags(in, cfg, now), "public mail domains are not evidence of same party")

	in.BuyerTier = "D"
	assert.Contains(t, LocalFraudFlags(in, cfg, now), "NEW_BUYER")
	in.BuyerCreatedAt = now.AddDate(-1, 0, 0)
	assert.Empty(t, LocalFraudFlags(in, cfg, now))

	in.SellerCount6m, in.SellerAvg6m, in.Amount = 4, d("20000"), d("70000")
	assert.Contains(t, LocalFraudFlags(in, cfg, now), "AMOUNT_SPIKE")
	in.SellerCount6m = 2
	assert.Empty(t, LocalFraudFlags(in, cfg, now), "needs ≥3 prior invoices")

	in.RemoteAvailable, in.RemoteRiskScore, in.RemoteRules = true, 0.6, []string{"VELOCITY"}
	flags := LocalFraudFlags(in, cfg, now)
	assert.Contains(t, flags, "FRAUD_SERVICE_RISK")
	assert.Contains(t, flags, "RULE_VELOCITY")
}

func TestInvoiceScoreAndDaysToDue(t *testing.T) {
	now := time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC)
	inv := sampleInvoice(now, "100000", 45)
	assert.Equal(t, 45, DaysToDue(inv, now))
	assert.Equal(t, 100, InvoiceScore(inv, 45, decimal.Zero, 0))
	assert.Equal(t, 90, InvoiceScore(inv, 10, decimal.Zero, 0))
	assert.Equal(t, 85, InvoiceScore(inv, 45, d("30000"), 5)) // 100k vs 30k avg → >2× → −15
	inv.NeedsConfirm = []string{"total", "dueDate"}
	inv.OCR.Confidence = map[string]float64{"total": 0.5, "dueDate": 0.5}
	assert.Equal(t, 70, InvoiceScore(inv, 45, decimal.Zero, 0)) // −20 low confidence, −10 two unconfirmed fields
	inv.OCR.Confidence = nil
	assert.Equal(t, 70, InvoiceScore(inv, 45, decimal.Zero, 0))
	inv.DueDate = nil
	assert.Equal(t, 0, DaysToDue(inv, now))
}
