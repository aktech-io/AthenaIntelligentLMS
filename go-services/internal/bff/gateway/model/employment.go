package model

import (
	"time"

	"github.com/google/uuid"
)

type UserEmployment struct {
	ID               uuid.UUID `db:"id" json:"id"`
	TenantID         string    `db:"tenant_id" json:"tenantId"`
	UserID           uuid.UUID `db:"user_id" json:"userId"`
	EmployerName     *string   `db:"employer_name" json:"employerName,omitempty"`
	JobTitle         *string   `db:"job_title" json:"jobTitle,omitempty"`
	MonthlyIncome    *float64  `db:"monthly_income" json:"monthlyIncome,omitempty"`
	EmploymentStatus *string   `db:"employment_status" json:"employmentStatus,omitempty"`
	CreatedAt        time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt        time.Time `db:"updated_at" json:"updatedAt"`
}
