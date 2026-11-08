package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/decision"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/invoice/corebanking"
	"github.com/athena-lms/go-services/internal/invoice/fingerprint"
	"github.com/athena-lms/go-services/internal/invoice/ledger"
	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/pipeline"
	"github.com/athena-lms/go-services/internal/invoice/policy"
)

// UploadInput is POST /invoices.
type UploadInput struct {
	FileName         string
	Data             []byte
	SellerCustomerID string
	RequestedAmount  *decimal.Decimal
	BuyerEmail       string
	SellerName       string
	SellerEmail      string
	InvoiceNumber    string
}

// ErrDuplicate is returned (with the stored rejected record) on a duplicate upload.
type ErrDuplicate struct {
	Invoice     *model.Invoice
	DuplicateOf uuid.UUID
}

func (e *ErrDuplicate) Error() string {
	return fmt.Sprintf("duplicate invoice (matches %s)", e.DuplicateOf)
}

// Upload stores the document, fingerprints it, rejects duplicates (409) and
// starts the pipeline. SyncWait bounds how long we run the pipeline inline
// before returning (so the demo response can carry the confirm link).
func (s *Service) Upload(ctx context.Context, in UploadInput, syncWait time.Duration) (*model.UploadResponse, error) {
	tenantID := auth.TenantIDOrDefault(ctx)
	if len(in.Data) == 0 {
		return nil, cerrors.BadRequest("file is required")
	}
	if strings.TrimSpace(in.SellerCustomerID) == "" {
		return nil, cerrors.BadRequest("sellerCustomerId is required")
	}
	fp := fingerprint.Bytes(in.Data)
	inv := &model.Invoice{
		ID: uuid.New(), TenantID: tenantID, SellerCustomerID: strings.TrimSpace(in.SellerCustomerID), Fingerprint: fp, FileName: in.FileName,
		InvoiceNumber: strings.TrimSpace(in.InvoiceNumber), RequestedAmount: in.RequestedAmount, Status: model.StatusUploaded,
		Seller: model.Party{Name: in.SellerName, Email: in.SellerEmail}, Buyer: model.Party{Email: strings.TrimSpace(in.BuyerEmail)},
		LineItems: []model.LineItem{}, NeedsConfirm: []string{}, FraudFlags: []string{}, CreatedBy: actorFrom(ctx), Currency: s.cfg.Currency,
	}
	dup, err := s.repo.FindDuplicate(ctx, tenantID, fp, inv.SellerCustomerID, inv.InvoiceNumber, "")
	if err != nil {
		return nil, err
	}
	if dup != nil {
		inv.Status, inv.StatusReason = model.StatusDeclined, model.ReasonDuplicate
		err := s.withTx(ctx, func(tx pgx.Tx) error {
			if err := s.repo.CreateInvoice(ctx, tx, inv); err != nil {
				return err
			}
			_, err := s.appendEvent(ctx, tx, inv, model.EvInvoiceDuplicateRejected, map[string]any{"duplicateOf": dup.ID.String(), "fingerprint": fp, "matchedOn": "fingerprint"}, "")
			return err
		})
		if err != nil {
			return nil, err
		}
		return nil, &ErrDuplicate{Invoice: inv, DuplicateOf: dup.ID}
	}
	// Store the document first (outside the tx; a media outage must not block).
	mctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	mediaID, merr := s.clients.Media.Upload(mctx, in.FileName, in.Data, inv.ID.String())
	cancel()
	if merr != nil {
		s.logger.Warn("media upload failed; continuing without media id", zap.Error(merr))
	}
	inv.MediaID = mediaID
	s.files.Store(inv.ID, in.Data)
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CreateInvoice(ctx, tx, inv); err != nil {
			return err
		}
		payload := map[string]any{"fingerprint": fp, "fileName": in.FileName, "bytes": len(in.Data), "mediaId": mediaID, "sellerCustomerId": inv.SellerCustomerID, "requestedAmount": in.RequestedAmount}
		if merr != nil {
			payload["mediaError"] = merr.Error()
		}
		_, err := s.appendEvent(ctx, tx, inv, model.EvInvoiceUploaded, payload, "")
		return err
	})
	if err != nil {
		s.files.Delete(inv.ID)
		return nil, err
	}
	// Run the pipeline inline for a bounded time, then hand off to the background.
	if syncWait > 0 {
		sctx, cancel := context.WithTimeout(ctx, syncWait)
		s.Advance(sctx, inv.ID)
		cancel()
	}
	s.AdvanceAsync(ctx, inv.ID)
	fresh, err := s.repo.GetInvoice(ctx, nil, tenantID, inv.ID)
	if err != nil {
		return nil, err
	}
	resp := &model.UploadResponse{Invoice: fresh}
	if s.cfg.DemoMode {
		if conf, _ := s.repo.LatestConfirmation(ctx, nil, inv.ID); conf != nil && conf.DemoToken != "" {
			resp.ConfirmURL = s.ConfirmURL(conf.DemoToken)
			resp.DemoOTP = demoOTP
		}
	}
	return resp, nil
}

