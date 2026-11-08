package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/decision"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
	"github.com/athena-lms/go-services/internal/invoice/client"
	"github.com/athena-lms/go-services/internal/invoice/corebanking"
	"github.com/athena-lms/go-services/internal/invoice/fingerprint"
	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/pipeline"
	"github.com/athena-lms/go-services/internal/invoice/policy"
)

// Advance runs automatic steps for the invoice until it reaches a status
// that waits on an external actor, a step fails, or ctx ends. Safe to call
// concurrently: a second caller for the same invoice returns immediately.
func (s *Service) Advance(ctx context.Context, id uuid.UUID) {
	if _, busy := s.running.LoadOrStore(id, struct{}{}); busy {
		return
	}
	defer s.running.Delete(id)
	for i := 0; i < 8; i++ {
		if ctx.Err() != nil {
			return
		}
		inv, err := s.repo.GetInvoice(ctx, nil, "", id)
		if err != nil {
			s.logger.Warn("pipeline: load invoice", zap.String("id", id.String()), zap.Error(err))
			return
		}
		step := pipeline.StepFor(inv.Status)
		if step == "" {
			return
		}
		if inv.Status == model.StatusAccepted && inv.StatusReason == model.ReasonAwaitingSecondApproval && inv.ApprovedBy == "" {
			return // money gate: waits for a MANAGER
		}
		if err := s.runStep(ctx, inv, step); err != nil {
			s.recordFailure(ctx, inv, step, err)
			return
		}
	}
}

// AdvanceAsync runs Advance in the background with the tenant/user context preserved.
func (s *Service) AdvanceAsync(parent context.Context, id uuid.UUID) {
	ctx := auth.WithUserID(auth.WithTenantID(context.Background(), auth.TenantIDOrDefault(parent)), actorFrom(parent))
	go func() {
		c, cancel := context.WithTimeout(ctx, s.cfg.StepTimeout*4+s.cfg.FundTimeout)
		defer cancel()
		s.Advance(c, id)
	}()
}

func (s *Service) runStep(ctx context.Context, inv *model.Invoice, step string) error {
	timeout := s.cfg.StepTimeout
	if step == "fund" {
		timeout = s.cfg.FundTimeout
	}
	ctx, cancel := context.WithTimeout(auth.WithTenantID(ctx, inv.TenantID), timeout)
	defer cancel()
	s.logger.Info("pipeline step", zap.String("invoiceId", inv.ID.String()), zap.String("step", step), zap.String("status", inv.Status))
	switch step {
	case "extract":
		return s.stepExtract(ctx, inv)
	case "notify_buyer":
		return s.stepNotifyBuyer(ctx, inv)
	case "score":
		return s.stepScore(ctx, inv)
	case "decide":
		return s.stepDecide(ctx, inv)
	case "fund":
		return s.stepFund(ctx, inv)
	}
	return fmt.Errorf("unknown step %s", step)
}

// recordFailure stores the error and schedules a retry with exponential backoff.
func (s *Service) recordFailure(ctx context.Context, inv *model.Invoice, step string, err error) {
	s.logger.Warn("pipeline step failed", zap.String("invoiceId", inv.ID.String()), zap.String("step", step), zap.Error(err))
	attempts := inv.PipelineAttempts + 1
	backoff := s.cfg.RetryInterval * time.Duration(math.Min(math.Pow(2, float64(attempts-1)), 32))
	next := s.now().Add(backoff)
	msg := fmt.Sprintf("%s: %v", step, err)
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	_, _ = s.repo.Pool().Exec(context.Background(),
		`UPDATE invoices SET pipeline_attempts=$2, pipeline_error=$3, pipeline_next_at=$4, updated_at=now() WHERE id=$1 AND status=$5`,
		inv.ID, attempts, msg, next, inv.Status)
}

