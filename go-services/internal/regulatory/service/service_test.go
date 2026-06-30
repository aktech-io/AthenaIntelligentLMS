package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/regulatory/model"
)

// fakeRepo is an in-memory Repository for unit tests (no DB).
type fakeRepo struct {
	profiles map[string]*model.RegulatoryProfile
	createN  int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{profiles: map[string]*model.RegulatoryProfile{}}
}

func (f *fakeRepo) GetActiveProfile(_ context.Context, tenantID string) (*model.RegulatoryProfile, error) {
	return f.profiles[tenantID], nil
}

func (f *fakeRepo) CreateProfile(_ context.Context, p *model.RegulatoryProfile) (*model.RegulatoryProfile, error) {
	f.createN++
	cp := *p
	cp.ID = "id-" + p.TenantID
	f.profiles[p.TenantID] = &cp
	return &cp, nil
}

func (f *fakeRepo) UpdateProfile(_ context.Context, p *model.RegulatoryProfile) (*model.RegulatoryProfile, error) {
	cp := *p
	f.profiles[p.TenantID] = &cp
	return &cp, nil
}

func newSvc(r Repository) *Service { return New(r, nil, zap.NewNop()) }

func TestGetOrCreateSeedsDCPDefaultAndIsIdempotent(t *testing.T) {
	f := newFakeRepo()
	s := newSvc(f)

	p, err := s.GetOrCreateForTenant(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if p.LicenseType != model.LicenseDCP {
		t.Fatalf("want DCP default, got %s", p.LicenseType)
	}
	if p.Country != "KE" || p.ReportingCurrency != "KES" {
		t.Fatalf("bad geo defaults: %+v", p)
	}
	if p.ProvisioningTableKey != model.ProvisioningCBKPG04 {
		t.Fatalf("want CBK_PG_04 default, got %s", p.ProvisioningTableKey)
	}
	if p.CrbEnabled {
		t.Fatalf("CRB should be disabled by default (bureau-agnostic)")
	}
	if !p.HasReport(model.ReportCRBFeed) {
		t.Fatalf("DCP default report set should include the CRB feed")
	}

	if _, err := s.GetOrCreateForTenant(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	if f.createN != 1 {
		t.Fatalf("expected exactly one create across two reads, got %d", f.createN)
	}
}

func TestGetOrCreateRejectsEmptyTenant(t *testing.T) {
	_, err := newSvc(newFakeRepo()).GetOrCreateForTenant(context.Background(), "")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation for empty tenant, got %v", err)
	}
}

func TestUpdateRejectsUnknownLicense(t *testing.T) {
	bad := model.LicenseType("WHATEVER")
	_, err := newSvc(newFakeRepo()).UpdateForTenant(context.Background(), "t1", "u1",
		&model.UpdateProfileRequest{LicenseType: &bad})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdateCrbEnabledRequiresBureau(t *testing.T) {
	on := true
	_, err := newSvc(newFakeRepo()).UpdateForTenant(context.Background(), "t1", "u1",
		&model.UpdateProfileRequest{CrbEnabled: &on})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation when CRB enabled without bureau, got %v", err)
	}
}

func TestUpdateRejectsUnknownReportCode(t *testing.T) {
	rs := []model.ReportCode{"NOT_A_REPORT"}
	_, err := newSvc(newFakeRepo()).UpdateForTenant(context.Background(), "t1", "u1",
		&model.UpdateProfileRequest{ReportSet: &rs})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation for unknown report code, got %v", err)
	}
}

func TestUpdateAppliesLicenseBureauAndReportSet(t *testing.T) {
	s := newSvc(newFakeRepo())
	mfb := model.LicenseMFB
	bureau := model.BureauMetropol
	on := true
	rs := model.DefaultReportSetFor(model.LicenseMFB)

	p, err := s.UpdateForTenant(context.Background(), "t1", "u1", &model.UpdateProfileRequest{
		LicenseType: &mfb,
		CrbEnabled:  &on,
		CrbBureau:   &bureau,
		ReportSet:   &rs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.LicenseType != model.LicenseMFB {
		t.Fatalf("license not applied: %s", p.LicenseType)
	}
	if p.CrbBureau == nil || *p.CrbBureau != model.BureauMetropol {
		t.Fatalf("bureau not applied: %v", p.CrbBureau)
	}
	if p.UpdatedBy == nil || *p.UpdatedBy != "u1" {
		t.Fatalf("updatedBy not recorded: %v", p.UpdatedBy)
	}
	var foundPrudential bool
	for _, c := range p.ReportSet {
		if c == model.ReportCBKClassification {
			foundPrudential = true
		}
	}
	if !foundPrudential {
		t.Fatalf("MFB report set should include CBK prudential classification")
	}
}

func TestUpdateCrbBureauEmptyStringUnsets(t *testing.T) {
	s := newSvc(newFakeRepo())
	// First enable with a bureau.
	bureau := model.BureauMetropol
	on := true
	if _, err := s.UpdateForTenant(context.Background(), "t1", "u1",
		&model.UpdateProfileRequest{CrbEnabled: &on, CrbBureau: &bureau}); err != nil {
		t.Fatal(err)
	}
	// Now disable and clear the bureau in one call.
	off := false
	empty := model.CrbBureau("")
	p, err := s.UpdateForTenant(context.Background(), "t1", "u1",
		&model.UpdateProfileRequest{CrbEnabled: &off, CrbBureau: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if p.CrbBureau != nil {
		t.Fatalf("bureau should be unset, got %v", *p.CrbBureau)
	}
}