// Get returns the full record.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*model.InvoiceDetail, error) {
	inv, err := s.repo.GetInvoice(ctx, nil, auth.TenantIDOrDefault(ctx), id)
	if err != nil {
		return nil, err
	}
	d := &model.InvoiceDetail{Invoice: inv, Payments: []model.Payment{}}
	if inv.BuyerID != nil {
		d.Buyer, _ = s.repo.GetBuyer(ctx, nil, *inv.BuyerID)
	}
	d.Offer, _ = s.repo.LatestOffer(ctx, nil, id)
	d.Advance, _ = s.repo.AdvanceByInvoice(ctx, nil, id, false)
	d.Decision, _ = s.repo.LatestDecision(ctx, nil, id)
	if p, err := s.repo.PaymentsByInvoice(ctx, id); err == nil {
		d.Payments = p
	}
	d.Confirmation, _ = s.repo.LatestConfirmation(ctx, nil, id)
	if s.cfg.DemoMode && d.Confirmation != nil && d.Confirmation.DemoToken != "" && d.Confirmation.Action == "" {
		d.ConfirmURL = s.ConfirmURL(d.Confirmation.DemoToken)
	}
	return d, nil
}

// List pages invoices.
func (s *Service) List(ctx context.Context, f model.ListFilter) (*model.Page[model.Invoice], error) {
	return s.repo.ListInvoices(ctx, auth.TenantIDOrDefault(ctx), f)
}

// Timeline returns the invoice's ledger events and re-verifies the tenant
// chain across the contiguous seq range they span (interleaved events of
// other invoices included in the check, excluded from the output).
func (s *Service) Timeline(ctx context.Context, id uuid.UUID) (*model.Timeline, error) {
	inv, err := s.repo.GetInvoice(ctx, nil, auth.TenantIDOrDefault(ctx), id)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.EventsByInvoice(ctx, id)
	if err != nil {
		return nil, err
	}
	tl := &model.Timeline{InvoiceID: id, Events: events, ChainOK: true}
	if len(events) > 0 {
		rng, err := s.repo.EventsRange(ctx, inv.TenantID, events[0].Seq, events[len(events)-1].Seq)
		if err != nil {
			return nil, err
		}
		ok, broken := ledger.Verify(rng, s.repo.HMACKey())
		tl.ChainOK = ok
		if !ok {
			tl.BrokenSeq = &broken
		}
	}
	return tl, nil
}

