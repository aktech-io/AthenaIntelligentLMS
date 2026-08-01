package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/compliance/ekyc"
	"github.com/athena-lms/go-services/internal/compliance/model"
)

// fakeOnboardingRepo is an in-memory OnboardingRepository.
type fakeOnboardingRepo struct {
	apps      map[uuid.UUID]*model.OnboardingApplication
	kyc       map[string]*model.KycRecord // key customerID
	events    []model.ComplianceEvent
	createErr error
}

func newFakeRepo() *fakeOnboardingRepo {
	return &fakeOnboardingRepo{
		apps: map[uuid.UUID]*model.OnboardingApplication{},
		kyc:  map[string]*model.KycRecord{},
	}
}

func (f *fakeOnboardingRepo) CreateOnboarding(_ context.Context, a *model.OnboardingApplication) (*model.OnboardingApplication, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	for _, ex := range f.apps {
		if ex.TenantID == a.TenantID && ex.NationalID == a.NationalID &&
			(ex.Status == model.OnboardingReceived || ex.Status == model.OnboardingReferred) {
			return nil, errors.New(`duplicate key value violates unique constraint "uq_onboarding_open" (SQLSTATE 23505)`)
		}
	}
	cp := *a
	cp.ID = uuid.New()
	f.apps[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeOnboardingRepo) GetOnboardingByID(_ context.Context, id uuid.UUID, tenantID string) (*model.OnboardingApplication, error) {
	a, ok := f.apps[id]
	if !ok || a.TenantID != tenantID {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (f *fakeOnboardingRepo) ListOnboarding(_ context.Context, tenantID string, status *model.OnboardingStatus, _, _ int) ([]model.OnboardingApplication, int64, error) {
	var out []model.OnboardingApplication
	for _, a := range f.apps {
		if a.TenantID == tenantID && (status == nil || a.Status == *status) {
			out = append(out, *a)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeOnboardingRepo) UpdateOnboardingDecision(_ context.Context, a *model.OnboardingApplication) error {
	if _, ok := f.apps[a.ID]; !ok {
		return errors.New("not found")
	}
	cp := *a
	f.apps[a.ID] = &cp
	return nil
}

func (f *fakeOnboardingRepo) UpsertKyc(_ context.Context, rec *model.KycRecord) (*model.KycRecord, error) {
	cp := *rec
	f.kyc[rec.CustomerID] = &cp
	return &cp, nil
}

func (f *fakeOnboardingRepo) CreateEvent(_ context.Context, evt *model.ComplianceEvent) (*model.ComplianceEvent, error) {
	f.events = append(f.events, *evt)
	return evt, nil
}

// failingProvider always errors.
type failingProvider struct{}

func (failingProvider) Name() string { return "failing" }
func (failingProvider) Verify(context.Context, ekyc.Request) (ekyc.Result, error) {
	return ekyc.Result{}, errors.New("vendor timeout")
}

// cannedProvider returns a fixed result (e.g. shadow-mode PAD outcomes).
type cannedProvider struct{ res ekyc.Result }

func (cannedProvider) Name() string { return "canned" }
func (p cannedProvider) Verify(context.Context, ekyc.Request) (ekyc.Result, error) {
	return p.res, nil
}

func obStr(s string) *string { return &s }

func newSvc(repo *fakeOnboardingRepo, p ekyc.Provider) *OnboardingService {
	return NewOnboarding(repo, p, zap.NewNop())
}

func TestSubmitCleanApplicantAutoApproves(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, ekyc.Sandbox{})
	app, err := svc.Submit(context.Background(), model.SubmitOnboardingRequest{
		Phone: "+254700000001", FullName: "Amina Odhiambo", NationalID: "12345601",
		DocumentRef: obStr("media:doc1"), SelfieRef: obStr("media:selfie1"),
		CustomerID: obStr("cust-1"),
	}, "t1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if app.Status != model.OnboardingAutoApproved {
		t.Fatalf("status = %s, want AUTO_APPROVED (reasons: %v)", app.Status, app.DecisionReasons)
	}
	if app.RiskTier == nil || *app.RiskTier != model.TierLow {
		t.Errorf("tier = %v, want LOW", app.RiskTier)
	}
	rec, ok := repo.kyc["cust-1"]
	if !ok {
		t.Fatal("no KYC record materialized for auto-approved applicant")
	}
	if rec.Status != model.KycStatusPassed {
		t.Errorf("KYC status = %s, want PASSED", rec.Status)
	}
	if rec.CheckedBy == nil || *rec.CheckedBy != "ekyc:sandbox" {
		t.Errorf("KYC checkedBy = %v, want ekyc:sandbox", rec.CheckedBy)
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "onboarding.auto_approved" {
		t.Errorf("events = %+v, want one onboarding.auto_approved", repo.events)
	}
}

func TestSubmitScreeningHitsRefer(t *testing.T) {
	cases := []struct {
		name       string
		nationalID string
		wantReason string
	}{
		{"sanctions", "12345699", "SANCTIONS_HIT"},
		{"pep", "12345688", "PEP_HIT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newSvc(repo, ekyc.Sandbox{})
			app, err := svc.Submit(context.Background(), model.SubmitOnboardingRequest{
				Phone: "+254700000002", FullName: "Test Person", NationalID: tc.nationalID,
				DocumentRef: obStr("d"), SelfieRef: obStr("s"),
			}, "t1")
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if app.Status != model.OnboardingReferred {
				t.Errorf("status = %s, want REFERRED", app.Status)
			}
			if app.RiskTier == nil || *app.RiskTier != model.TierHigh {
				t.Errorf("tier = %v, want HIGH", app.RiskTier)
			}
			if app.DecisionReasons == nil || !strings.Contains(*app.DecisionReasons, tc.wantReason) {
				t.Errorf("reasons = %v, want %s", app.DecisionReasons, tc.wantReason)
			}
			if len(repo.kyc) != 0 {
				t.Error("KYC record must not exist for a referred applicant")
			}
		})
	}
}

func TestSubmitMissingEvidenceRefersMedium(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, ekyc.Sandbox{})
	app, err := svc.Submit(context.Background(), model.SubmitOnboardingRequest{
		Phone: "+254700000003", FullName: "No Selfie", NationalID: "12345602",
		DocumentRef: obStr("d"), // no selfie → liveness fail
	}, "t1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if app.Status != model.OnboardingReferred {
		t.Errorf("status = %s, want REFERRED", app.Status)
	}
	if app.RiskTier == nil || *app.RiskTier != model.TierMedium {
		t.Errorf("tier = %v, want MEDIUM", app.RiskTier)
	}
	if !strings.Contains(*app.DecisionReasons, "LIVENESS_FAILED") {
		t.Errorf("reasons = %v, want LIVENESS_FAILED", *app.DecisionReasons)
	}
}

func TestSubmitProviderErrorFailsClosedToReferral(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, failingProvider{})
	app, err := svc.Submit(context.Background(), model.SubmitOnboardingRequest{
		Phone: "+254700000004", FullName: "Vendor Down", NationalID: "12345603",
		DocumentRef: obStr("d"), SelfieRef: obStr("s"),
	}, "t1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if app.Status != model.OnboardingReferred {
		t.Errorf("status = %s, want REFERRED on provider error (fail closed)", app.Status)
	}
	if !strings.Contains(*app.DecisionReasons, "PROVIDER_ERROR") {
		t.Errorf("reasons = %v, want PROVIDER_ERROR", *app.DecisionReasons)
	}
	if len(repo.kyc) != 0 {
		t.Error("provider error must never create a PASSED KYC record")
	}
}

func TestSubmitValidatesRequiredFields(t *testing.T) {
	svc := newSvc(newFakeRepo(), ekyc.Sandbox{})
	for _, req := range []model.SubmitOnboardingRequest{
		{FullName: "X", NationalID: "1"},
		{Phone: "1", NationalID: "1"},
		{Phone: "1", FullName: "X"},
	} {
		if _, err := svc.Submit(context.Background(), req, "t1"); err == nil {
			t.Errorf("Submit(%+v) accepted an incomplete request", req)
		}
	}
}

func TestSubmitDuplicateOpenApplicationRejected(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, ekyc.Sandbox{})
	req := model.SubmitOnboardingRequest{
		Phone: "+254700000005", FullName: "Dup", NationalID: "12345604",
		DocumentRef: obStr("d"), // referred → stays open
	}
	if _, err := svc.Submit(context.Background(), req, "t1"); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	if _, err := svc.Submit(context.Background(), req, "t1"); err == nil {
		t.Fatal("second open application for same identity must be rejected")
	}
}

func TestOfficerDecisionLifecycle(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, ekyc.Sandbox{})
	app, err := svc.Submit(context.Background(), model.SubmitOnboardingRequest{
		Phone: "+254700000006", FullName: "Referred Person", NationalID: "12345699",
		DocumentRef: obStr("d"), SelfieRef: obStr("s"),
	}, "t1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Approve binds the customer and materializes KYC with the officer as checker.
	decided, err := svc.Decide(context.Background(), app.ID, true,
		model.OnboardingDecisionRequest{Reason: "screening false positive", CustomerID: obStr("cust-9")},
		"t1", "officer-1")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if decided.Status != model.OnboardingApproved {
		t.Errorf("status = %s, want APPROVED", decided.Status)
	}
	rec := repo.kyc["cust-9"]
	if rec == nil || rec.Status != model.KycStatusPassed || *rec.CheckedBy != "officer-1" {
		t.Errorf("KYC after officer approval = %+v", rec)
	}

	// A decided application cannot be decided again.
	if _, err := svc.Decide(context.Background(), app.ID, false,
		model.OnboardingDecisionRequest{Reason: "changed my mind"}, "t1", "officer-2"); err == nil {
		t.Error("second decision on a decided application must fail")
	}

	// Reason is mandatory.
	app2, _ := svc.Submit(context.Background(), model.SubmitOnboardingRequest{
		Phone: "+254700000007", FullName: "Another", NationalID: "22345699",
		DocumentRef: obStr("d"), SelfieRef: obStr("s"),
	}, "t1")
	if _, err := svc.Decide(context.Background(), app2.ID, false,
		model.OnboardingDecisionRequest{}, "t1", "officer-1"); err == nil {
		t.Error("decision without reason must fail")
	}
}

func TestTierDecisionMatrix(t *testing.T) {
	cases := []struct {
		name   string
		res    ekyc.Result
		err    error
		tier   model.RiskTier
		status model.OnboardingStatus
	}{
		{"all clear", ekyc.Result{DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.97}, nil, model.TierLow, model.OnboardingAutoApproved},
		{"match at threshold", ekyc.Result{DocumentVerified: true, LivenessPassed: true, FaceMatchScore: faceMatchThreshold}, nil, model.TierLow, model.OnboardingAutoApproved},
		{"match below threshold", ekyc.Result{DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.5}, nil, model.TierMedium, model.OnboardingReferred},
		{"sanctions trumps evidence", ekyc.Result{DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.99, SanctionsHit: true}, nil, model.TierHigh, model.OnboardingReferred},
		{"provider error", ekyc.Result{}, errors.New("down"), model.TierHigh, model.OnboardingReferred},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, status, reasons := tierDecision(tc.res, tc.err)
			if tier != tc.tier || status != tc.status {
				t.Errorf("tierDecision = (%s,%s), want (%s,%s); reasons %v", tier, status, tc.tier, tc.status, reasons)
			}
			if len(reasons) == 0 {
				t.Error("every decision must carry at least one reason")
			}
		})
	}
}

