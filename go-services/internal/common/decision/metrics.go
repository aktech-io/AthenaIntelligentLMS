package decision

import "github.com/prometheus/client_golang/prometheus"

// Drift feeds v1 (design §4): decision.recorded is the durable feed; these
// Prometheus series make score-distribution and approval-rate drift alert
// through the standard /metrics plumbing (H2) without waiting for the E7
// statistical jobs.
var (
	outcomesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "decision_outcomes_total",
		Help: "Decisions evaluated, by decision type, outcome and variant.",
	}, []string{"type", "outcome", "variant"})

	latencyMS = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "decision_latency_ms",
		Help:    "End-to-end Evaluate latency in milliseconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100},
	}, []string{"type"})

	modelScore = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "decision_model_score",
		Help:    "Model scores observed on decisions, per model name and version.",
		Buckets: prometheus.LinearBuckets(300, 60, 11), // credit-score range 300-900
	}, []string{"model", "version"})

	evalErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "decision_evaluation_errors_total",
		Help: "Evaluations that could not run, by decision type and stage (resolve|evaluate|challenger).",
	}, []string{"type", "stage"})
)

func init() {
	prometheus.MustRegister(outcomesTotal, latencyMS, modelScore, evalErrors)
}
