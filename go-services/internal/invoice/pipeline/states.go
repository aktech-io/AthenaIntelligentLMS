// Package pipeline holds the invoice state machine (SPEC §2 statuses, §3
// steps) as data: which transitions are legal, which ledger event each
// transition writes, and which statuses the in-process orchestrator advances
// automatically. The service layer executes steps; this package decides.
package pipeline

import (
	"fmt"

	"github.com/athena-lms/go-services/internal/invoice/model"
)

// ErrIllegalTransition is returned for a transition the machine forbids.
type ErrIllegalTransition struct{ From, To string }

func (e *ErrIllegalTransition) Error() string {
	return fmt.Sprintf("illegal invoice transition %s -> %s", e.From, e.To)
}

// transitions is the closed set of legal edges. Terminal: PAID, DECLINED, CANCELLED.
var transitions = map[string]map[string]bool{
	model.StatusUploaded: {
		model.StatusExtracted: true, model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusExtracted: {
		model.StatusAwaitingBuyer: true, model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusAwaitingBuyer: {
		model.StatusBuyerConfirmed: true, model.StatusBuyerDisputed: true,
		model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusBuyerDisputed: {
		model.StatusDeclined: true, model.StatusCancelled: true, model.StatusAwaitingBuyer: true,
	},
	model.StatusBuyerConfirmed: {
		model.StatusScored: true, model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusScored: {
		model.StatusDecided: true, model.StatusOffered: true, model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusDecided: { // REFER parked here until a MANAGER approves/rejects
		model.StatusOffered: true, model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusOffered: {
		model.StatusAccepted: true, model.StatusDeclined: true, model.StatusCancelled: true,
	},
	model.StatusAccepted: {
		model.StatusFunded: true, model.StatusCancelled: true, model.StatusDeclined: true,
	},
	model.StatusFunded: {
		model.StatusPartiallyPaid: true, model.StatusPaid: true, model.StatusOverdue: true,
	},
	model.StatusPartiallyPaid: {
		model.StatusPartiallyPaid: true, model.StatusPaid: true, model.StatusOverdue: true,
	},
	model.StatusOverdue: {
		model.StatusPartiallyPaid: true, model.StatusPaid: true, model.StatusOverdue: true,
	},
}

// CanTransition reports whether from -> to is legal.
func CanTransition(from, to string) bool {
	return transitions[from][to]
}

// Check returns an ErrIllegalTransition when from -> to is not legal.
func Check(from, to string) error {
	if !CanTransition(from, to) {
		return &ErrIllegalTransition{From: from, To: to}
	}
	return nil
}

// IsTerminal reports whether no further transition is possible.
func IsTerminal(status string) bool { return len(transitions[status]) == 0 }

// EventFor returns the ledger event type written for entering `to` from
// `from`. Transitions that need more context (DECIDED can be APPROVE/REFER)
// are resolved by the caller; this is the default mapping.
func EventFor(from, to string) string {
	switch to {
	case model.StatusUploaded:
		return model.EvInvoiceUploaded
	case model.StatusExtracted:
		return model.EvInvoiceExtracted
	case model.StatusAwaitingBuyer:
		return model.EvBuyerNotified
	case model.StatusBuyerConfirmed:
		return model.EvBuyerConfirmed
	case model.StatusBuyerDisputed:
		return model.EvBuyerDisputed
	case model.StatusScored:
		return model.EvInvoiceScored
	case model.StatusDecided:
		return model.EvInvoiceDecided
	case model.StatusOffered:
		if from == model.StatusDecided {
			return model.EvReviewApproved
		}
		return model.EvOfferCreated
	case model.StatusAccepted:
		return model.EvOfferAccepted
	case model.StatusFunded:
		return model.EvAdvanceFunded
	case model.StatusPartiallyPaid, model.StatusPaid:
		return model.EvPaymentReceived
	case model.StatusOverdue:
		return model.EvAdvanceOverdue
	case model.StatusDeclined:
		if from == model.StatusDecided {
			return model.EvReviewRejected
		}
		return model.EvInvoiceDeclined
	case model.StatusCancelled:
		return model.EvInvoiceCancelled
	}
	return "invoice.status_changed"
}

// Auto-advanced statuses: the orchestrator runs the next step as soon as an
// invoice lands here (and the retry loop picks up stragglers). Everything
// else waits for an external actor (buyer, seller, manager, payer, clock).
var autoSteps = map[string]string{
	model.StatusUploaded:       "extract",
	model.StatusExtracted:      "notify_buyer",
	model.StatusBuyerConfirmed: "score",
	model.StatusScored:         "decide",
	model.StatusAccepted:       "fund",
}

// StepFor returns the automatic step name for a status, or "" when the
// pipeline waits for an external actor.
func StepFor(status string) string { return autoSteps[status] }

// AutoStatuses lists the statuses the retry loop scans.
func AutoStatuses() []string {
	return []string{model.StatusUploaded, model.StatusExtracted, model.StatusBuyerConfirmed, model.StatusScored, model.StatusAccepted}
}

// Outcome → next status after the decide step.
func StatusForDecision(outcome string) (status, reason string) {
	switch outcome {
	case "APPROVE":
		return model.StatusOffered, ""
	case "REFER", "FLAG":
		return model.StatusDecided, model.ReasonAwaitingReview
	default:
		return model.StatusDeclined, "POLICY_DECLINE"
	}
}