// RetryLoop periodically re-runs stuck/failed automatic steps.
func (s *Service) RetryLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.RetryInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			due, err := s.repo.PipelineDue(ctx, pipeline.AutoStatuses(), s.cfg.RetryInterval, 50)
			if err != nil {
				s.logger.Warn("pipeline retry scan failed", zap.Error(err))
				continue
			}
			for _, inv := range due {
				if inv.PipelineAttempts >= s.cfg.MaxAttempts {
					continue
				}
				c := auth.WithTenantID(ctx, inv.TenantID)
				s.Advance(c, inv.ID)
			}
		}
	}
}

// ─── Step 2: extract ─────────────────────────────────────────────────────────

var requiredFields = []string{"invoiceNumber", "dueDate", "total", "buyerName"}

func (s *Service) stepExtract(ctx context.Context, inv *model.Invoice) error {
	data, _ := s.fileBytes(ctx, inv)
	var res *client.ExtractResult
	var extractErr error
	if len(data) > 0 {
		res, extractErr = s.clients.Extract.Extract(ctx, inv.FileName, data)
	} else {
		extractErr = errors.New("source document unavailable")
	}
	now := s.now().UTC()
	inv.OCR = model.OCRMeta{Confidence: map[string]float64{}, ExtractedAt: &now}
	needs := map[string]bool{}
	if extractErr != nil {
		// Never block the demo: proceed with empty fields flagged for the seller.
		s.logger.Warn("extraction unavailable; continuing with empty fields", zap.String("invoiceId", inv.ID.String()), zap.Error(extractErr))
		inv.OCR.Error = extractErr.Error()
		inv.OCR.Engine = "none"
		for _, f := range requiredFields {
			needs[f] = true
		}
	} else {
		inv.OCR.Engine = res.Engine
		inv.OCR.RawWordCount = res.WordCount
		s.applyFields(inv, res, needs)
	}
	// Upload-time overrides win over OCR.
	if inv.Buyer.Email == "" {
		needs["buyerEmail"] = true
	}
	if inv.Total == nil && inv.RequestedAmount != nil {
		t := *inv.RequestedAmount
		inv.Total = &t
	}
	if inv.Currency == "" {
		inv.Currency = s.cfg.Currency
	}
	inv.NeedsConfirm = inv.NeedsConfirm[:0]
	for _, f := range []string{"invoiceNumber", "issueDate", "dueDate", "currency", "subtotal", "vat", "total", "sellerName", "sellerTaxId", "sellerEmail", "buyerName", "buyerTaxId", "buyerEmail", "buyerPhone"} {
		if needs[f] {
			inv.NeedsConfirm = append(inv.NeedsConfirm, f)
		}
	}
	if inv.Total != nil {
		inv.FieldsHash = fingerprint.Fields(inv.Seller.TaxID, inv.InvoiceNumber, inv.Buyer.TaxID, *inv.Total, inv.Currency)
	}

	return s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, inv.ID)
		if err != nil {
			return err
		}
		if cur.Status != model.StatusUploaded {
			return nil // already advanced elsewhere
		}
		// Second duplicate check now that we know the invoice number.
		if dup, err := s.repo.FindDuplicate(ctx, inv.TenantID, "", inv.SellerCustomerID, inv.InvoiceNumber, inv.Buyer.TaxID); err == nil && dup != nil && dup.ID != inv.ID {
			inv.Status = cur.Status
			if err := s.transition(ctx, tx, inv, model.StatusDeclined, model.ReasonDuplicate, model.EvInvoiceDuplicateRejected,
				map[string]any{"duplicateOf": dup.ID.String(), "matchedOn": "seller+invoiceNumber+buyerTaxId"}); err != nil {
				return err
			}
			s.files.Delete(inv.ID)
			return nil
		}
		if inv.Buyer.Name != "" || inv.Buyer.TaxID != "" || inv.Buyer.Email != "" {
			b, err := s.repo.UpsertBuyer(ctx, tx, inv.TenantID, inv.Buyer)
			if err != nil {
				return fmt.Errorf("upsert buyer: %w", err)
			}
			inv.BuyerID = &b.ID
			if inv.Buyer.Name == "" {
				inv.Buyer.Name = b.Name
			}
		}
		inv.Status = cur.Status
		payload := map[string]any{
			"engine": inv.OCR.Engine, "wordCount": inv.OCR.RawWordCount, "fieldsHash": inv.FieldsHash,
			"needsSellerConfirmation": inv.NeedsConfirm, "invoiceNumber": inv.InvoiceNumber, "total": inv.Total, "dueDate": inv.DueDate,
		}
		if inv.OCR.Error != "" {
			payload["extractionError"] = inv.OCR.Error
		}
		if err := s.transition(ctx, tx, inv, model.StatusExtracted, "", model.EvInvoiceExtracted, payload); err != nil {
			return err
		}
		s.files.Delete(inv.ID)
		return nil
	})
}