// ResendConfirmation issues a fresh token/OTP to the buyer.
func (s *Service) ResendConfirmation(ctx context.Context, id uuid.UUID, email string) (string, error) {
	inv, err := s.repo.GetInvoice(ctx, nil, auth.TenantIDOrDefault(ctx), id)
	if err != nil {
		return "", err
	}
	if inv.Status != model.StatusAwaitingBuyer && inv.Status != model.StatusExtracted && inv.Status != model.StatusBuyerDisputed {
		return "", cerrors.NewBusinessError("invoice is not awaiting buyer confirmation")
	}
	if email != "" {
		inv.Buyer.Email = strings.TrimSpace(email)
		_, _ = s.repo.Pool().Exec(ctx, `UPDATE invoices SET buyer_snapshot = buyer_snapshot || jsonb_build_object('email',$2::text), updated_at=now() WHERE id=$1`, inv.ID, inv.Buyer.Email)
	}
	if inv.Status == model.StatusExtracted {
		token, err := s.issueConfirmation(ctx, inv, true)
		return token, err
	}
	if inv.Status == model.StatusBuyerDisputed {
		if err := s.withTx(ctx, func(tx pgx.Tx) error {
			cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, id)
			if err != nil {
				return err
			}
			return s.transition(ctx, tx, cur, model.StatusAwaitingBuyer, "RECONFIRM", model.EvBuyerNotified, map[string]any{"reopened": true})
		}); err != nil {
			return "", err
		}
	}
	return s.issueConfirmation(ctx, inv, false)
}

// AcceptOffer moves OFFERED → ACCEPTED and starts funding.
func (s *Service) AcceptOffer(ctx context.Context, id uuid.UUID) (*model.InvoiceDetail, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.TenantID != auth.TenantIDOrDefault(ctx) {
			return cerrors.NotFound("invoice not found")
		}
		if inv.Status != model.StatusOffered {
			return cerrors.NewBusinessError("invoice has no open offer (status " + inv.Status + ")")
		}
		offer, err := s.repo.LatestOffer(ctx, tx, id)
		if err != nil || offer == nil || offer.Status != model.OfferOffered {
			return cerrors.NewBusinessError("no open offer")
		}
		if s.now().After(offer.ExpiresAt) {
			offer.Status = model.OfferExpired
			_ = s.repo.UpdateOffer(ctx, tx, offer)
			return cerrors.NewBusinessError("offer expired")
		}
		now := s.now().UTC()
		offer.Status, offer.AcceptedBy, offer.AcceptedAt = model.OfferAccepted, actorFrom(ctx), &now
		if err := s.repo.UpdateOffer(ctx, tx, offer); err != nil {
			return err
		}
		reason := ""
		if offer.AdvanceAmount.GreaterThan(s.cfg.SecondApprovalAbove) && inv.ApprovedBy == "" {
			reason = model.ReasonAwaitingSecondApproval
		}
		return s.transition(ctx, tx, inv, model.StatusAccepted, reason, model.EvOfferAccepted, map[string]any{"offerId": offer.ID.String(), "acceptedBy": offer.AcceptedBy, "advanceAmount": offer.AdvanceAmount})
	})
	if err != nil {
		return nil, err
	}
	s.AdvanceAsync(ctx, id)
	return s.Get(ctx, id)
}

