package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/invoice/ledger"
	"github.com/athena-lms/go-services/internal/invoice/model"
)

// StartJobs launches the periodic jobs: buyer reminders / auto-decline
// (every 10 min), OVERDUE marking (daily at hourUTC, plus once at start),
// and hourly ledger anchoring.
func (s *Service) StartJobs(ctx context.Context, overdueHourUTC int) {
	go s.loop(ctx, 10*time.Minute, s.RunReminders)
	go s.loop(ctx, time.Hour, s.RunAnchors)
	go func() {
		s.RunOverdue(ctx)
		for {
			now := time.Now().UTC()
			next := time.Date(now.Year(), now.Month(), now.Day(), overdueHourUTC, 0, 0, 0, time.UTC)
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				s.RunOverdue(ctx)
			}
		}
	}()
}

func (s *Service) loop(ctx context.Context, every time.Duration, fn func(context.Context)) {
	t := time.NewTicker(every)
	defer t.Stop()
	fn(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

// RunReminders sends +2d/+4d reminders and auto-declines at +5d (SPEC §3.3).
func (s *Service) RunReminders(ctx context.Context) {
	invs, err := s.repo.InvoicesByStatus(ctx, []string{model.StatusAwaitingBuyer}, 500)
	if err != nil {
		s.logger.Warn("reminder scan failed", zap.Error(err))
		return
	}
	for i := range invs {
		inv := invs[i]
		conf, err := s.repo.LatestConfirmation(ctx, nil, inv.ID)
		if err != nil || conf == nil {
			continue
		}
		age := s.now().Sub(conf.CreatedAt)
		tctx := auth.WithUserID(auth.WithTenantID(ctx, inv.TenantID), Source)
		if age >= s.cfg.ConfirmTTL || s.now().After(conf.ExpiresAt) {
			err := s.withTx(tctx, func(tx pgx.Tx) error {
				cur, err := s.repo.GetInvoiceForUpdate(tctx, tx, inv.ID)
				if err != nil || cur.Status != model.StatusAwaitingBuyer {
					return err
				}
				return s.transition(tctx, tx, cur, model.StatusDeclined, model.ReasonBuyerNoResponse, model.EvInvoiceDeclined, map[string]any{"confirmationId": conf.ID.String(), "remindersSent": conf.RemindersSent})
			})
			if err != nil {
				s.logger.Warn("auto-decline failed", zap.String("invoiceId", inv.ID.String()), zap.Error(err))
			}
			continue
		}
		due := 0
		for _, after := range s.cfg.ReminderAfter {
			if age >= after {
				due++
			}
		}
		if conf.RemindersSent >= due || conf.SentTo == "" {
			continue
		}
		link := ""
		if conf.DemoToken != "" {
			link = s.ConfirmURL(conf.DemoToken)
		} else {
			link = strings.TrimRight(s.cfg.PublicPortalURL, "/") + "/confirm (use the link from our earlier email)"
		}
		body := fmt.Sprintf("Reminder: %s has asked Nemo to finance invoice %s issued to you. Please confirm or dispute it: %s", inv.Seller.Name, inv.InvoiceNumber, link)
		sendErr := s.clients.Notify.Send(tctx, "EMAIL", conf.SentTo, "Reminder: please confirm invoice "+inv.InvoiceNumber, body, map[string]any{"template": "invoice.buyer.remind", "invoiceId": inv.ID.String()})
		conf.RemindersSent++
		_ = s.repo.UpdateConfirmation(tctx, nil, conf)
		_ = s.withTx(tctx, func(tx pgx.Tx) error {
			payload := map[string]any{"sentTo": conf.SentTo, "reminder": conf.RemindersSent, "delivered": sendErr == nil}
			if sendErr != nil {
				payload["deliveryError"] = sendErr.Error()
			}
			_, err := s.appendEvent(tctx, tx, &inv, model.EvBuyerReminded, payload, Source)
			return err
		})
	}
}

// RunOverdue marks ACTIVE advances past due_date + grace as OVERDUE.
func (s *Service) RunOverdue(ctx context.Context) {
	advs, err := s.repo.OverdueCandidates(ctx, s.cfg.OverdueGraceDays)
	if err != nil {
		s.logger.Warn("overdue scan failed", zap.Error(err))
		return
	}
	for i := range advs {
		adv := advs[i]
		err := s.withTx(ctx, func(tx pgx.Tx) error {
			inv, err := s.repo.GetInvoiceForUpdate(ctx, tx, adv.InvoiceID)
			if err != nil {
				return err
			}
			if inv.Status != model.StatusFunded && inv.Status != model.StatusPartiallyPaid {
				return nil
			}
			a, err := s.repo.AdvanceByInvoice(ctx, tx, adv.InvoiceID, true)
			if err != nil || a == nil || a.Status != model.AdvanceActive {
				return err
			}
			a.Status = model.AdvanceOverdue
			if err := s.repo.UpdateAdvance(ctx, tx, a); err != nil {
				return err
			}
			tctx := auth.WithUserID(auth.WithTenantID(ctx, inv.TenantID), Source)
			daysLate := int(s.now().Sub(a.DueDate).Hours() / 24)
			return s.transition(tctx, tx, inv, model.StatusOverdue, "PAST_DUE", model.EvAdvanceOverdue, map[string]any{"advanceId": a.ID.String(), "dueDate": a.DueDate, "daysLate": daysLate, "outstanding": a.Outstanding(), "graceDays": s.cfg.OverdueGraceDays})
		})
		if err != nil {
			s.logger.Warn("overdue marking failed", zap.String("advanceId", adv.ID.String()), zap.Error(err))
		}
	}
}

// RunAnchors computes one Merkle root per tenant over all unanchored events
// and stores it (method NONE in v1; OTS via ledger.OTSAnchorer later).
func (s *Service) RunAnchors(ctx context.Context) {
	tenants, err := s.repo.TenantsWithUnanchored(ctx)
	if err != nil {
		s.logger.Warn("anchor scan failed", zap.Error(err))
		return
	}
	for _, tenant := range tenants {
		if _, err := s.AnchorTenant(ctx, tenant); err != nil {
			s.logger.Warn("anchoring failed", zap.String("tenant", tenant), zap.Error(err))
		}
	}
}

// AnchorTenant anchors a tenant's unanchored events now (also used by tests/ops).
func (s *Service) AnchorTenant(ctx context.Context, tenant string) (*model.Anchor, error) {
	var anchor *model.Anchor
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ledger_events:' || $1))`, tenant); err != nil {
			return err
		}
		events, err := s.repo.UnanchoredEvents(ctx, tx, tenant, 50000)
		if err != nil || len(events) == 0 {
			return err
		}
		hashes := make([]string, len(events))
		for i, ev := range events {
			hashes[i] = ev.Hash
		}
		root := ledger.MerkleRoot(hashes)
		method := s.anchorer.Method()
		receipt, aerr := s.anchorer.Anchor(root)
		if aerr != nil {
			s.logger.Warn("external anchoring failed; storing root with method NONE", zap.Error(aerr))
			method, receipt = "NONE", nil
		}
		anchor = &model.Anchor{ID: uuid.New(), TenantID: tenant, MerkleRoot: root, FromSeq: events[0].Seq, ToSeq: events[len(events)-1].Seq, EventCount: len(events), Method: method, Receipt: receipt, AnchoredAt: s.now().UTC()}
		return s.repo.CreateAnchor(ctx, tx, anchor)
	})
	return anchor, err
}