// fileBytes returns the uploaded bytes (memory cache, else media-service).
func (s *Service) fileBytes(ctx context.Context, inv *model.Invoice) ([]byte, error) {
	if v, ok := s.files.Load(inv.ID); ok {
		return v.([]byte), nil
	}
	if inv.MediaID == "" {
		return nil, errors.New("no media id")
	}
	return s.clients.Media.Download(ctx, inv.MediaID)
}

func (s *Service) applyFields(inv *model.Invoice, res *client.ExtractResult, needs map[string]bool) {
	get := func(name string) (client.ExtractField, bool) {
		f, ok := res.Fields[name]
		if ok {
			inv.OCR.Confidence[name] = f.Confidence
			if f.Confidence < s.cfg.ConfidenceFloor {
				needs[name] = true
			}
		}
		return f, ok && f.Str() != ""
	}
	str := func(name string) string {
		f, ok := get(name)
		if !ok {
			return ""
		}
		return f.Str()
	}
	dec := func(name string) *decimal.Decimal {
		f, ok := get(name)
		if !ok {
			return nil
		}
		d, ok := f.Decimal()
		if !ok {
			needs[name] = true
			return nil
		}
		return &d
	}
	date := func(name string) *time.Time {
		v := str(name)
		if v == "" {
			return nil
		}
		for _, layout := range []string{"2006-01-02", "02/01/2006", "2006/01/02", "02-01-2006", "2 Jan 2006", "Jan 2, 2006", "January 2, 2006", "02.01.2006"} {
			if t, err := time.Parse(layout, v); err == nil {
				t = t.UTC()
				return &t
			}
		}
		needs[name] = true
		return nil
	}
	if inv.InvoiceNumber == "" {
		inv.InvoiceNumber = str("invoiceNumber")
	}
	inv.IssueDate = date("issueDate")
	inv.DueDate = date("dueDate")
	if c := strings.ToUpper(str("currency")); len(c) == 3 {
		inv.Currency = c
	}
	inv.Subtotal, inv.VAT, inv.Total = dec("subtotal"), dec("vat"), dec("total")
	if inv.Total == nil && inv.Subtotal != nil {
		t := *inv.Subtotal
		if inv.VAT != nil {
			t = t.Add(*inv.VAT)
		}
		inv.Total = &t
	}
	if inv.Seller.Name == "" {
		inv.Seller.Name = str("sellerName")
	}
	inv.Seller.TaxID = fingerprint.NormalizeID(str("sellerTaxId"))
	if inv.Seller.Email == "" {
		inv.Seller.Email = str("sellerEmail")
	}
	inv.Buyer.Name = str("buyerName")
	inv.Buyer.TaxID = fingerprint.NormalizeID(str("buyerTaxId"))
	if e := str("buyerEmail"); inv.Buyer.Email == "" && e != "" {
		inv.Buyer.Email = e
	}
	if p := str("buyerPhone"); inv.Buyer.Phone == "" {
		inv.Buyer.Phone = p
	}
	for _, f := range requiredFields {
		switch f {
		case "invoiceNumber":
			if inv.InvoiceNumber == "" {
				needs[f] = true
			}
		case "dueDate":
			if inv.DueDate == nil {
				needs[f] = true
			}
		case "total":
			if inv.Total == nil {
				needs[f] = true
			}
		case "buyerName":
			if inv.Buyer.Name == "" {
				needs[f] = true
			}
		}
	}
	inv.LineItems = inv.LineItems[:0]
	for _, li := range res.LineItems {
		inv.LineItems = append(inv.LineItems, model.LineItem{
			Description: li.Description, Qty: decimal.NewFromFloat(li.Qty), UnitPrice: decimal.NewFromFloat(li.UnitPrice),
			Amount: decimal.NewFromFloat(li.Amount), Confidence: li.Confidence,
		})
	}
}