// Approve is the MANAGER money gate: REFER → APPROVE (creates the offer) or
// second approval of a large disbursement.
func (s *Service) Approve(ctx context.Context, id uuid.UUID, note string) (*model.InvoiceDetail, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.TenantID != auth.TenantIDOrDefault(ctx) {
			return cerrors.NotFound("invoice not found")
		}
		actor := actorFrom(ctx)
		now := s.now().UTC()
		switch {
		case inv.Status == model.StatusDecided:
			parent, _ := s.repo.LatestDecision(ctx, tx, id)
			terms := policy.Terms{AdvanceRate: decimal.RequireFromString("0.70"), FeePerDay: decimal.RequireFromString("0.0009")}
			var parentID *uuid.UUID
			if parent != nil {
				parentID = &parent.ID
				if t, ok := policy.TermsFrom(parent.Detail); ok {
					terms = t
				}
			}
			human := &model.Decision{
				InvoiceID: inv.ID, TenantID: inv.TenantID, PolicyID: policy.ID, Outcome: decision.Approve,
				Inputs: map[string]any{"note": note, "reviewOf": parentOutcome(parent)}, Detail: map[string]any{"advance_rate": terms.AdvanceRate, "fee_per_day": terms.FeePerDay},
				Reasons: []model.DecisionReason{{Code: "HUMAN_REVIEW", Detail: note}}, ActorType: decision.ActorHuman, ActorID: actor, ParentID: parentID, DecidedAt: now,
			}
			if parent != nil {
				human.PolicyVersion, human.PolicyHash = parent.PolicyVersion, parent.PolicyHash
			}
			if err := s.repo.CreateDecision(ctx, tx, human); err != nil {
				return err
			}
			inv.ApprovedBy, inv.ApprovedAt, inv.DecisionOutcome = actor, &now, decision.Approve
			return s.createOffer(ctx, tx, inv, terms, model.EvReviewApproved)
		case inv.Status == model.StatusAccepted && inv.StatusReason == model.ReasonAwaitingSecondApproval:
			inv.ApprovedBy, inv.ApprovedAt = actor, &now
			inv.StatusReason = "SECOND_APPROVAL_GRANTED"
			if err := s.repo.UpdateInvoice(ctx, tx, inv); err != nil {
				return err
			}
			_, err := s.appendEvent(ctx, tx, inv, model.EvReviewApproved, map[string]any{"gate": "SECOND_APPROVAL", "note": note, "approvedBy": actor}, actor)
			return err
		}
		return cerrors.NewBusinessError("invoice is not awaiting approval (status " + inv.Status + ")")
	})
	if err != nil {
		return nil, err
	}
	s.AdvanceAsync(ctx, id)
	return s.Get(ctx, id)
}

func parentOutcome(d *model.Decision) string {
	if d == nil {
		return ""
	}
	return d.Outcome
}

// Reject is the MANAGER rejection of a referred / gated invoice.
func (s *Service) Reject(ctx context.Context, id uuid.UUID, reason string) (*model.InvoiceDetail, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, cerrors.BadRequest("reason is required")
	}
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.TenantID != auth.TenantIDOrDefault(ctx) {
			return cerrors.NotFound("invoice not found")
		}
		switch inv.Status {
		case model.StatusDecided, model.StatusOffered, model.StatusAccepted, model.StatusScored, model.StatusBuyerConfirmed:
		default:
			return cerrors.NewBusinessError("invoice cannot be rejected in status " + inv.Status)
		}
		if adv, _ := s.repo.AdvanceByInvoice(ctx, tx, id, false); adv != nil {
			return cerrors.NewBusinessError("advance already funded")
		}
		if offer, _ := s.repo.LatestOffer(ctx, tx, id); offer != nil && offer.Status != model.OfferExpired {
			offer.Status = model.OfferWithdrawn
			_ = s.repo.UpdateOffer(ctx, tx, offer)
		}
		parent, _ := s.repo.LatestDecision(ctx, tx, id)
		human := &model.Decision{InvoiceID: inv.ID, TenantID: inv.TenantID, PolicyID: policy.ID, Outcome: decision.Decline,
			Inputs: map[string]any{"reason": reason}, Detail: map[string]any{}, Reasons: []model.DecisionReason{{Code: "HUMAN_REVIEW", Detail: reason}},
			ActorType: decision.ActorHuman, ActorID: actorFrom(ctx), DecidedAt: s.now().UTC()}
		if parent != nil {
			human.ParentID, human.PolicyVersion, human.PolicyHash = &parent.ID, parent.PolicyVersion, parent.PolicyHash
		}
		if err := s.repo.CreateDecision(ctx, tx, human); err != nil {
			return err
		}
		inv.DecisionOutcome = decision.Decline
		return s.transition(ctx, tx, inv, model.StatusDeclined, model.ReasonRejectedByReviewer, model.EvReviewRejected, map[string]any{"note": reason})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Fund triggers the adapter for an ACCEPTED invoice (normally automatic).
