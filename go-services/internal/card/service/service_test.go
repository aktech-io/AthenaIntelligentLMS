package service

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/card/model"
	"github.com/athena-lms/go-services/internal/card/processor"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/event"
)

// fakeRepo is an in-memory Repository. It records every audit row and outbox
// event so tests can assert the atomic side effects.
type fakeRepo struct {
	cards  map[uuid.UUID]model.Card
	audits []model.CardEvent
	events []event.DomainEvent
}

func newFakeRepo() *fakeRepo { return &fakeRepo{cards: map[uuid.UUID]model.Card{}} }

func (f *fakeRepo) InsertCard(_ context.Context, c *model.Card, audit *model.CardEvent, evt *event.DomainEvent) (*model.Card, error) {
	f.cards[c.ID] = *c
	f.audits = append(f.audits, *audit)
	f.events = append(f.events, *evt)
	out := *c
	return &out, nil
}

func (f *fakeRepo) UpdateCard(_ context.Context, c *model.Card, audit *model.CardEvent, evt *event.DomainEvent) (*model.Card, error) {
	stored, ok := f.cards[c.ID]
	if !ok || stored.TenantID != c.TenantID {
		return nil, stderrors.New("update card: not found for tenant")
	}
	f.cards[c.ID] = *c
	f.audits = append(f.audits, *audit)
	f.events = append(f.events, *evt)
	out := *c
	return &out, nil
}

func (f *fakeRepo) FindByTenantAndID(_ context.Context, tenantID string, id uuid.UUID) (*model.Card, error) {
	c, ok := f.cards[id]
	if !ok || c.TenantID != tenantID {
		return nil, nil
	}
	out := c
	return &out, nil
}

