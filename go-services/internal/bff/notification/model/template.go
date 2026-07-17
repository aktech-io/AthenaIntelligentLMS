package model

import (
	"time"

	"github.com/google/uuid"
)

type Channel string

const (
	ChannelPush  Channel = "PUSH"
	ChannelSMS   Channel = "SMS"
	ChannelEmail Channel = "EMAIL"
	ChannelInApp Channel = "IN_APP"
)

type NotificationTemplate struct {
	ID            uuid.UUID `db:"id"`
	TenantID      string    `db:"tenant_id"`
	TemplateCode  string    `db:"template_code"`
	Channel       Channel   `db:"channel"`
	TitleTemplate *string   `db:"title_template"`
	BodyTemplate  string    `db:"body_template"`
	Category      *string   `db:"category"`
	Active        bool      `db:"active"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}