func (s *Service) Fund(ctx context.Context, id uuid.UUID) (*model.InvoiceDetail, error) {
	inv, err := s.repo.GetInvoice(ctx, nil, auth.TenantIDOrDefault(ctx), id)
	if err != nil {
		return nil, err
	}
	if inv.Status != model.StatusAccepted {
		return nil, cerrors.NewBusinessError("invoice is not ACCEPTED (status " + inv.Status + ")")
	}
	if inv.StatusReason == model.ReasonAwaitingSecondApproval && inv.ApprovedBy == "" {
		return nil, cerrors.NewBusinessError("disbursement requires MANAGER approval (POST /approve)")
	}
	fctx, cancel := context.WithTimeout(ctx, s.cfg.FundTimeout)
	defer cancel()
	if err := s.stepFund(fctx, inv); err != nil {
		s.recordFailure(ctx, inv, "fund", err)
		return nil, cerrors.NewBusinessError("funding failed: " + err.Error())
	}
	return s.Get(ctx, id)
}

// RecordPayment applies a buyer payment (API or payment.completed event).
func (s *Service) RecordPayment(ctx context.Context, id uuid.UUID, req model.PaymentRequest, source string) (*model.InvoiceDetail, error) {
	if !req.Amount.IsPositive() {
		return nil, cerrors.BadRequest("amount must be positive")
	}
	tenantID := auth.TenantIDOrDefault(ctx)
	inv, err := s.repo.GetInvoice(ctx, nil, "", id)
	if err != nil {
		return nil, err
	}
	if source == "API" && inv.TenantID != tenantID {
		return nil, cerrors.NotFound("invoice not found")
	}
	ctx = auth.WithTenantID(ctx, inv.TenantID)
	switch inv.Status {
	case model.StatusFunded, model.StatusPartiallyPaid, model.StatusOverdue:
	default:
		return nil, cerrors.NewBusinessError("invoice is not funded (status " + inv.Status + ")")
	}
	adv, err := s.repo.AdvanceByInvoice(ctx, nil, id, false)
	if err != nil || adv == nil {
		return nil, cerrors.NewBusinessError("no advance on record")
	}
	fees, principal, release, covered := ApplyPayment(adv, req.Amount)
	ref := strings.TrimSpace(req.Reference)
	if ref == "" {
		ref = "PAY-" + uuid.NewString()[:8]
	}
	// Core banking first (synchronous); the DB write only happens on success.
	coreRef := ""
	if repay := fees.Add(principal); repay.IsPositive() {
		res, err := s.core.RecordRepayment(ctx, corebanking.RepaymentRequest{
			TenantID: inv.TenantID, InvoiceID: inv.ID.String(), CoreRef: adv.CoreRef, CoreCustomerID: corebanking.CoreCustomerID(adv.CoreCustomerID),
			CustomerID: inv.SellerCustomerID, Amount: repay, Currency: inv.Currency, Reference: ref, Payer: req.Payer,
			IdempotencyKey: "invadv:" + inv.ID.String() + ":" + ref,
		})
		if err != nil {
			return nil, cerrors.NewBusinessError("core banking repayment failed: " + err.Error())
		}
		coreRef = string(res.TxnRef)
	}
	releaseRef := ""
	if release.IsPositive() {
		tx, err := s.core.Credit(ctx, corebanking.CreditRequest{
			TenantID: inv.TenantID, InvoiceID: inv.ID.String(), CoreCustomerID: corebanking.CoreCustomerID(adv.CoreCustomerID), CustomerID: inv.SellerCustomerID,
			Amount: release, Currency: inv.Currency, Reference: "RES-" + ref, Description: "Invoice " + inv.InvoiceNumber + " reserve release",
			IdempotencyKey: "invres:" + inv.ID.String() + ":" + ref,
		})
		if err != nil {
			return nil, cerrors.NewBusinessError("reserve release failed: " + err.Error())
		}
		releaseRef = string(tx)
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		a, err := s.repo.AdvanceByInvoice(ctx, tx, id, true)
		if err != nil || a == nil {
			return fmt.Errorf("advance vanished")
		}
		p := &model.Payment{InvoiceID: id, AdvanceID: &a.ID, Amount: req.Amount, AppliedFees: fees, AppliedPrincipal: principal, ReleasedReserve: release,
			Reference: ref, Payer: req.Payer, Source: source, CoreRef: coreRef, CreatedBy: actorFrom(ctx)}
		if err := s.repo.CreatePayment(ctx, tx, p); err != nil {
			return err
		}
		a.FeesRepaid = a.FeesRepaid.Add(fees)
		a.Repaid = a.Repaid.Add(principal)
		a.ReserveReleased = a.ReserveReleased.Add(release)
		next := model.StatusPartiallyPaid
		if covered {
			a.Status, next = model.AdvancePaid, model.StatusPaid
			if cur.BuyerID != nil {
				onTime := 0
				if !s.now().After(a.DueDate.AddDate(0, 0, s.cfg.OverdueGraceDays)) {
					onTime = 1
				}
				_ = s.repo.BumpBuyerStats(ctx, tx, *cur.BuyerID, 0, 0, a.Principal.Neg())
				_, _ = tx.Exec(ctx, `UPDATE buyers SET on_time_rate = CASE WHEN confirmations_count > 0 THEN least(1, ((on_time_rate * greatest(confirmations_count-1,0)) + $2) / confirmations_count) ELSE on_time_rate END WHERE id=$1`, *cur.BuyerID, onTime)
			}
		} else if a.Status == model.AdvanceOverdue {
			next = model.StatusOverdue
		}
		if err := s.repo.UpdateAdvance(ctx, tx, a); err != nil {
			return err
		}
		payload := map[string]any{"paymentId": p.ID.String(), "amount": req.Amount, "reference": ref, "payer": req.Payer, "appliedFees": fees,
			"appliedPrincipal": principal, "coreTxnRef": coreRef, "outstanding": a.Outstanding(), "source": source}
		if err := s.transition(ctx, tx, cur, next, "", model.EvPaymentReceived, payload); err != nil {
			return err
		}
		if release.IsPositive() {
			_, err = s.appendEvent(ctx, tx, cur, model.EvReserveReleased, map[string]any{"amount": release, "coreTxnRef": releaseRef, "paymentId": p.ID.String(), "reserveReleasedTotal": a.ReserveReleased}, "")
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Queue lists invoices waiting at a human money gate, with evidence.
func (s *Service) Queue(ctx context.Context) ([]model.QueueItem, error) {
	invs, err := s.repo.QueueInvoices(ctx, auth.TenantIDOrDefault(ctx))
	if err != nil {
		return nil, err
	}
	items := []model.QueueItem{}
	for i := range invs {
		inv := invs[i]
		item := model.QueueItem{Invoice: &inv, Gate: "REVIEW"}
		if inv.Status == model.StatusAccepted {
			item.Gate = "SECOND_APPROVAL"
		}
		if inv.BuyerID != nil {
			item.Buyer, _ = s.repo.GetBuyer(ctx, nil, *inv.BuyerID)
		}
		item.Decision, _ = s.repo.LatestDecision(ctx, nil, inv.ID)
		item.Offer, _ = s.repo.LatestOffer(ctx, nil, inv.ID)
		items = append(items, item)
	}
	return items, nil
}

// Stats aggregates the portfolio.
func (s *Service) Stats(ctx context.Context) (*model.Stats, error) {
	return s.repo.Stats(ctx, auth.TenantIDOrDefault(ctx))
}

// Anchors lists anchors.
func (s *Service) Anchors(ctx context.Context) ([]model.Anchor, error) {
	return s.repo.ListAnchors(ctx, auth.TenantIDOrDefault(ctx), 200)
}

// Buyers lists buyers.
func (s *Service) Buyers(ctx context.Context) ([]model.Buyer, error) {
	return s.repo.ListBuyers(ctx, auth.TenantIDOrDefault(ctx))
}

// Buyer loads one buyer.
func (s *Service) Buyer(ctx context.Context, id uuid.UUID) (*model.Buyer, error) {
	b, err := s.repo.GetBuyer(ctx, nil, id)
	if err != nil {
		return nil, err
	}
	if b.TenantID != auth.TenantIDOrDefault(ctx) {
		return nil, cerrors.NotFound("buyer not found")
	}
	return b, nil
}

// PatchBuyer updates tier / limit.
func (s *Service) PatchBuyer(ctx context.Context, id uuid.UUID, p model.BuyerPatch) (*model.Buyer, error) {
	if _, err := s.Buyer(ctx, id); err != nil {
		return nil, err
	}
	if p.Tier != nil {
		t := strings.ToUpper(strings.TrimSpace(*p.Tier))
		switch t {
		case model.TierA, model.TierB, model.TierC, model.TierD, model.TierUnrated:
		default:
			return nil, cerrors.BadRequest("tier must be A|B|C|D|UNRATED")
		}
		p.Tier = &t
	}
	if p.LimitAmount != nil && p.LimitAmount.IsNegative() {
		return nil, cerrors.BadRequest("limitAmount must be >= 0")
	}
	if err := s.repo.PatchBuyer(ctx, id, p.Tier, p.LimitAmount); err != nil {
		return nil, err
	}
	return s.repo.GetBuyer(ctx, nil, id)
}

// PolicyView returns the current policy document.
func (s *Service) PolicyView(ctx context.Context) model.PolicyView {
	d := s.policy.Current(auth.TenantIDOrDefault(ctx))
	return model.PolicyView{PolicyID: policy.ID, Version: d.Version, Hash: d.Hash, YAML: d.YAML, Source: d.Source, UpdatedAt: d.UpdatedAt, UpdatedBy: d.UpdatedBy}
}

// PutPolicy validates/activates a new version and writes policy.changed.
func (s *Service) PutPolicy(ctx context.Context, yamlDoc string) (model.PolicyView, error) {
	tenantID := auth.TenantIDOrDefault(ctx)
	if strings.TrimSpace(yamlDoc) == "" {
		return model.PolicyView{}, cerrors.BadRequest("yaml is required")
	}
	prev := s.policy.Current(tenantID)
	doc, err := s.policy.Put(ctx, tenantID, actorFrom(ctx), yamlDoc)
	if err != nil {
		var be *cerrors.BusinessError
		if errors.As(err, &be) {
			return model.PolicyView{}, err
		}
		return model.PolicyView{}, cerrors.BadRequest(err.Error())
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		ev := &model.LedgerEvent{TenantID: tenantID, Type: model.EvPolicyChanged, Actor: actorFrom(ctx), OccurredAt: s.now(),
			Payload: map[string]any{"policyId": policy.ID, "fromVersion": prev.Version, "fromHash": prev.Hash, "toVersion": doc.Version, "toHash": doc.Hash}}
		return s.repo.AppendEvent(ctx, tx, ev)
	})
	if err != nil {
		return model.PolicyView{}, err
	}
	return model.PolicyView{PolicyID: policy.ID, Version: doc.Version, Hash: doc.Hash, YAML: doc.YAML, Source: doc.Source, UpdatedAt: doc.UpdatedAt, UpdatedBy: doc.UpdatedBy}, nil
}

// Cancel marks a pre-funding invoice CANCELLED.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, reason string) (*model.InvoiceDetail, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.TenantID != auth.TenantIDOrDefault(ctx) {
			return cerrors.NotFound("invoice not found")
		}
		if !pipeline.CanTransition(inv.Status, model.StatusCancelled) {
			return cerrors.NewBusinessError("invoice cannot be cancelled in status " + inv.Status)
		}
		return s.transition(ctx, tx, inv, model.StatusCancelled, "CANCELLED", model.EvInvoiceCancelled, map[string]any{"note": reason})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}
