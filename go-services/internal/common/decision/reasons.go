package decision

// Reason codes are stable, market-neutral, and APPEND-ONLY: meanings never
// mutate (design §3, same discipline as event versioning in EDA_HARDENING
// §1.2). Machine codes stay stable while customer-facing wording localises via
// market-pack content (increment 2). Policies may only reference registered
// codes; the policy loader validates this at parse time so a typo'd code is a
// build/startup defect, not a silent audit gap.
const (
	ReasonScoreBandLow          = "SCORE_BAND_LOW"
	ReasonScoreBandUnconfigured = "SCORE_BAND_UNCONFIGURED"
	ReasonKYCIncomplete         = "KYC_INCOMPLETE"
	ReasonScoreStale            = "SCORE_STALE"
	ReasonExposureLimit         = "EXPOSURE_LIMIT"
	ReasonWatchlistHit          = "WATCHLIST_HIT"
	ReasonModelUnavailable      = "MODEL_UNAVAILABLE"
	ReasonBureauHistory         = "BUREAU_HISTORY"
	ReasonFacilityExists        = "FACILITY_EXISTS"
	// ReasonPolicyNoMatch is the fail-closed default: no policy rule produced
	// an outcome, so the evaluator declines rather than inventing one.
	ReasonPolicyNoMatch = "POLICY_NO_MATCH"
)

// reasonRegistry maps every registered code to its stable, regulator-facing
// meaning. Append-only.
var reasonRegistry = map[string]string{
	ReasonScoreBandLow:          "Credit score band below the approval threshold",
	ReasonScoreBandUnconfigured: "No policy configuration exists for the credit score band",
	ReasonKYCIncomplete:         "Customer identity verification (KYC) not completed",
	ReasonScoreStale:            "Credit score is older than the policy allows",
	ReasonExposureLimit:         "Requested amount exceeds the exposure limit",
	ReasonWatchlistHit:          "Subject matched a watchlist entry",
	ReasonModelUnavailable:      "A required scoring model was unavailable or its output could not be trusted",
	ReasonBureauHistory:         "Credit bureau history contributed negatively",
	ReasonFacilityExists:        "An active facility already exists for this subject",
	ReasonPolicyNoMatch:         "No policy rule matched the application",
}

// KnownReason reports whether code is registered.
func KnownReason(code string) bool {
	_, ok := reasonRegistry[code]
	return ok
}

// ReasonDescription returns the registered meaning of a code ("" if unknown).
func ReasonDescription(code string) string { return reasonRegistry[code] }
