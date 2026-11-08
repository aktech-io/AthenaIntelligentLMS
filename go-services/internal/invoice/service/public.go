package service

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/common/auth"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/invoice/fingerprint"
	"github.com/athena-lms/go-services/internal/invoice/ledger"
	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/repository"
)

const maxOTPAttempts = 5

var decimalZero = decimal.Zero

// loadConfirmation resolves a plaintext token to its confirmation + invoice.
func (s *Service) loadConfirmation(ctx context.Context, tx pgx.Tx, token string) (*model.BuyerConfirmation, *model.Invoice, error) {
	if len(token) < 32 || len(token) > 128 {
		return nil, nil, cerrors.NotFound("confirmation link is invalid")
	}
	var q repository.Querier
	if tx != nil {
		q = tx
	}
	conf, err := s.repo.ConfirmationByToken(ctx, q, sha(token))
	if err != nil {
		return nil, nil, cerrors.NotFound("confirmation link is invalid")
	}
	var inv *model.Invoice
	if tx != nil {
		inv, err = s.repo.GetInvoiceForUpdate(ctx, tx, conf.InvoiceID)
	} else {
		inv, err = s.repo.GetInvoice(ctx, nil, "", conf.InvoiceID)
	}
	if err != nil {
		return nil, nil, err
	}
	return conf, inv, nil
}

// PublicConfirmView is GET /public/invoices/confirm/{token}: buyer-visible
// fields only (no seller contact details, no amounts beyond the invoice itself).
func (s *Service) PublicConfirmView(ctx context.Context, token string) (*model.PublicConfirmView, error) {
	conf, inv, err := s.loadConfirmation(ctx, nil, token)
	if err != nil {
		return nil, err
	}
	fp := inv.Fingerprint
	if len(fp) > 12 {
		fp = fp[:12]
	}
	return &model.PublicConfirmView{
		InvoiceID: inv.ID, InvoiceNumber: inv.InvoiceNumber, SellerName: inv.Seller.Name, BuyerName: inv.Buyer.Name,
		IssueDate: inv.IssueDate, DueDate: inv.DueDate, Currency: inv.Currency, Total: inv.Total, LineItems: inv.LineItems,
		Status: coarseStatus(inv.Status), Action: conf.Action, ExpiresAt: conf.ExpiresAt, Expired: s.now().After(conf.ExpiresAt) && conf.Action == "",
		OTPRequired: conf.OTPHash != "", FingerprintS: fp,
	}, nil
}

// PublicConfirm is POST /public/invoices/confirm/{token}.
func (s *Service) PublicConfirm(ctx context.Context, token string, req model.PublicConfirmRequest, ip, userAgent string) (*model.PublicConfirmView, error) {
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	switch action {
	case model.ActionConfirmed, model.ActionDisputed, model.ActionAlreadyPaid:
	default:
		return nil, cerrors.BadRequest("action must be CONFIRMED|DISPUTED|ALREADY_PAID")
	}
	if action == model.ActionDisputed && strings.TrimSpace(req.DisputeReason) == "" {
		return nil, cerrors.BadRequest("disputeReason is required for DISPUTED")
	}
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		conf, inv, err := s.loadConfirmation(ctx, tx, token)
		if err != nil {
			return err
		}
		if conf.Action != "" {
			return cerrors.Conflict("this confirmation link was already used (" + conf.Action + ")")
		}
		if s.now().After(conf.ExpiresAt) {
			return cerrors.NewBusinessError("confirmation link has expired; ask the seller to resend it")
		}
		if inv.Status != model.StatusAwaitingBuyer {
			return cerrors.Conflict("invoice is no longer awaiting confirmation")
		}
		if conf.OTPHash != "" {
			if conf.OTPAttempts >= maxOTPAttempts {
				return cerrors.TooManyRequests("too many incorrect codes; ask the seller to resend the link")
			}
			otp := strings.TrimSpace(req.OTP)
			if otp == "" || subtle.ConstantTimeCompare([]byte(sha(otp)), []byte(conf.OTPHash)) != 1 || (conf.OTPExpiresAt != nil && s.now().After(*conf.OTPExpiresAt)) {
				conf.OTPAttempts++
				_ = s.repo.UpdateConfirmation(ctx, tx, conf)
				if err := tx.Commit(ctx); err != nil { // persist the attempt counter even though we fail
					return err
				}
				return cerrors.BadRequest("incorrect or expired one-time code")
			}
		}
		now := s.now().UTC()
		conf.Action, conf.ActedAt, conf.IP, conf.UserAgent = action, &now, ip, userAgent
		conf.DisputeReason = strings.TrimSpace(req.DisputeReason)
		if err := s.repo.UpdateConfirmation(ctx, tx, conf); err != nil {
			return err
		}
		ctx = auth.WithUserID(auth.WithTenantID(ctx, inv.TenantID), "buyer:"+conf.SentTo)
		payload := map[string]any{"confirmationId": conf.ID.String(), "action": action, "sentTo": conf.SentTo, "ip": ip}
		switch action {
		case model.ActionConfirmed:
			if inv.BuyerID != nil {
				_ = s.repo.BumpBuyerStats(ctx, tx, *inv.BuyerID, 1, 0, decimalZero)
			}
			return s.transition(ctx, tx, inv, model.StatusBuyerConfirmed, "", model.EvBuyerConfirmed, payload)
		case model.ActionDisputed:
			payload["disputeReason"] = conf.DisputeReason
			if inv.BuyerID != nil {
				_ = s.repo.BumpBuyerStats(ctx, tx, *inv.BuyerID, 0, 1, decimalZero)
			}
			return s.transition(ctx, tx, inv, model.StatusBuyerDisputed, model.ReasonBuyerDisputed, model.EvBuyerDisputed, payload)
		default: // ALREADY_PAID: nothing to finance
			payload["disputeReason"] = "already paid"
			if err := s.transition(ctx, tx, inv, model.StatusBuyerDisputed, model.ReasonBuyerAlreadyPaid, model.EvBuyerDisputed, payload); err != nil {
				return err
			}
			return s.transition(ctx, tx, inv, model.StatusDeclined, model.ReasonBuyerAlreadyPaid, model.EvInvoiceDeclined, nil)
		}
	})
	if err != nil {
		return nil, err
	}
	view, verr := s.PublicConfirmView(ctx, token)
	if verr != nil {
		return nil, verr
	}
	if conf, inv, err := s.loadConfirmation(ctx, nil, token); err == nil && conf.Action == model.ActionConfirmed {
		s.AdvanceAsync(auth.WithTenantID(ctx, inv.TenantID), inv.ID)
	}
	return view, nil
}