// ─── Step 3: buyer confirmation ──────────────────────────────────────────────

const demoOTP = "000000"

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomOTP() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return demoOTP
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// ConfirmURL builds the public link for a plaintext token.
func (s *Service) ConfirmURL(token string) string {
	return strings.TrimRight(s.cfg.PublicPortalURL, "/") + "/confirm/" + token
}

func (s *Service) stepNotifyBuyer(ctx context.Context, inv *model.Invoice) error {
	_, err := s.issueConfirmation(ctx, inv, true)
	return err
}

// issueConfirmation creates a token+OTP, emails the buyer and (when
// transition is true) moves EXTRACTED → AWAITING_BUYER. Returns the token.
func (s *Service) issueConfirmation(ctx context.Context, inv *model.Invoice, transition bool) (string, error) {
	token := randomToken()
	otp := randomOTP()
	if s.cfg.DemoMode {
		otp = demoOTP
	}
	now := s.now().UTC()
	otpExp := now.Add(s.cfg.ConfirmTTL)
	conf := &model.BuyerConfirmation{
		InvoiceID: inv.ID, TokenHash: sha(token), ExpiresAt: now.Add(s.cfg.ConfirmTTL), SentTo: inv.Buyer.Email,
		OTPHash: sha(otp), OTPExpiresAt: &otpExp,
	}
	if s.cfg.DemoMode {
		conf.DemoToken = token
	}
	link := s.ConfirmURL(token)
	delivered := false
	var deliveryErr string
	if inv.Buyer.Email != "" {
		subject := fmt.Sprintf("Please confirm invoice %s from %s", inv.InvoiceNumber, inv.Seller.Name)
		body := s.confirmEmailBody(inv, link, otp)
		if err := s.clients.Notify.Send(ctx, "EMAIL", inv.Buyer.Email, subject, body, map[string]any{"template": "invoice.buyer.confirm", "invoiceId": inv.ID.String(), "confirmUrl": link}); err != nil {
			deliveryErr = err.Error()
			s.logger.Warn("buyer email not sent", zap.String("invoiceId", inv.ID.String()), zap.Error(err))
		} else {
			delivered = true
		}
	}
	if inv.Buyer.Phone != "" && !s.cfg.DemoMode {
		if err := s.clients.Notify.Send(ctx, "SMS", inv.Buyer.Phone, "Invoice confirmation code", fmt.Sprintf("Your Nemo confirmation code for invoice %s is %s", inv.InvoiceNumber, otp), nil); err != nil {
			s.logger.Debug("buyer SMS not sent", zap.Error(err))
		}
	}
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, inv.ID)
		if err != nil {
			return err
		}
		if transition && cur.Status != model.StatusExtracted {
			return nil
		}
		if err := s.repo.CreateConfirmation(ctx, tx, conf); err != nil {
			return err
		}
		payload := map[string]any{"sentTo": inv.Buyer.Email, "delivered": delivered, "expiresAt": conf.ExpiresAt, "confirmationId": conf.ID.String()}
		if deliveryErr != "" {
			payload["deliveryError"] = deliveryErr
		}
		if transition {
			return s.transition(ctx, tx, cur, model.StatusAwaitingBuyer, "", model.EvBuyerNotified, payload)
		}
		payload["resend"] = true
		_, err = s.appendEvent(ctx, tx, cur, model.EvBuyerNotified, payload, "")
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) confirmEmailBody(inv *model.Invoice, link, otp string) string {
	total := ""
	if inv.Total != nil {
		total = inv.Currency + " " + inv.Total.StringFixed(2)
	}
	due := ""
	if inv.DueDate != nil {
		due = inv.DueDate.Format("2 Jan 2006")
	}
	return fmt.Sprintf(`Hello %s,

%s has asked Nemo to finance invoice %s (%s, due %s) issued to you.
Please confirm that this invoice is genuine and unpaid, dispute it, or tell us it is already paid:

%s

Your one-time code: %s (valid 5 days)

If you did not expect this, you can ignore this email or report it via the link.
`, inv.Buyer.Name, inv.Seller.Name, inv.InvoiceNumber, total, due, link, otp)
}

