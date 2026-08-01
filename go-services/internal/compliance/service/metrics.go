package service

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/athena-lms/go-services/internal/common/metrics"
)

// Shadow-liveness calibration metrics (docs/ekyc/05 audit action 2, docs/
// nemo/08): the onboarding decision path observes every PAD score, face-match
// score and tier decision so threshold calibration has an aggregate view from
// day one of real traffic — the per-application substrate is the structured
// columns (migrations 7 and 8). Registered on the default registry, which
// cmd/compliance-service already serves at /metrics (H2 baseline); names
// carry the repo-wide nemo_ prefix.
var (
	// onboardingDecisions counts Submit outcomes by status and risk tier
	// (auto-approval rate, referral mix).
	onboardingDecisions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nemo_onboarding_decisions_total",
		Help: "Onboarding Submit decisions by status and risk tier.",
	}, []string{"status", "risk_tier"})

	// onboardingLivenessScore is the PAD score distribution, observed only
	// when a score actually arrived (mode shadow/enforce with score >= 0 —
	// shadow-error carries no score). Buckets cover [0,1] in 0.1 steps, the
	// resolution the LIVENESS_ENFORCE threshold calibration needs.
	onboardingLivenessScore = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nemo_onboarding_liveness_score",
		Help:    "Passive-PAD P(live) scores from onboarding verification, by mode and liveness provider.",
		Buckets: prometheus.LinearBuckets(0, 0.1, 11),
	}, []string{"mode", "provider"})

	// onboardingFaceMatchScore is the document-vs-selfie score distribution,
	// observed only when a face match ran (same rule as the
	// face_match_score column).
	onboardingFaceMatchScore = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "nemo_onboarding_face_match_score",
		Help:    "Document-vs-selfie face-match scores from onboarding verification.",
		Buckets: prometheus.LinearBuckets(0, 0.1, 11),
	})
)

func init() {
	metrics.MustRegister(onboardingDecisions, onboardingLivenessScore, onboardingFaceMatchScore)
}
