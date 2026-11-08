package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/athena-lms/go-services/internal/invoice/model"
)

func TestHappyPathIsLegal(t *testing.T) {
	path := []string{
		model.StatusUploaded, model.StatusExtracted, model.StatusAwaitingBuyer, model.StatusBuyerConfirmed,
		model.StatusScored, model.StatusOffered, model.StatusAccepted, model.StatusFunded,
		model.StatusPartiallyPaid, model.StatusPaid,
	}
	for i := 1; i < len(path); i++ {
		assert.NoError(t, Check(path[i-1], path[i]), "%s -> %s", path[i-1], path[i])
	}
	assert.True(t, IsTerminal(model.StatusPaid))
	assert.True(t, IsTerminal(model.StatusDeclined))
	assert.True(t, IsTerminal(model.StatusCancelled))
}

func TestReferPathIsLegal(t *testing.T) {
	assert.True(t, CanTransition(model.StatusScored, model.StatusDecided))
	assert.True(t, CanTransition(model.StatusDecided, model.StatusOffered))
	assert.True(t, CanTransition(model.StatusDecided, model.StatusDeclined))
	assert.Equal(t, model.EvReviewApproved, EventFor(model.StatusDecided, model.StatusOffered))
	assert.Equal(t, model.EvReviewRejected, EventFor(model.StatusDecided, model.StatusDeclined))
	assert.Equal(t, model.EvOfferCreated, EventFor(model.StatusScored, model.StatusOffered))
}

func TestIllegalTransitions(t *testing.T) {
	cases := [][2]string{
		{model.StatusUploaded, model.StatusFunded},
		{model.StatusAwaitingBuyer, model.StatusScored}, // must confirm first
		{model.StatusOffered, model.StatusFunded},       // must accept first
		{model.StatusPaid, model.StatusFunded},
		{model.StatusDeclined, model.StatusUploaded},
		{model.StatusFunded, model.StatusCancelled}, // money moved: no cancel
	}
	for _, c := range cases {
		err := Check(c[0], c[1])
		assert.Error(t, err, "%s -> %s", c[0], c[1])
		var e *ErrIllegalTransition
		assert.ErrorAs(t, err, &e)
	}
}

func TestOverdueAndSettlementLoop(t *testing.T) {
	assert.True(t, CanTransition(model.StatusFunded, model.StatusOverdue))
	assert.True(t, CanTransition(model.StatusOverdue, model.StatusPartiallyPaid))
	assert.True(t, CanTransition(model.StatusOverdue, model.StatusPaid))
	assert.True(t, CanTransition(model.StatusPartiallyPaid, model.StatusPartiallyPaid))
	assert.Equal(t, model.EvAdvanceOverdue, EventFor(model.StatusFunded, model.StatusOverdue))
	assert.Equal(t, model.EvPaymentReceived, EventFor(model.StatusFunded, model.StatusPaid))
}

func TestAutoSteps(t *testing.T) {
	assert.Equal(t, "extract", StepFor(model.StatusUploaded))
	assert.Equal(t, "notify_buyer", StepFor(model.StatusExtracted))
	assert.Equal(t, "score", StepFor(model.StatusBuyerConfirmed))
	assert.Equal(t, "decide", StepFor(model.StatusScored))
	assert.Equal(t, "fund", StepFor(model.StatusAccepted))
	for _, s := range []string{model.StatusAwaitingBuyer, model.StatusDecided, model.StatusOffered, model.StatusFunded, model.StatusPaid} {
		assert.Empty(t, StepFor(s), s)
	}
	assert.Len(t, AutoStatuses(), 5)
}

func TestStatusForDecision(t *testing.T) {
	s, r := StatusForDecision("APPROVE")
	assert.Equal(t, model.StatusOffered, s)
	assert.Empty(t, r)
	s, r = StatusForDecision("REFER")
	assert.Equal(t, model.StatusDecided, s)
	assert.Equal(t, model.ReasonAwaitingReview, r)
	s, _ = StatusForDecision("DECLINE")
	assert.Equal(t, model.StatusDeclined, s)
}
