package service

import (
	"testing"

	"github.com/athena-lms/go-services/internal/collections/model"
	"github.com/stretchr/testify/assert"
)

func TestMapStage(t *testing.T) {
	tests := []struct {
		input    string
		expected model.CollectionStage
	}{
		{"WATCH", model.CollectionStageWatch},
		{"watch", model.CollectionStageWatch},
		{"SUBSTANDARD", model.CollectionStageSubstandard},
		{"substandard", model.CollectionStageSubstandard},
		{"DOUBTFUL", model.CollectionStageDoubtful},
		{"doubtful", model.CollectionStageDoubtful},
		{"LOSS", model.CollectionStageLoss},
		{"loss", model.CollectionStageLoss},
		{"PERFORMING", model.CollectionStageWatch},
		{"", model.CollectionStageWatch},
		{"UNKNOWN", model.CollectionStageWatch},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapStage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWorseStage(t *testing.T) {
	tests := []struct {
		name     string
		current  model.CollectionStage
		next     model.CollectionStage
		expected bool
	}{
		{"WATCH to SUBSTANDARD", model.CollectionStageWatch, model.CollectionStageSubstandard, true},
		{"WATCH to DOUBTFUL", model.CollectionStageWatch, model.CollectionStageDoubtful, true},
		{"WATCH to LOSS", model.CollectionStageWatch, model.CollectionStageLoss, true},
		{"SUBSTANDARD to DOUBTFUL", model.CollectionStageSubstandard, model.CollectionStageDoubtful, true},
		{"SUBSTANDARD to LOSS", model.CollectionStageSubstandard, model.CollectionStageLoss, true},
		{"DOUBTFUL to LOSS", model.CollectionStageDoubtful, model.CollectionStageLoss, true},
		{"same stage WATCH", model.CollectionStageWatch, model.CollectionStageWatch, false},
		{"same stage LOSS", model.CollectionStageLoss, model.CollectionStageLoss, false},
		{"LOSS to WATCH (improvement)", model.CollectionStageLoss, model.CollectionStageWatch, false},
		{"DOUBTFUL to SUBSTANDARD (improvement)", model.CollectionStageDoubtful, model.CollectionStageSubstandard, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorseStage(tt.current, tt.next)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCaseNumber(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		prefix   string
	}{
		{"normal tenant", "acmebank", "COL-ACM-"},
		{"short tenant", "ab", "COL-AB-"},
		{"empty tenant", "", "COL-GEN-"},
		{"exactly 3 chars", "xyz", "COL-XYZ-"},
		{"long tenant", "longtenantname", "COL-LON-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateCaseNumber(tt.tenantID)
			assert.Contains(t, result, tt.prefix)
			// Should be unique (contains timestamp)
			result2 := generateCaseNumber(tt.tenantID)
			// Timestamps may match if called too fast, but format should be correct
			assert.Contains(t, result2, tt.prefix)
		})
	}
}

func TestStageOrdinal(t *testing.T) {
	assert.Equal(t, 0, model.StageOrdinal(model.CollectionStageWatch))
	assert.Equal(t, 1, model.StageOrdinal(model.CollectionStageSubstandard))
	assert.Equal(t, 2, model.StageOrdinal(model.CollectionStageDoubtful))
	assert.Equal(t, 3, model.StageOrdinal(model.CollectionStageLoss))
	assert.Equal(t, 0, model.StageOrdinal(model.CollectionStage("UNKNOWN")))
}

func TestCaseStatusConstants(t *testing.T) {
	assert.Equal(t, model.CaseStatus("OPEN"), model.CaseStatusOpen)
	assert.Equal(t, model.CaseStatus("IN_PROGRESS"), model.CaseStatusInProgress)
	assert.Equal(t, model.CaseStatus("PENDING_LEGAL"), model.CaseStatusPendingLegal)
	assert.Equal(t, model.CaseStatus("WRITTEN_OFF"), model.CaseStatusWrittenOff)
	assert.Equal(t, model.CaseStatus("CLOSED"), model.CaseStatusClosed)
}

func TestCasePriorityConstants(t *testing.T) {
	assert.Equal(t, model.CasePriority("LOW"), model.CasePriorityLow)
	assert.Equal(t, model.CasePriority("NORMAL"), model.CasePriorityNormal)
	assert.Equal(t, model.CasePriority("HIGH"), model.CasePriorityHigh)
	assert.Equal(t, model.CasePriority("CRITICAL"), model.CasePriorityCritical)
}

func TestActionTypeConstants(t *testing.T) {
	assert.Equal(t, model.ActionType("PHONE_CALL"), model.ActionTypePhoneCall)
	assert.Equal(t, model.ActionType("SMS"), model.ActionTypeSMS)
	assert.Equal(t, model.ActionType("EMAIL"), model.ActionTypeEmail)
	assert.Equal(t, model.ActionType("FIELD_VISIT"), model.ActionTypeFieldVisit)
	assert.Equal(t, model.ActionType("LEGAL_NOTICE"), model.ActionTypeLegalNotice)
	assert.Equal(t, model.ActionType("RESTRUCTURE_OFFER"), model.ActionTypeRestructureOffer)
	assert.Equal(t, model.ActionType("WRITE_OFF"), model.ActionTypeWriteOff)
	assert.Equal(t, model.ActionType("OTHER"), model.ActionTypeOther)
}

func TestActionOutcomeConstants(t *testing.T) {
	assert.Equal(t, model.ActionOutcome("CONTACTED"), model.ActionOutcomeContacted)
	assert.Equal(t, model.ActionOutcome("NO_ANSWER"), model.ActionOutcomeNoAnswer)
	assert.Equal(t, model.ActionOutcome("PROMISE_RECEIVED"), model.ActionOutcomePromiseReceived)
	assert.Equal(t, model.ActionOutcome("REFUSED_TO_PAY"), model.ActionOutcomeRefusedToPay)
	assert.Equal(t, model.ActionOutcome("PAYMENT_RECEIVED"), model.ActionOutcomePaymentReceived)
	assert.Equal(t, model.ActionOutcome("ESCALATED"), model.ActionOutcomeEscalated)
	assert.Equal(t, model.ActionOutcome("OTHER"), model.ActionOutcomeOther)
}

func TestPtpStatusConstants(t *testing.T) {
	assert.Equal(t, model.PtpStatus("PENDING"), model.PtpStatusPending)
	assert.Equal(t, model.PtpStatus("FULFILLED"), model.PtpStatusFulfilled)
	assert.Equal(t, model.PtpStatus("BROKEN"), model.PtpStatusBroken)
	assert.Equal(t, model.PtpStatus("CANCELLED"), model.PtpStatusCancelled)
}

func TestToCaseResponse(t *testing.T) {
	customerID := "CUST-123"
	assignedTo := "agent@example.com"
	notes := "Test notes"
	c := &model.CollectionCase{
		TenantID:     "test-tenant",
		CustomerID:   &customerID,
		CaseNumber:   "COL-TES-123",
		Status:       model.CaseStatusOpen,
		Priority:     model.CasePriorityNormal,
		CurrentStage: model.CollectionStageWatch,
		AssignedTo:   &assignedTo,
		Notes:        &notes,
	}

	resp := model.ToCaseResponse(c)
	assert.Equal(t, c.TenantID, resp.TenantID)
	assert.Equal(t, c.CaseNumber, resp.CaseNumber)
	assert.Equal(t, c.Status, resp.Status)
	assert.Equal(t, c.Priority, resp.Priority)
	assert.Equal(t, c.CurrentStage, resp.CurrentStage)
	assert.Equal(t, &customerID, resp.CustomerID)
	assert.Equal(t, &assignedTo, resp.AssignedTo)
	assert.Equal(t, &notes, resp.Notes)
}

func TestToActionResponse(t *testing.T) {
	outcome := model.ActionOutcomeContacted
	notes := "Called customer"
	a := &model.CollectionAction{
		ActionType: model.ActionTypePhoneCall,
		Outcome:    &outcome,
		Notes:      &notes,
	}

	resp := model.ToActionResponse(a)
	assert.Equal(t, a.ActionType, resp.ActionType)
	assert.Equal(t, a.Outcome, resp.Outcome)
	assert.Equal(t, a.Notes, resp.Notes)
}

func TestToPtpResponse(t *testing.T) {
	notes := "Will pay on Friday"
	createdBy := "agent@example.com"
	p := &model.PromiseToPay{
		Status:    model.PtpStatusPending,
		Notes:     &notes,
		CreatedBy: &createdBy,
	}

	resp := model.ToPtpResponse(p)
	assert.Equal(t, p.Status, resp.Status)
	assert.Equal(t, p.Notes, resp.Notes)
	assert.Equal(t, p.CreatedBy, resp.CreatedBy)
}

// -----------------------------------------------------------------------
// Collections-priority (NemoScore contract 1.5.0)
// -----------------------------------------------------------------------

func TestDpdRulePriority(t *testing.T) {
	tests := []struct {
		name     string
		dpd      int
		current  model.CasePriority
		expected model.CasePriority
	}{
		{"under 60 keeps current", 45, model.CasePriorityNormal, model.CasePriorityNormal},
		{"under 60 keeps manual escalation", 45, model.CasePriorityCritical, model.CasePriorityCritical},
		{"over 60 escalates to HIGH", 61, model.CasePriorityNormal, model.CasePriorityHigh},
		{"over 90 escalates to CRITICAL", 91, model.CasePriorityNormal, model.CasePriorityCritical},
		{"over 60 never downgrades CRITICAL", 61, model.CasePriorityCritical, model.CasePriorityCritical},
		{"empty current defaults to NORMAL", 10, "", model.CasePriorityNormal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, dpdRulePriority(tt.dpd, tt.current))
		})
	}
}

func TestMaxPriority(t *testing.T) {
	assert.Equal(t, model.CasePriorityCritical, maxPriority(model.CasePriorityHigh, model.CasePriorityCritical))
	assert.Equal(t, model.CasePriorityCritical, maxPriority(model.CasePriorityCritical, model.CasePriorityLow))
	assert.Equal(t, model.CasePriorityNormal, maxPriority(model.CasePriorityLow, model.CasePriorityNormal))
	assert.Equal(t, model.CasePriorityHigh, maxPriority(model.CasePriorityHigh, model.CasePriorityHigh))
}

func TestBandToPriority(t *testing.T) {
	for _, band := range []string{"LOW", "NORMAL", "HIGH", "CRITICAL"} {
		p, ok := bandToPriority(band)
		assert.True(t, ok, band)
		assert.Equal(t, model.CasePriority(band), p)
	}
	for _, bad := range []string{"", "critical", "URGENT", "P1"} {
		_, ok := bandToPriority(bad)
		assert.False(t, ok, bad)
	}
}
