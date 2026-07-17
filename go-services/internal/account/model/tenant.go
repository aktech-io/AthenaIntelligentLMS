package model

import "time"

// ─── Tenant ───────────────────────────────────────────────────────────────────

// TenantStatus enumerates the tenant lifecycle states.
type TenantStatus string

const (
	// TenantStatusProvisioning is the state a tenant is created in: settings,
	// admin user and the tenant.provisioned event are committed, but the tenant
	// awaits explicit activation (maker-checker friendly: a second admin
	// activates).
	TenantStatusProvisioning TenantStatus = "PROVISIONING"
	TenantStatusActive       TenantStatus = "ACTIVE"
	TenantStatusSuspended    TenantStatus = "SUSPENDED"
)

// ValidTenantStatus reports whether s is a known tenant status.
func ValidTenantStatus(s string) bool {
	switch TenantStatus(s) {
	case TenantStatusProvisioning, TenantStatusActive, TenantStatusSuspended:
		return true
	}
	return false
}

// Tenant is one provisioned neobank in the platform registry (Nemo gap C1).
// ID is the tenant slug used as tenant_id across every service.
type Tenant struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	MarketCode  string       `json:"marketCode"`
	Status      TenantStatus `json:"status"`
	CreatedBy   *string      `json:"createdBy,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}