// Structured PAD observability (docs/nemo/08, audit action 2): shadow-mode
// liveness outcomes persist as queryable columns beside the LIVENESS[...]
// reason string, so calibration has a dataset from day one.
func TestSubmitPersistsStructuredLiveness(t *testing.T) {
	submit := func(t *testing.T, res ekyc.Result) *model.OnboardingApplication {
		t.Helper()
		app, err := newSvc(newFakeRepo(), cannedProvider{res: res}).Submit(context.Background(),
			model.SubmitOnboardingRequest{
				Phone: "+254700000010", FullName: "Shadow Case", NationalID: "12345610",
				DocumentRef: obStr("d"), SelfieRef: obStr("s"),
			}, "t1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		return app
	}

	t.Run("shadow score persists with mode and provider", func(t *testing.T) {
		app := submit(t, ekyc.Result{
			DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.97,
			LivenessScore: 0.42, LivenessMode: "shadow",
			LivenessProvider: "inhouse", LivenessAuditRef: "inhouse-liveness-x",
		})
		if app.LivenessScore == nil || *app.LivenessScore != 0.42 {
			t.Errorf("LivenessScore = %v, want 0.42", app.LivenessScore)
		}
		if app.LivenessMode == nil || *app.LivenessMode != "shadow" {
			t.Errorf("LivenessMode = %v, want shadow", app.LivenessMode)
		}
		if app.LivenessProvider == nil || *app.LivenessProvider != "inhouse" {
			t.Errorf("LivenessProvider = %v, want inhouse", app.LivenessProvider)
		}
		// The legacy reason string stays for backward compatibility.
		if app.DecisionReasons == nil || !strings.Contains(*app.DecisionReasons, "LIVENESS[mode=shadow score=0.42") {
			t.Errorf("reasons = %v, want LIVENESS[mode=shadow score=0.42 ...]", app.DecisionReasons)
		}
	})

	t.Run("shadow-error stores NULL score", func(t *testing.T) {
		app := submit(t, ekyc.Result{
			DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.97,
			LivenessScore: -1, LivenessMode: "shadow-error",
		})
		if app.LivenessScore != nil {
			t.Errorf("LivenessScore = %v, want nil (score -1 → NULL)", *app.LivenessScore)
		}
		if app.LivenessMode == nil || *app.LivenessMode != "shadow-error" {
			t.Errorf("LivenessMode = %v, want shadow-error", app.LivenessMode)
		}
		if app.LivenessProvider != nil {
			t.Errorf("LivenessProvider = %v, want nil (no score arrived)", *app.LivenessProvider)
		}
	})

	t.Run("no PAD stores all NULLs", func(t *testing.T) {
		app := submit(t, ekyc.Result{
			DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.97,
		})
		if app.LivenessScore != nil || app.LivenessMode != nil || app.LivenessProvider != nil {
			t.Errorf("liveness columns = (%v,%v,%v), want all nil when no PAD ran",
				app.LivenessScore, app.LivenessMode, app.LivenessProvider)
		}
	})
}

// Structured face-match evidence (docs/ekyc/05 "Referral queue" gap,
// migration 8): the score persists as a queryable column whenever the check
// ran — score 0 with incomplete doc/selfie evidence means "not run" → NULL.
func TestSubmitPersistsFaceMatchScore(t *testing.T) {
	submit := func(t *testing.T, res ekyc.Result, req model.SubmitOnboardingRequest) *model.OnboardingApplication {
		t.Helper()
		app, err := newSvc(newFakeRepo(), cannedProvider{res: res}).Submit(context.Background(), req, "t1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		return app
	}
	full := func(nationalID string) model.SubmitOnboardingRequest {
		return model.SubmitOnboardingRequest{
			Phone: "+254700000011", FullName: "Face Case", NationalID: nationalID,
			DocumentRef: obStr("d"), SelfieRef: obStr("s"),
		}
	}

	t.Run("below-threshold score persists", func(t *testing.T) {
		app := submit(t, ekyc.Result{
			DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0.79,
		}, full("12345611"))
		if app.FaceMatchScore == nil || *app.FaceMatchScore != 0.79 {
			t.Errorf("FaceMatchScore = %v, want 0.79", app.FaceMatchScore)
		}
		if app.Status != model.OnboardingReferred {
			t.Errorf("status = %s, want REFERRED (0.79 < 0.85)", app.Status)
		}
	})

	t.Run("genuine 0.0 with both refs persists as 0", func(t *testing.T) {
		app := submit(t, ekyc.Result{
			DocumentVerified: true, LivenessPassed: true, FaceMatchScore: 0,
		}, full("12345612"))
		if app.FaceMatchScore == nil || *app.FaceMatchScore != 0 {
			t.Errorf("FaceMatchScore = %v, want 0 (check ran, no match)", app.FaceMatchScore)
		}
	})

	t.Run("no selfie evidence stores NULL", func(t *testing.T) {
		app := submit(t, ekyc.Result{DocumentVerified: true},
			model.SubmitOnboardingRequest{
				Phone: "+254700000012", FullName: "No Selfie", NationalID: "12345613",
				DocumentRef: obStr("d"), // no selfie → face match never ran
			})
		if app.FaceMatchScore != nil {
			t.Errorf("FaceMatchScore = %v, want nil (no face match ran)", *app.FaceMatchScore)
		}
	})

	t.Run("provider error stores NULL", func(t *testing.T) {
		app, err := newSvc(newFakeRepo(), failingProvider{}).Submit(context.Background(),
			full("12345614"), "t1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if app.FaceMatchScore != nil {
			t.Errorf("FaceMatchScore = %v, want nil on provider error", *app.FaceMatchScore)
		}
	})
}