// ─── payment.completed consumer ──────────────────────────────────────────────

// paymentCompleted mirrors payment-service's event payload (subset).
type paymentCompleted struct {
	PaymentID         string          `json:"paymentId"`
	CustomerID        string          `json:"customerId"`
	PaymentType       string          `json:"paymentType"`
	Amount            decimal.Decimal `json:"amount"`
	Currency          string          `json:"currency"`
	ExternalReference string          `json:"externalReference"`
	InternalReference string          `json:"internalReference"`
}

// HandlePaymentEvent applies payment.completed events whose externalReference
// is "invoice:<uuid>[:<ref>]" (buyer payments collected elsewhere). Our own
// repayments use the "invadv:" prefix and are ignored to avoid a loop.
func (s *Service) HandlePaymentEvent(ctx context.Context, evt *event.DomainEvent) error {
	if evt.Type != event.PaymentCompleted {
		return nil
	}
	var p paymentCompleted
	if err := evt.UnmarshalPayload(&p); err != nil {
		return nil // malformed: ack, nothing we can do
	}
	ref := strings.TrimSpace(p.ExternalReference)
	if !strings.HasPrefix(strings.ToLower(ref), "invoice:") {
		return nil
	}
	parts := strings.SplitN(ref, ":", 3)
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil
	}
	payRef := p.PaymentID
	if len(parts) == 3 && parts[2] != "" {
		payRef = parts[2]
	}
	tenant := evt.TenantID
	if tenant == "" {
		tenant = "default"
	}
	ctx = auth.WithUserID(auth.WithTenantID(ctx, tenant), "payment-service")
	_, err = s.RecordPayment(ctx, id, model.PaymentRequest{Amount: p.Amount, Reference: payRef, Payer: p.CustomerID}, "EVENT")
	if err != nil {
		if strings.Contains(err.Error(), "already recorded") {
			return nil
		}
		s.logger.Warn("payment.completed not applied", zap.String("invoiceId", id.String()), zap.Error(err))
		// Business errors (not funded, etc.) are final; only transport-ish errors should requeue.
		if strings.Contains(err.Error(), "core banking") {
			return err
		}
	}
	return nil
}
