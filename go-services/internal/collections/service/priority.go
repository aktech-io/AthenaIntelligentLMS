package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/collections/model"
	scoringmodel "github.com/athena-lms/go-services/internal/scoring/model"
)

// PriorityScorer is the NemoScore collections-priority client surface
// (contract 1.5.0). Implemented by scoring/client.AthenaScoreClient.
type PriorityScorer interface {
	CollectionsPriority(ctx context.Context, customerID int64, req scoringmodel.CollectionsPriorityRequest) (*scoringmodel.CollectionsPriorityResponse, error)
}

// priorityScoreTimeout bounds the outbound scoring call inside event
// handling — a slow scoring API must not stall the DPD consumer.
const priorityScoreTimeout = 3 * time.Second

// SetPriorityScorer wires the NemoScore client. When unset (or when any call
// fails) the service keeps its DPD-threshold rules — fail-closed to the
// pre-scoring behaviour, never a fabricated band.
func (s *CollectionsService) SetPriorityScorer(scorer PriorityScorer) {
	s.priorityScorer = scorer
}

func priorityOrdinal(p model.CasePriority) int {
	switch p {
	case model.CasePriorityLow:
		return 0
	case model.CasePriorityNormal:
		return 1
	case model.CasePriorityHigh:
		return 2
	case model.CasePriorityCritical:
		return 3
	default:
		return 1 // unknown → NORMAL
	}
}

func maxPriority(a, b model.CasePriority) model.CasePriority {
	if priorityOrdinal(b) > priorityOrdinal(a) {
		return b
	}
	return a
}

// dpdRulePriority is the pre-scoring DPD-threshold rule, kept as both the
// fallback and the hard floor: a score may escalate a case earlier than DPD
// alone would, but can never soften what the DPD rules mandate.
func dpdRulePriority(dpd int, current model.CasePriority) model.CasePriority {
	base := current
	if base == "" {
		base = model.CasePriorityNormal
	}
	switch {
	case dpd > 90:
		return maxPriority(base, model.CasePriorityCritical)
	case dpd > 60:
		return maxPriority(base, model.CasePriorityHigh)
	default:
		return base
	}
}

// bandToPriority validates a NemoScore priority_band into the CasePriority
// enum; unknown strings report false and the caller keeps the DPD rule.
func bandToPriority(band string) (model.CasePriority, bool) {
	switch model.CasePriority(band) {
	case model.CasePriorityLow, model.CasePriorityNormal,
		model.CasePriorityHigh, model.CasePriorityCritical:
		return model.CasePriority(band), true
	default:
		return "", false
	}
}

// scoredPriority asks NemoScore for a collections-priority band for the case.
// Returns ok=false whenever no usable band is available (no scorer wired, no
// customer id on the case, API error/404/409, unknown band) — the caller then
// keeps the DPD-rule priority.
func (s *CollectionsService) scoredPriority(ctx context.Context, kase *model.CollectionCase) (model.CasePriority, bool) {
	if s.priorityScorer == nil || kase.CustomerID == nil || *kase.CustomerID == "" {
		return "", false
	}

	broken, fulfilled := 0, 0
	if kase.ID != uuid.Nil {
		if ptps, err := s.ptpRepo.FindByCaseIDOrderByCreatedAtDesc(ctx, kase.ID); err == nil {
			for _, p := range ptps {
				switch p.Status {
				case model.PtpStatusBroken:
					broken++
				case model.PtpStatusFulfilled:
					fulfilled++
				}
			}
		}
	}

	cctx, cancel := context.WithTimeout(ctx, priorityScoreTimeout)
	defer cancel()
	resp, err := s.priorityScorer.CollectionsPriority(cctx,
		scoringmodel.FlexibleCustomerID(*kase.CustomerID),
		scoringmodel.CollectionsPriorityRequest{
			Dpd:               kase.CurrentDPD,
			OutstandingAmount: kase.OutstandingAmount.InexactFloat64(),
			BrokenPtpCount:    broken,
			FulfilledPtpCount: fulfilled,
			ProductType:       kase.ProductType,
		})
	if err != nil {
		s.logger.Debug("Collections-priority score unavailable, keeping DPD rules",
			zap.String("loanId", kase.LoanID.String()), zap.Error(err))
		return "", false
	}
	band, ok := bandToPriority(resp.PriorityBand)
	if !ok {
		s.logger.Warn("Collections-priority returned unknown band",
			zap.String("band", resp.PriorityBand))
		return "", false
	}
	s.logger.Info("Collections priority scored",
		zap.String("loanId", kase.LoanID.String()),
		zap.Float64("priorityScore", resp.PriorityScore),
		zap.String("priorityBand", resp.PriorityBand),
		zap.String("recommendedAction", resp.RecommendedAction),
		zap.String("abilityToPay", resp.AbilityToPay))
	return band, true
}

// resolvePriority combines the DPD-rule floor with the scored band (when
// available): final = max(DPD rule, scored band).
func (s *CollectionsService) resolvePriority(ctx context.Context, kase *model.CollectionCase, current model.CasePriority) model.CasePriority {
	rule := dpdRulePriority(kase.CurrentDPD, current)
	if scored, ok := s.scoredPriority(ctx, kase); ok {
		return maxPriority(rule, scored)
	}
	return rule
}
