package event

import (
	"context"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/accounting/model"
	"github.com/athena-lms/go-services/internal/common/event"
)

// Publisher publishes accounting domain events.
type Publisher struct {
	pub    *event.Publisher
	logger *zap.Logger
}

// NewPublisher creates a new accounting event publisher.
func NewPublisher(pub *event.Publisher, logger *zap.Logger) *Publisher {
	return &Publisher{pub: pub, logger: logger}
}

// BuildJournalPosted constructs the "accounting.posted" DomainEvent WITHOUT
// publishing it. Used by the transactional-outbox path so the event is persisted
// atomically with the journal-entry insert and delivered at-least-once by the
// relay. Safe to use as a builder callback after the entry's ID is assigned.
func (p *Publisher) BuildJournalPosted(entry *model.JournalEntry) (*event.DomainEvent, error) {
	sourceEvent := ""
	if entry.SourceEvent != nil {
		sourceEvent = *entry.SourceEvent
	}
	sourceID := ""
	if entry.SourceID != nil {
		sourceID = *entry.SourceID
	}

	payload := map[string]any{
		"entryId":     entry.ID.String(),
		"reference":   entry.Reference,
		"entryDate":   entry.EntryDate.Format("2006-01-02"),
		"sourceEvent": sourceEvent,
		"sourceId":    sourceID,
		"totalDebit":  entry.TotalDebit.String(),
		"totalCredit": entry.TotalCredit.String(),
	}

	return event.NewDomainEvent("accounting.posted", "accounting-service", entry.TenantID, "", payload)
}

// PublishJournalPosted publishes an "accounting.posted" event after a journal entry is committed.
func (p *Publisher) PublishJournalPosted(ctx context.Context, entry *model.JournalEntry) {
	evt, err := p.BuildJournalPosted(entry)
	if err != nil {
		p.logger.Error("Failed to create accounting.posted event", zap.Error(err))
		return
	}

	if err := p.pub.Publish(ctx, evt); err != nil {
		p.logger.Error("Failed to publish accounting.posted event",
			zap.String("entryId", entry.ID.String()),
			zap.Error(err))
		return
	}

	p.logger.Info("Published accounting.posted",
		zap.String("entryId", entry.ID.String()),
		zap.String("reference", entry.Reference))
}