// coarseStatus hides pipeline internals from the public.
func coarseStatus(st string) string {
	switch st {
	case model.StatusUploaded, model.StatusExtracted, model.StatusAwaitingBuyer:
		return "PENDING_CONFIRMATION"
	case model.StatusBuyerConfirmed, model.StatusScored, model.StatusDecided, model.StatusOffered, model.StatusAccepted:
		return "CONFIRMED"
	case model.StatusFunded, model.StatusPartiallyPaid, model.StatusOverdue:
		return "FINANCED"
	case model.StatusPaid:
		return "SETTLED"
	case model.StatusBuyerDisputed:
		return "DISPUTED"
	default:
		return "NOT_FINANCED"
	}
}

// PublicVerify answers GET /public/invoices/verify/{fingerprint}.
func (s *Service) PublicVerify(ctx context.Context, fp string) (*model.PublicVerifyView, error) {
	fp = strings.ToLower(strings.TrimSpace(fp))
	if !fingerprint.IsValidHex64(fp) {
		return nil, cerrors.BadRequest("fingerprint must be 64 hex characters")
	}
	view := &model.PublicVerifyView{Fingerprint: fp}
	inv, err := s.repo.FindByFingerprint(ctx, fp)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return view, nil
	}
	view.Known = true
	view.Status = coarseStatus(inv.Status)
	events, err := s.repo.EventsByInvoice(ctx, inv.ID)
	if err != nil {
		return nil, err
	}
	for i := range events {
		ev := events[i]
		t := ev.OccurredAt
		switch ev.Type {
		case model.EvBuyerConfirmed:
			view.BuyerConfirmedAt = &t
		case model.EvAdvanceFunded:
			view.FundedAt = &t
		case model.EvPaymentReceived:
			if st, _ := ev.Payload["to"].(string); st == model.StatusPaid {
				view.PaidAt = &t
			}
		}
	}
	if len(events) > 0 {
		rng, err := s.repo.EventsRange(ctx, inv.TenantID, events[0].Seq, events[len(events)-1].Seq)
		if err == nil {
			ok, _ := ledger.Verify(rng, s.repo.HMACKey())
			view.ChainOK = &ok
		}
	}
	if a, _ := s.repo.AnchorForInvoice(ctx, inv.ID); a != nil {
		view.AnchoredAt = &a.AnchoredAt
		view.MerkleRoot = a.MerkleRoot
	}
	return view, nil
}

// PublicVerifyBytes computes the fingerprint of an uploaded file and verifies it.
func (s *Service) PublicVerifyBytes(ctx context.Context, data []byte) (*model.PublicVerifyView, error) {
	if len(data) == 0 {
		return nil, cerrors.BadRequest("file is required")
	}
	return s.PublicVerify(ctx, fingerprint.Bytes(data))
}
