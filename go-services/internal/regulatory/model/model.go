// Package model defines the per-tenant regulatory profile — the foundation for
// the Kenya/CBK regulatory reporting epic. The profile is bureau-agnostic and
// config-driven: it POINTS AT which provisioning rule-set and CRB bureau apply
// for a tenant, but holds no rates itself (those land with H-4). Downstream
// features (CRB feed, CBK provisioning overlay, prudential returns) read this
// profile to decide what to produce.
package model

import "time"

// ─── Enums ──────────────────────────────────────────────────────────────────

// LicenseType is the regulatory license a tenant operates under. It drives the
// report set and whether prudential (bank/MFB) returns apply.
type LicenseType string

const (
	// LicenseDCP is a standalone Digital Credit Provider (CBK DCP Regs 2022).
	LicenseDCP LicenseType = "DCP"
	// LicenseMFB is a Microfinance Bank (CBK prudential returns apply).
	LicenseMFB LicenseType = "MFB"
	// LicenseBank is a commercial bank (full CBK prudential returns apply).
	LicenseBank LicenseType = "BANK"
	// LicenseUnregulated is a non-licensed lender (IFRS statements only).
	LicenseUnregulated LicenseType = "UNREGULATED"
)

// ValidLicenseType reports whether s is a known LicenseType.
func ValidLicenseType(s string) bool {
	switch LicenseType(s) {
	case LicenseDCP, LicenseMFB, LicenseBank, LicenseUnregulated:
		return true
	}
	return false
}

// CrbBureau is a licensed Credit Reference Bureau a tenant submits its borrower
// feed to. Nullable on the profile — bureau-agnostic and unset by default.
type CrbBureau string

const (
	BureauMetropol   CrbBureau = "METROPOL"
	BureauTransUnion CrbBureau = "TRANSUNION"
	BureauCreditInfo CrbBureau = "CREDITINFO"
)

// ValidCrbBureau reports whether s is a known CrbBureau.
func ValidCrbBureau(s string) bool {
	switch CrbBureau(s) {
	case BureauMetropol, BureauTransUnion, BureauCreditInfo:
		return true
	}
	return false
}

// SubmissionFrequency is the cadence of a regulatory submission (e.g. CRB feed).
type SubmissionFrequency string

const (
	FrequencyDaily     SubmissionFrequency = "DAILY"
	FrequencyWeekly    SubmissionFrequency = "WEEKLY"
	FrequencyMonthly   SubmissionFrequency = "MONTHLY"
	FrequencyQuarterly SubmissionFrequency = "QUARTERLY"
)

// ValidSubmissionFrequency reports whether s is a known SubmissionFrequency.
func ValidSubmissionFrequency(s string) bool {
	switch SubmissionFrequency(s) {
	case FrequencyDaily, FrequencyWeekly, FrequencyMonthly, FrequencyQuarterly:
		return true
	}
	return false
}

// ReportCode identifies a single regulatory report a tenant may be obliged to
// produce. The enabled set per tenant lives on the profile's report_set.
type ReportCode string

const (
	// IFRS financial statements (all licenses).
	ReportIFRSTrialBalance ReportCode = "IFRS_TRIAL_BALANCE"
	ReportIFRSIncomeStmt   ReportCode = "IFRS_INCOME_STATEMENT"
	ReportIFRSBalanceSheet ReportCode = "IFRS_BALANCE_SHEET"
	ReportIFRSCashFlow     ReportCode = "IFRS_CASH_FLOW"

	// CBK Digital Credit Provider returns (DCP Regs 2022).
	ReportDCPLoanBookReturn ReportCode = "DCP_LOAN_BOOK_RETURN"
	ReportDCPAPRDisclosure  ReportCode = "DCP_APR_DISCLOSURE"
	ReportDCPComplaints     ReportCode = "DCP_COMPLAINTS"

	// CRB borrower feed (mandatory for licensed lenders).
	ReportCRBFeed ReportCode = "CRB_FEED"

	// AML/CFT to the Financial Reporting Centre.
	ReportAMLSTR ReportCode = "AML_STR"
	ReportAMLCTR ReportCode = "AML_CTR"

	// CBK prudential returns (bank/MFB-licensed tenants only).
	ReportCBKClassification  ReportCode = "CBK_PRUDENTIAL_CLASSIFICATION"
	ReportCBKNPLRatio        ReportCode = "CBK_NPL_RATIO"
	ReportCBKCapitalAdequacy ReportCode = "CBK_CAPITAL_ADEQUACY"
	ReportCBKLiquidity       ReportCode = "CBK_LIQUIDITY"
	ReportCBKLargeExposure   ReportCode = "CBK_LARGE_EXPOSURE"
)

// validReportCodes is the universe of recognised report codes.
var validReportCodes = map[ReportCode]bool{
	ReportIFRSTrialBalance: true, ReportIFRSIncomeStmt: true,
	ReportIFRSBalanceSheet: true, ReportIFRSCashFlow: true,
	ReportDCPLoanBookReturn: true, ReportDCPAPRDisclosure: true, ReportDCPComplaints: true,
	ReportCRBFeed: true,
	ReportAMLSTR:  true, ReportAMLCTR: true,
	ReportCBKClassification: true, ReportCBKNPLRatio: true,
	ReportCBKCapitalAdequacy: true, ReportCBKLiquidity: true, ReportCBKLargeExposure: true,
}