// ─── Step 4: score ───────────────────────────────────────────────────────────

func (s *Service) stepScore(ctx context.Context, inv *model.Invoice) error {
	var sc *client.Score
	scoreErr := ""
	latest, err := s.clients.Scoring.Latest(ctx, inv.SellerCustomerID)
	if err != nil {
		scoreErr = err.Error()
		s.logger.Warn("ai-scoring unavailable", zap.Error(err))
	} else {
		sc = latest
	}
	stale := sc == nil || sc.ScoredAt == nil || s.now().Sub(*sc.ScoredAt) > s.cfg.ScoreMaxAge
	if stale {
		// Ask for a fresh score (async upstream); use what we have meanwhile.
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := s.clients.Scoring.Request(rctx, inv.SellerCustomerID, inv.ID.String()); err != nil {
			s.logger.Debug("scoring request not accepted", zap.Error(err))
		}
		cancel()
	}
	band := s.cfg.SellerBandFallback
	available := false
	var scoredAt *time.Time
	provider := ""
	var finalScore *decimal.Decimal
	if sc != nil && sc.Band != "" && sc.Trusted() {
		band, available, scoredAt, provider = sc.Band, true, sc.ScoredAt, sc.Provider
		fs := sc.FinalScore
		finalScore = &fs
	} else if sc != nil {
		provider = sc.Provider
	}
	count, avg, _ := s.repo.SellerHistory(ctx, inv.TenantID, inv.SellerCustomerID, inv.ID)
	days := DaysToDue(inv, s.now())
	score := InvoiceScore(inv, days, avg, count)

	return s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, inv.ID)
		if err != nil {
			return err
		}
		if cur.Status != model.StatusBuyerConfirmed {
			return nil
		}
		cur.SellerBand = band
		cur.InvoiceScore = &score
		buyerTier := model.TierC
		if cur.BuyerID != nil {
			if b, err := s.repo.GetBuyer(ctx, tx, *cur.BuyerID); err == nil {
				buyerTier = b.EffectiveTier()
			}
		}
		payload := map[string]any{
			"sellerBand": band, "sellerScoreAvailable": available, "sellerScoreProvider": provider, "sellerScoredAt": scoredAt,
			"sellerFinalScore": finalScore, "buyerTier": buyerTier, "invoiceScore": score, "daysToDue": days,
			"sellerInvoices6m": count, "sellerAvgTotal6m": avg,
		}
		if scoreErr != "" {
			payload["scoringError"] = scoreErr
		}
		if !available {
			payload["sellerBandSource"] = "fallback"
		}
		return s.transition(ctx, tx, cur, model.StatusScored, "", model.EvInvoiceScored, payload)
	})
}

// ─── Steps 5+6: fraud + decide (+ offer) ─────────────────────────────────────