func (f *fakeRepo) FindByTenant(_ context.Context, tenantID string, customerID *uuid.UUID) ([]model.Card, error) {
	var result []model.Card
	for _, c := range f.cards {
		if c.TenantID != tenantID {
			continue
		}
		if customerID != nil && c.CustomerID != *customerID {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (f *fakeRepo) FindEventsByCard(_ context.Context, tenantID string, cardID uuid.UUID, _, _ int) ([]model.CardEvent, error) {
	var result []model.CardEvent
	for _, e := range f.audits {
		if e.TenantID == tenantID && e.CardID == cardID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (f *fakeRepo) eventTypes() []string {
	var types []string
	for _, e := range f.events {
		types = append(types, e.Type)
	}
	return types
}

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	return New(repo, processor.Sandbox{}, zap.NewNop()), repo
}

func issueReq() *model.IssueCardRequest {
	return &model.IssueCardRequest{
		CustomerID:     uuid.New(),
		AccountID:      uuid.New(),
		CardholderName: "JANE WANJIKU",
	}
}

// Issue a virtual card end-to-end on the sandbox: ACTIVE, last4 + ref stored,
// card.issued through the outbox, audit row written. And critically: nothing
// PAN-shaped exists on the persisted model.
func TestIssueCard_SandboxEndToEnd(t *testing.T) {
	svc, repo := newTestService()

	card, err := svc.IssueCard(context.Background(), issueReq(), "tenant-a")
	if err != nil {
		t.Fatalf("IssueCard: %v", err)
	}
	if card.Status != model.CardStatusActive {
		t.Errorf("virtual card status = %s, want ACTIVE", card.Status)
	}
	if card.Type != model.CardTypeVirtual || card.Network != model.CardNetworkMastercard || card.Currency != "KES" {
		t.Errorf("defaults not applied: %+v", card)
	}
	if len(card.PanLast4) != 4 {
		t.Errorf("PanLast4 = %q, want 4 digits", card.PanLast4)
	}
	if card.Processor != "sandbox" || card.ProcessorRef == "" {
		t.Errorf("processor identity not stored: %+v", card)
	}
	if got := repo.eventTypes(); len(got) != 1 || got[0] != event.CardIssued {
		t.Errorf("outbox events = %v, want [card.issued]", got)
	}
	if len(repo.audits) != 1 || repo.audits[0].EventType != event.CardIssued {
		t.Errorf("audit trail = %+v, want one card.issued row", repo.audits)
	}
}

// Physical cards land in REQUESTED (pending activation).
func TestIssueCard_PhysicalPendingActivation(t *testing.T) {
	svc, _ := newTestService()
	req := issueReq()
	req.Type = model.CardTypePhysical

	card, err := svc.IssueCard(context.Background(), req, "tenant-a")
	if err != nil {
		t.Fatalf("IssueCard: %v", err)
	}
	if card.Status != model.CardStatusRequested {
		t.Errorf("physical card status = %s, want REQUESTED", card.Status)
	}
}

// A processor decline surfaces as a BusinessError and persists nothing.
func TestIssueCard_ProcessorDecline(t *testing.T) {
	svc, repo := newTestService()
	req := issueReq()
	req.CardholderName = "DECLINE ME"

	_, err := svc.IssueCard(context.Background(), req, "tenant-a")
	var be *errors.BusinessError
	if !stderrors.As(err, &be) {
		t.Fatalf("expected BusinessError, got %T: %v", err, err)
	}
	if len(repo.cards) != 0 || len(repo.events) != 0 {
		t.Error("declined issuance must persist nothing")
	}
}

// Validation failures reject before touching the processor.
func TestIssueCard_Validation(t *testing.T) {
	svc, _ := newTestService()
	req := issueReq()
	req.CustomerID = uuid.Nil

	_, err := svc.IssueCard(context.Background(), req, "tenant-a")
	var be *errors.BusinessError
	if !stderrors.As(err, &be) || be.StatusCode != 400 {
		t.Fatalf("expected 400 BusinessError, got %v", err)
	}
}

// Tenant scoping: another tenant can neither read nor mutate the card.
func TestTenantScoping(t *testing.T) {
	svc, _ := newTestService()
	card, _ := svc.IssueCard(context.Background(), issueReq(), "tenant-a")

	var nfe *errors.NotFoundError
	if _, err := svc.GetCard(context.Background(), "tenant-b", card.ID); !stderrors.As(err, &nfe) {
		t.Errorf("cross-tenant GetCard err = %v, want NotFoundError", err)
	}
	if _, err := svc.FreezeCard(context.Background(), "tenant-b", card.ID); !stderrors.As(err, &nfe) {
		t.Errorf("cross-tenant FreezeCard err = %v, want NotFoundError", err)
	}

	cards, _ := svc.ListCards(context.Background(), "tenant-b", nil)
	if len(cards) != 0 {
		t.Errorf("tenant-b sees %d cards, want 0", len(cards))
	}
}

// Freeze then unfreeze: state machine + one event per real transition.
// Second freeze is idempotent — same state back, NO duplicate event.
func TestFreezeUnfreezeLifecycle(t *testing.T) {
	svc, repo := newTestService()
	card, _ := svc.IssueCard(context.Background(), issueReq(), "tenant-a")

	frozen, err := svc.FreezeCard(context.Background(), "tenant-a", card.ID)
	if err != nil {
		t.Fatalf("FreezeCard: %v", err)
	}
	if frozen.Status != model.CardStatusFrozen {
		t.Errorf("status = %s, want FROZEN", frozen.Status)
	}

	// Idempotent second freeze: no error, no extra event.
	before := len(repo.events)
	again, err := svc.FreezeCard(context.Background(), "tenant-a", card.ID)
	if err != nil {
		t.Fatalf("idempotent FreezeCard: %v", err)
	}
	if again.Status != model.CardStatusFrozen {
		t.Errorf("idempotent freeze status = %s", again.Status)
	}
	if len(repo.events) != before {
		t.Errorf("idempotent freeze emitted %d extra events", len(repo.events)-before)
	}

	unfrozen, err := svc.UnfreezeCard(context.Background(), "tenant-a", card.ID)
	if err != nil {
		t.Fatalf("UnfreezeCard: %v", err)
	}
	if unfrozen.Status != model.CardStatusActive {
		t.Errorf("status = %s, want ACTIVE", unfrozen.Status)
	}

	want := []string{event.CardIssued, event.CardFrozen, event.CardUnfrozen}
	got := repo.eventTypes()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// Blocked cards are immutable: no unfreeze, freeze, or limit changes; block
// itself is idempotent.
func TestBlockedCardImmutability(t *testing.T) {
	svc, repo := newTestService()
	card, _ := svc.IssueCard(context.Background(), issueReq(), "tenant-a")

	blocked, err := svc.BlockCard(context.Background(), "tenant-a", card.ID, "STOLEN")
	if err != nil {
		t.Fatalf("BlockCard: %v", err)
	}
	if blocked.Status != model.CardStatusBlocked {
		t.Errorf("status = %s, want BLOCKED", blocked.Status)
	}

	var be *errors.BusinessError
	if _, err := svc.UnfreezeCard(context.Background(), "tenant-a", card.ID); !stderrors.As(err, &be) || be.StatusCode != 409 {
		t.Errorf("unfreeze blocked card err = %v, want 409 conflict", err)
	}
	if _, err := svc.FreezeCard(context.Background(), "tenant-a", card.ID); !stderrors.As(err, &be) || be.StatusCode != 409 {
		t.Errorf("freeze blocked card err = %v, want 409 conflict", err)
	}
	if _, err := svc.SetLimits(context.Background(), "tenant-a", card.ID, model.DefaultLimits()); !stderrors.As(err, &be) || be.StatusCode != 409 {
		t.Errorf("set limits on blocked card err = %v, want 409 conflict", err)
	}

	// Idempotent re-block: success, no extra event.
	before := len(repo.events)
	if _, err := svc.BlockCard(context.Background(), "tenant-a", card.ID, "STOLEN"); err != nil {
		t.Errorf("idempotent BlockCard: %v", err)
	}
	if len(repo.events) != before {
		t.Error("idempotent block emitted an extra event")
	}
}

// Limits: update persists, emits card.limits.changed, rejects negatives.
func TestSetLimits(t *testing.T) {
	svc, repo := newTestService()
	card, _ := svc.IssueCard(context.Background(), issueReq(), "tenant-a")

	limits := model.SpendingLimits{
		PerTransaction: decimal.NewFromInt(10000),
		Daily:          decimal.NewFromInt(50000),
		Monthly:        decimal.NewFromInt(300000),
		ECommerce:      true, POS: true,
	}
	updated, err := svc.SetLimits(context.Background(), "tenant-a", card.ID, limits)
	if err != nil {
		t.Fatalf("SetLimits: %v", err)
	}
	if !updated.Limits.Daily.Equal(decimal.NewFromInt(50000)) {
		t.Errorf("daily limit = %s, want 50000", updated.Limits.Daily)
	}
	if updated.Limits.ATM {
		t.Error("ATM should be disabled after update")
	}
	if got := repo.eventTypes(); got[len(got)-1] != event.CardLimitsChanged {
		t.Errorf("last event = %s, want card.limits.changed", got[len(got)-1])
	}

	bad := limits
	bad.Daily = decimal.NewFromInt(-1)
	var be *errors.BusinessError
	if _, err := svc.SetLimits(context.Background(), "tenant-a", card.ID, bad); !stderrors.As(err, &be) || be.StatusCode != 400 {
		t.Errorf("negative limit err = %v, want 400", err)
	}
}

// ListCards filters by customer; the audit trail is readable per card.
func TestListAndEvents(t *testing.T) {
	svc, _ := newTestService()
	req1 := issueReq()
	card1, _ := svc.IssueCard(context.Background(), req1, "tenant-a")
	svc.IssueCard(context.Background(), issueReq(), "tenant-a")

	all, _ := svc.ListCards(context.Background(), "tenant-a", nil)
	if len(all) != 2 {
		t.Errorf("all cards = %d, want 2", len(all))
	}
	mine, _ := svc.ListCards(context.Background(), "tenant-a", &req1.CustomerID)
	if len(mine) != 1 || mine[0].ID != card1.ID {
		t.Errorf("customer filter returned %d cards", len(mine))
	}

	svc.FreezeCard(context.Background(), "tenant-a", card1.ID)
	events, err := svc.ListCardEvents(context.Background(), "tenant-a", card1.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListCardEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("card events = %d, want 2 (issued + frozen)", len(events))
	}
}