// ValidReportCode reports whether s is a recognised ReportCode.
func ValidReportCode(s string) bool {
	return validReportCodes[ReportCode(s)]
}

// Provisioning rule-set pointer keys. These name WHICH rule table applies; the
// rates themselves are owned by H-4 and are not stored here.
const (
	// ProvisioningCBKPG04 = higher-of(IFRS 9 ECL, CBK PG/04 5-bucket prudential).
	ProvisioningCBKPG04 = "CBK_PG_04"
	// ProvisioningIFRS9Only = IFRS 9 ECL only (no CBK prudential overlay).
	ProvisioningIFRS9Only = "IFRS9_ONLY"
)

// validProvisioningKeys is the universe of recognised provisioning rule-set keys.
var validProvisioningKeys = map[string]bool{
	ProvisioningCBKPG04:   true,
	ProvisioningIFRS9Only: true,
}

// ValidProvisioningKey reports whether s is a recognised provisioning rule-set key.
func ValidProvisioningKey(s string) bool {
	return validProvisioningKeys[s]
}

// ─── Defaults ───────────────────────────────────────────────────────────────

// DefaultReportSetFor returns the minimum enabled report set for a license type.
// DCP (the v1 target license) gets IFRS statements + DCP returns + CRB feed +
// AML; bank/MFB additionally get the prudential returns. The result is freshly
// allocated so callers may mutate it safely.
func DefaultReportSetFor(lt LicenseType) []ReportCode {
	base := []ReportCode{
		ReportIFRSTrialBalance, ReportIFRSIncomeStmt, ReportIFRSBalanceSheet, ReportIFRSCashFlow,
		ReportDCPLoanBookReturn, ReportDCPAPRDisclosure, ReportDCPComplaints,
		ReportCRBFeed,
		ReportAMLSTR, ReportAMLCTR,
	}
	switch lt {
	case LicenseMFB, LicenseBank:
		base = append(base,
			ReportCBKClassification, ReportCBKNPLRatio,
			ReportCBKCapitalAdequacy, ReportCBKLiquidity, ReportCBKLargeExposure)
	case LicenseUnregulated:
		// No regulatory returns; IFRS statements only.
		base = []ReportCode{
			ReportIFRSTrialBalance, ReportIFRSIncomeStmt,
			ReportIFRSBalanceSheet, ReportIFRSCashFlow,
		}
	}
	return base
}

// Default profile field values seeded for a new tenant.
const (
	DefaultCountry           = "KE"
	DefaultReportingCurrency = "KES"
	DefaultProvisioningKey   = ProvisioningCBKPG04
)

// ─── Entity ─────────────────────────────────────────────────────────────────

// RegulatoryProfile is a row in the regulatory_profile table: a tenant's active
// regulatory configuration.
type RegulatoryProfile struct {
	ID                     string              `json:"id"`
	TenantID               string              `json:"tenantId"`
	LicenseType            LicenseType         `json:"licenseType"`
	Country                string              `json:"country"`
	ReportingCurrency      string              `json:"reportingCurrency"`
	ProvisioningTableKey   string              `json:"provisioningTableKey"`
	CrbEnabled             bool                `json:"crbEnabled"`
	CrbBureau              *CrbBureau          `json:"crbBureau,omitempty"`
	CrbSubmissionFrequency SubmissionFrequency `json:"crbSubmissionFrequency"`
	ReportSet              []ReportCode        `json:"reportSet"`
	Active                 bool                `json:"active"`
	Notes                  *string             `json:"notes,omitempty"`
	CreatedAt              time.Time           `json:"createdAt"`
	UpdatedAt              time.Time           `json:"updatedAt"`
	CreatedBy              *string             `json:"createdBy,omitempty"`
	UpdatedBy              *string             `json:"updatedBy,omitempty"`
}

// HasReport reports whether the profile's report set contains code.
func (p *RegulatoryProfile) HasReport(code ReportCode) bool {
	for _, c := range p.ReportSet {
		if c == code {
			return true
		}
	}
	return false
}

// ─── Request DTO ────────────────────────────────────────────────────────────

// UpdateProfileRequest is the PUT body for the tenant's regulatory profile. All
// fields are optional; only the supplied (non-nil) fields are applied, so a
// caller can change just the license or just the CRB target.
type UpdateProfileRequest struct {
	LicenseType            *LicenseType         `json:"licenseType,omitempty"`
	Country                *string              `json:"country,omitempty"`
	ReportingCurrency      *string              `json:"reportingCurrency,omitempty"`
	ProvisioningTableKey   *string              `json:"provisioningTableKey,omitempty"`
	CrbEnabled             *bool                `json:"crbEnabled,omitempty"`
	CrbBureau              *CrbBureau           `json:"crbBureau,omitempty"`
	CrbSubmissionFrequency *SubmissionFrequency `json:"crbSubmissionFrequency,omitempty"`
	ReportSet              *[]ReportCode        `json:"reportSet,omitempty"`
	Notes                  *string              `json:"notes,omitempty"`
}