func (s *Service) stepDecide(ctx context.Context, inv *model.Invoice) error {
	amount := inv.FinanceableAmount()
	var buyer *model.Buyer
	if inv.BuyerID != nil {
		buyer, _ = s.repo.GetBuyer(ctx, nil, *inv.BuyerID)
	}
	// Remote fraud evaluation (bounded; unavailability is recorded, not fatal).
	fin := FraudInputs{SellerEmail: inv.Seller.Email, BuyerEmail: inv.Buyer.Email, Amount: amount}
	if buyer != nil {
		fin.BuyerCreatedAt, fin.BuyerTier, fin.BuyerDisputes = buyer.CreatedAt, buyer.EffectiveTier(), buyer.DisputesCount
	}
	fin.SellerCount6m, fin.SellerAvg6m, _ = s.repo.SellerHistory(ctx, inv.TenantID, inv.SellerCustomerID, inv.ID)
	fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	fr, ferr := s.clients.Fraud.Evaluate(fctx, inv.SellerCustomerID, amount, inv.ID.String())
	cancel()
	if ferr != nil {
		s.logger.Warn("fraud-detection unavailable", zap.Error(ferr))
	} else {
		fin.RemoteAvailable, fin.RemoteRiskScore, fin.RemoteRules = true, fr.RiskScore, fr.TriggeredRules
	}
	flags := LocalFraudFlags(fin, s.cfg, s.now())
	if flags == nil {
		flags = []string{}
	}

	// Concentration: buyer share of the funded book, only once the book is meaningful.
	total, buyerExp, active, _ := s.repo.Exposure(ctx, inv.TenantID, inv.BuyerID)
	conc := 0.0
	if active >= s.cfg.ConcentrationMinActive && total.Add(amount).IsPositive() {
		c, _ := buyerExp.Add(amount).Div(total.Add(amount)).Mul(decimal.NewFromInt(100)).Float64()
		conc = c
	}
	buyerTier := model.TierC
	if buyer != nil {
		buyerTier = buyer.EffectiveTier()
	}
	band := inv.SellerBand
	if band == "" {
		band = s.cfg.SellerBandFallback
	}
	score := 0
	if inv.InvoiceScore != nil {
		score = *inv.InvoiceScore
	}
	in := policy.Inputs{
		SellerBand: band, BuyerTier: buyerTier, InvoiceScore: score, DaysToDue: DaysToDue(inv, s.now()), Amount: amount,
		FraudFlags: flags, BuyerConfirmed: true, ConcentrationPct: conc, ScoreAvailable: inv.SellerBand != "",
	}
	req, out, err := s.policy.Evaluate(ctx, inv.TenantID, inv.ID.String(), inv.SellerCustomerID, in)
	if err != nil {
		return fmt.Errorf("policy evaluation: %w", err)
	}
	decidedAt := s.now().UTC()

	return s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, inv.ID)
		if err != nil {
			return err
		}
		if cur.Status != model.StatusScored {
			return nil
		}
		cur.FraudFlags = flags
		if err := s.repo.UpdateInvoice(ctx, tx, cur); err != nil {
			return err
		}
		fpayload := map[string]any{"flags": flags, "remoteAvailable": fin.RemoteAvailable, "remoteRiskScore": fin.RemoteRiskScore, "remoteRules": fin.RemoteRules}
		if ferr != nil {
			fpayload["remoteError"] = ferr.Error()
		}
		if _, err := s.appendEvent(ctx, tx, cur, model.EvInvoiceFraudChecked, fpayload, ""); err != nil {
			return err
		}
		// Persist the decision (+ decision.recorded through the outbox).
		dec := &model.Decision{
			InvoiceID: cur.ID, TenantID: cur.TenantID, PolicyID: out.Policy.ID, PolicyVersion: out.Policy.Version, PolicyHash: out.Policy.Hash,
			Inputs: req.Inputs, Outcome: out.Decision, Detail: out.Detail, ActorType: decision.ActorSystem, ActorID: Source, LatencyMS: out.LatencyMS, DecidedAt: decidedAt,
		}
		for _, r := range out.Reasons {
			dec.Reasons = append(dec.Reasons, model.DecisionReason{Code: r.Code, RuleID: r.RuleID, Detail: r.Detail})
		}
		if dec.Reasons == nil {
			dec.Reasons = []model.DecisionReason{}
		}
		if dec.Detail == nil {
			dec.Detail = map[string]any{}
		}
		if err := s.repo.CreateDecision(ctx, tx, dec); err != nil {
			return err
		}
		rec := decision.NewRecord(req, out, decidedAt)
		if de, err := event.NewDomainEvent(event.DecisionRecorded, Source, cur.TenantID, cur.ID.String(), rec); err == nil {
			if err := outbox.Write(ctx, tx, de, cur.ID.String()); err != nil {
				return err
			}
		}
		cur.DecisionOutcome = out.Decision
		next, reason := pipeline.StatusForDecision(out.Decision)
		dpayload := map[string]any{"outcome": out.Decision, "reasons": dec.Reasons, "detail": out.Detail, "policy": out.Policy, "decisionId": dec.ID.String()}
		if err := s.transition(ctx, tx, cur, model.StatusDecided, reason, model.EvInvoiceDecided, dpayload); err != nil {
			return err
		}
		switch next {
		case model.StatusOffered:
			terms, ok := policy.TermsFrom(out.Detail)
			if !ok {
				return fmt.Errorf("APPROVE outcome without advance terms (policy %s v%d)", out.Policy.ID, out.Policy.Version)
			}
			return s.createOffer(ctx, tx, cur, terms, model.EvOfferCreated)
		case model.StatusDeclined:
			code := "POLICY_DECLINE"
			if len(dec.Reasons) > 0 {
				code = dec.Reasons[0].Code
			}
			return s.transition(ctx, tx, cur, model.StatusDeclined, code, model.EvInvoiceDeclined, map[string]any{"decisionId": dec.ID.String()})
		}
		return nil // REFER: parked in DECIDED for the review queue
	})
}

