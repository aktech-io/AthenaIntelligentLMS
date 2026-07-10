package event

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/origination/model"
)

func testApp() *model.LoanApplication {
	approved := decimal.RequireFromString("10000")
	rate := decimal.RequireFromString("14.5")
	acct := "11111111-2222-3333-4444-555555555555"
	return &model.LoanApplication{
		ID:                  uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		TenantID:            "tenant1",
		CustomerID:          "cust-42",
		ProductID:           uuid.MustParse("99999999-8888-7777-6666-555555555555"),
		ApprovedAmount:      &approved,
		RequestedAmount:     decimal.RequireFromString("12000"),
		Currency:            "KES",
		TenorMonths:         12,
		InterestRate:        &rate,
		Status:              model.StatusDisbursed,
		DisbursementAccount: &acct,
		DepositAmount:       decimal.Zero,
	}
}

func TestBuildFeeCharged_ExactPayloadContract(t *testing.T) {
	p := NewPublisher(nil, zap.NewNop())
	app := testApp()
	fee := model.DisbursementFee{
		ApplicationID:   app.ID,
		TenantID:        app.TenantID,
		FeeName:         "Processing Fee",
		FeeType:         "PROCESSING",
		CalculationType: "PERCENTAGE",
		Amount:          decimal.RequireFromString("150"),
		Currency:        "KES",
		Reference:       "FEE-" + app.ID.String() + "-1",
	}

	evt, err := p.BuildFeeCharged(app, fee)
	require.NoError(t, err)
	assert.Equal(t, commonEvent.LoanFeeCharged, evt.Type)
	assert.Equal(t, "loan-origination-service", evt.Source)
	assert.Equal(t, "tenant1", evt.TenantID)

	// The payload contract consumed by accounting — keys and types are fixed.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, map[string]any{
		"applicationId": app.ID.String(),
		"customerId":    "cust-42",
		"feeType":       "PROCESSING",
		"feeName":       "Processing Fee",
		"amount":        "150.00", // decimal string, 2dp
		"currency":      "KES",
		"reference":     "FEE-" + app.ID.String() + "-1",
		"tenantId":      "tenant1",
	}, payload)
}

func TestBuildDisbursed_AdditiveNetAndFeeFields(t *testing.T) {
	p := NewPublisher(nil, zap.NewNop())
	app := testApp()
	st := "FLAT"
	rf := "MONTHLY"

	evt, err := p.BuildDisbursed(app, &st, &rf,
		decimal.RequireFromString("9700"), decimal.RequireFromString("300"))
	require.NoError(t, err)
	assert.Equal(t, commonEvent.LoanDisbursed, evt.Type)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	// New additive fields (decimal strings).
	assert.Equal(t, "9700.00", payload["netDisbursedAmount"])
	assert.Equal(t, "300.00", payload["totalFeesCharged"])

	// Pre-existing contract fields the management consumer parses are unchanged:
	// amount stays the GROSS principal.
	assert.Equal(t, app.ID.String(), payload["applicationId"])
	assert.Equal(t, "tenant1", payload["tenantId"])
	assert.Equal(t, "cust-42", payload["customerId"])
	assert.Equal(t, "DISBURSED", payload["status"])
	assert.Equal(t, "FLAT", payload["scheduleType"])
	assert.Equal(t, "MONTHLY", payload["repaymentFrequency"])
	gross, _ := json.Marshal(payload["amount"])
	assert.Contains(t, []string{"10000", "\"10000\"", "10000.0"}, string(gross))
}

func TestBuildDisbursed_ZeroFees(t *testing.T) {
	p := NewPublisher(nil, zap.NewNop())
	app := testApp()

	evt, err := p.BuildDisbursed(app, nil, nil,
		decimal.RequireFromString("10000"), decimal.Zero)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, "10000.00", payload["netDisbursedAmount"])
	assert.Equal(t, "0.00", payload["totalFeesCharged"])
}