// createOffer computes and stores the offer and moves the invoice to OFFERED.
func (s *Service) createOffer(ctx context.Context, tx pgx.Tx, inv *model.Invoice, terms policy.Terms, evType string) error {
	offer := ComputeOffer(inv, terms, s.cfg, s.now())
	if !offer.AdvanceAmount.IsPositive() {
		return s.transition(ctx, tx, inv, model.StatusDeclined, "ZERO_ADVANCE", model.EvInvoiceDeclined, map[string]any{"reason": "financeable amount is zero"})
	}
	if err := s.repo.CreateOffer(ctx, tx, offer); err != nil {
		return err
	}
	return s.transition(ctx, tx, inv, model.StatusOffered, "", evType, map[string]any{
		"offerId": offer.ID.String(), "advanceRate": offer.AdvanceRate, "advanceAmount": offer.AdvanceAmount, "feePerDay": offer.DiscountFeePerDay,
		"tenorDays": offer.TenorDays, "feeAmount": offer.FeeAmount, "processingFee": offer.ProcessingFee, "netDisbursement": offer.NetDisbursement,
		"reserveAmount": offer.ReserveAmount, "expiresAt": offer.ExpiresAt,
	})
}

// ─── Step 8: fund ────────────────────────────────────────────────────────────

func (s *Service) stepFund(ctx context.Context, inv *model.Invoice) error {
	offer, err := s.repo.LatestOffer(ctx, nil, inv.ID)
	if err != nil || offer == nil {
		return fmt.Errorf("no offer to fund: %v", err)
	}
	if offer.AdvanceAmount.GreaterThan(s.cfg.SecondApprovalAbove) && inv.ApprovedBy == "" {
		if inv.StatusReason != model.ReasonAwaitingSecondApproval {
			_, _ = s.repo.Pool().Exec(ctx, `UPDATE invoices SET status_reason=$2, pipeline_error=NULL, updated_at=now() WHERE id=$1 AND status='ACCEPTED'`, inv.ID, model.ReasonAwaitingSecondApproval)
		}
		return nil // wait for POST /approve by a MANAGER
	}
	if existing, _ := s.repo.AdvanceByInvoice(ctx, nil, inv.ID, false); existing != nil {
		// Disbursed earlier but the transition did not commit: finish it.
		return s.withTx(ctx, func(tx pgx.Tx) error {
			cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, inv.ID)
			if err != nil || cur.Status != model.StatusAccepted {
				return err
			}
			return s.transition(ctx, tx, cur, model.StatusFunded, "", model.EvAdvanceFunded, map[string]any{"advanceId": existing.ID.String(), "coreRef": existing.CoreRef, "resumed": true})
		})
	}
	due := s.now().AddDate(0, 0, offer.TenorDays)
	if inv.DueDate != nil {
		due = *inv.DueDate
	}
	freq := corebanking.FundingRequest{
		TenantID: inv.TenantID, InvoiceID: inv.ID.String(), OfferID: offer.ID.String(), IdempotencyKey: "fund:" + inv.ID.String(),
		Customer: corebanking.CustomerRef{TenantID: inv.TenantID, CustomerID: inv.SellerCustomerID, Name: inv.Seller.Name, TaxID: inv.Seller.TaxID, Email: inv.Seller.Email},
		Currency: inv.Currency, AdvanceAmount: offer.AdvanceAmount, NetDisbursement: offer.NetDisbursement, FeeAmount: offer.FeeAmount,
		ProcessingFee: offer.ProcessingFee, ReserveAmount: offer.ReserveAmount, TenorDays: offer.TenorDays, DueDate: due, FeePerDay: offer.DiscountFeePerDay,
		InvoiceNumber: inv.InvoiceNumber, BuyerName: inv.Buyer.Name,
	}
	coreID, err := s.core.EnsureCustomer(ctx, freq.Customer)
	if err != nil {
		return fmt.Errorf("ensure core customer: %w", err)
	}
	freq.CoreCustomerID = coreID
	res, err := s.core.Disburse(ctx, freq)
	if err != nil {
		return fmt.Errorf("core disburse: %w", err)
	}
	adv := &model.Advance{
		InvoiceID: inv.ID, OfferID: offer.ID, Provider: s.core.Name(), CoreRef: res.CoreRef, CoreCustomerID: string(res.CoreCustomerID),
		DisbursedAt: &res.DisbursedAt, DueDate: due, Principal: offer.AdvanceAmount, FeesAccrued: offer.TotalFees(),
		Repaid: decimal.Zero, FeesRepaid: decimal.Zero, ReserveAmount: offer.ReserveAmount, ReserveReleased: decimal.Zero, Status: model.AdvanceActive,
	}
	if adv.CoreCustomerID == "" {
		adv.CoreCustomerID = string(coreID)
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetInvoiceForUpdate(ctx, tx, inv.ID)
		if err != nil {
			return err
		}
		if cur.Status != model.StatusAccepted {
			s.logger.Error("advance disbursed but invoice no longer ACCEPTED; recording advance anyway", zap.String("invoiceId", inv.ID.String()), zap.String("status", cur.Status))
		}
		if err := s.repo.CreateAdvance(ctx, tx, adv); err != nil {
			return err
		}
		if cur.BuyerID != nil {
			_ = s.repo.BumpBuyerStats(ctx, tx, *cur.BuyerID, 0, 0, adv.Principal)
		}
		return s.transition(ctx, tx, cur, model.StatusFunded, "", model.EvAdvanceFunded, map[string]any{
			"advanceId": adv.ID.String(), "provider": adv.Provider, "coreRef": adv.CoreRef, "coreCustomerId": adv.CoreCustomerID,
			"principal": adv.Principal, "netDisbursement": offer.NetDisbursement, "fees": adv.FeesAccrued, "reserve": adv.ReserveAmount,
			"dueDate": adv.DueDate, "accountRef": res.AccountRef, "core": res.Raw,
		})
	})
}
