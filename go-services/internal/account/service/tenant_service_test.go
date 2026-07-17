package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/athena-lms/go-services/internal/account/model"
	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
	commonevent "github.com/athena-lms/go-services/internal/common/event"
)

// fakeTenantRepo is an in-memory TenantRepository for unit tests (no DB).
type fakeTenantRepo struct {
	tenants    map[string]*model.Tenant
	settings   map[string]*model.TenantSettings
	users      map[string]*model.User
	hashes     map[string]string
	events     []*commonevent.DomainEvent
	brands     map[string][]byte
	provisionN int
}

func newFakeTenantRepo() *fakeTenantRepo {
	return &fakeTenantRepo{
		tenants:  map[string]*model.Tenant{},
		settings: map[string]*model.TenantSettings{},
		users:    map[string]*model.User{},
		hashes:   map[string]string{},
	}
}

func (f *fakeTenantRepo) GetTenantBrand(_ context.Context, id string) ([]byte, bool, error) {
	t, ok := f.tenants[id]
	if !ok || t == nil {
		return nil, false, nil
	}
	return f.brands[id], true, nil
}

func (f *fakeTenantRepo) SetTenantBrand(_ context.Context, id string, raw []byte) error {
	if _, ok := f.tenants[id]; !ok {
		return pgx.ErrNoRows
	}
	if f.brands == nil {
		f.brands = map[string][]byte{}
	}
	f.brands[id] = raw
	return nil
}

func (f *fakeTenantRepo) GetTenant(_ context.Context, id string) (*model.Tenant, error) {
	return f.tenants[id], nil
}

func (f *fakeTenantRepo) ListTenants(_ context.Context) ([]*model.Tenant, error) {
	out := []*model.Tenant{}
	for _, t := range f.tenants {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeTenantRepo) ProvisionTenant(_ context.Context, t *model.Tenant, s *model.TenantSettings, u *model.User, hash string, evt *commonevent.DomainEvent) error {
	f.provisionN++
	u.ID = "user-" + t.ID
	f.tenants[t.ID] = t
	f.settings[t.ID] = s
	f.users[t.ID] = u
	f.hashes[t.ID] = hash
	f.events = append(f.events, evt)
	return nil
}

func (f *fakeTenantRepo) UpdateTenantStatus(_ context.Context, id string, status model.TenantStatus, evt *commonevent.DomainEvent) error {
	f.tenants[id].Status = status
	f.events = append(f.events, evt)
	return nil
}

func newTenantSvc(f *fakeTenantRepo) *TenantService {
	return NewTenantService(f, nil, zap.NewNop())
}

func validReq() ProvisionTenantRequest {
	return ProvisionTenantRequest{
		ID:          "acme-bank",
		DisplayName: "Acme Bank",
		AdminEmail:  "admin@acme.example",
	}
}

func TestProvisionSeedsMarketDefaultsAndAdminUser(t *testing.T) {
	f := newFakeTenantRepo()
	res, err := newTenantSvc(f).Provision(context.Background(), validReq())
	if err != nil {
		t.Fatal(err)
	}

	// Tenant registered in PROVISIONING (awaits explicit activation).
	if res.Tenant.ID != "acme-bank" || res.Tenant.Status != model.TenantStatusProvisioning {
		t.Fatalf("bad tenant: %+v", res.Tenant)
	}
	// Market defaults from the active pack (KE in tests).
	if res.Tenant.MarketCode != "KE" {
		t.Fatalf("want market KE default, got %s", res.Tenant.MarketCode)
	}
	if res.Settings.Currency != "KES" || res.Settings.Timezone != "Africa/Nairobi" {
		t.Fatalf("settings not seeded from market pack: %+v", res.Settings)
	}
	if res.Settings.OrgName == nil || *res.Settings.OrgName != "Acme Bank" {
		t.Fatalf("org name not seeded: %+v", res.Settings)
	}
	// Admin user defaults.
	u := res.AdminUser
	if u.Username != "admin" || u.Role != "ADMIN" || u.Status != "ACTIVE" || u.TenantID != "acme-bank" {
		t.Fatalf("bad admin user: %+v", u)
	}
	// One-time password: returned once, stored only as a verifying bcrypt hash.
	if len(res.OneTimePassword) != otpLength {
		t.Fatalf("want %d-char one-time password, got %q", otpLength, res.OneTimePassword)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(f.hashes["acme-bank"]), []byte(res.OneTimePassword)); err != nil {
		t.Fatalf("stored hash does not verify the one-time password: %v", err)
	}
	// tenant.provisioned emitted with the tenant scoping.
	if len(f.events) != 1 || f.events[0].Type != commonevent.TenantProvisioned {
		t.Fatalf("want one tenant.provisioned event, got %+v", f.events)
	}
	if f.events[0].TenantID != "acme-bank" {
		t.Fatalf("event not tenant-scoped: %+v", f.events[0])
	}
}

func TestProvisionExplicitMarketCode(t *testing.T) {
	f := newFakeTenantRepo()
	req := validReq()
	req.MarketCode = "ke" // case-insensitive
	res, err := newTenantSvc(f).Provision(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tenant.MarketCode != "KE" {
		t.Fatalf("want normalised KE, got %s", res.Tenant.MarketCode)
	}
}

func TestProvisionNormalisesSlugCase(t *testing.T) {
	f := newFakeTenantRepo()
	req := validReq()
	req.ID = "AcmeBank"
	res, err := newTenantSvc(f).Provision(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tenant.ID != "acmebank" {
		t.Fatalf("want lowercased slug, got %s", res.Tenant.ID)
	}
}

func TestProvisionRejectsUnknownMarket(t *testing.T) {
	req := validReq()
	req.MarketCode = "ZZ"
	_, err := newTenantSvc(newFakeTenantRepo()).Provision(context.Background(), req)
	assertBusinessStatus(t, err, 400)
}

func TestProvisionValidatesInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProvisionTenantRequest)
	}{
		{"empty id", func(r *ProvisionTenantRequest) { r.ID = "" }},
		{"underscore id", func(r *ProvisionTenantRequest) { r.ID = "acme_bank" }},
		{"too short", func(r *ProvisionTenantRequest) { r.ID = "ab" }},
		{"leading hyphen", func(r *ProvisionTenantRequest) { r.ID = "-acme" }},
		{"reserved system", func(r *ProvisionTenantRequest) { r.ID = "system" }},
		{"reserved admin", func(r *ProvisionTenantRequest) { r.ID = "admin" }},
		{"no display name", func(r *ProvisionTenantRequest) { r.DisplayName = "  " }},
		{"no admin email", func(r *ProvisionTenantRequest) { r.AdminEmail = "" }},
		{"bad admin email", func(r *ProvisionTenantRequest) { r.AdminEmail = "not-an-email" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validReq()
			tc.mutate(&req)
			_, err := newTenantSvc(newFakeTenantRepo()).Provision(context.Background(), req)
			assertBusinessStatus(t, err, 400)
		})
	}
}

func TestProvisionExistingSlugIsConflict(t *testing.T) {
	f := newFakeTenantRepo()
	svc := newTenantSvc(f)
	if _, err := svc.Provision(context.Background(), validReq()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Provision(context.Background(), validReq())
	assertBusinessStatus(t, err, 409)
	if f.provisionN != 1 {
		t.Fatalf("second create must not reach the repo, provisionN=%d", f.provisionN)
	}
}

func TestActivateAndSuspendLifecycle(t *testing.T) {
	f := newFakeTenantRepo()
	svc := newTenantSvc(f)
	ctx := context.Background()
	if _, err := svc.Provision(ctx, validReq()); err != nil {
		t.Fatal(err)
	}

	// PROVISIONING -> ACTIVE
	tenant, err := svc.Activate(ctx, "acme-bank")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Status != model.TenantStatusActive {
		t.Fatalf("want ACTIVE, got %s", tenant.Status)
	}
	// Idempotent re-activate: no extra event.
	events := len(f.events)
	if _, err := svc.Activate(ctx, "acme-bank"); err != nil {
		t.Fatal(err)
	}
	if len(f.events) != events {
		t.Fatalf("idempotent activate must not emit events")
	}

	// ACTIVE -> SUSPENDED -> ACTIVE
	tenant, err = svc.Suspend(ctx, "acme-bank")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Status != model.TenantStatusSuspended {
		t.Fatalf("want SUSPENDED, got %s", tenant.Status)
	}
	if _, err := svc.Activate(ctx, "acme-bank"); err != nil {
		t.Fatal(err)
	}

	// Lifecycle events carried the right types.
	types := []string{}
	for _, e := range f.events[1:] {
		types = append(types, e.Type)
	}
	want := []string{commonevent.TenantActivated, commonevent.TenantSuspended, commonevent.TenantActivated}
	if len(types) != len(want) {
		t.Fatalf("want %v, got %v", want, types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("want %v, got %v", want, types)
		}
	}
}

func TestLifecycleOnMissingTenantIsNotFound(t *testing.T) {
	svc := newTenantSvc(newFakeTenantRepo())
	if _, err := svc.Get(context.Background(), "ghost"); err == nil {
		t.Fatal("want not-found error")
	} else if _, ok := err.(*commonerrors.NotFoundError); !ok {
		t.Fatalf("want NotFoundError, got %T", err)
	}
	if _, err := svc.Activate(context.Background(), "ghost"); err == nil {
		t.Fatal("want not-found error from activate")
	}
}

func TestOneTimePasswordsAreUnique(t *testing.T) {
	a, err := generateOneTimePassword()
	if err != nil {
		t.Fatal(err)
	}
	b, err := generateOneTimePassword()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two generated one-time passwords were identical")
	}
}

func assertBusinessStatus(t *testing.T, err error, status int) {
	t.Helper()
	be, ok := err.(*commonerrors.BusinessError)
	if !ok {
		t.Fatalf("want *BusinessError(%d), got %T: %v", status, err, err)
	}
	if be.StatusCode != status {
		t.Fatalf("want status %d, got %d (%s)", status, be.StatusCode, be.Message)
	}
}

func TestBrandPackDefaultAndRoundTrip(t *testing.T) {
	f := newFakeTenantRepo()
	svc := newTenantSvc(f)
	if _, err := svc.Provision(context.Background(), validReq()); err != nil {
		t.Fatal(err)
	}
	const fixtureTenantID = "acme-bank"

	// Unknown tenant → not found.
	if _, err := svc.GetBrand(context.Background(), "ghost"); err == nil {
		t.Error("GetBrand for unknown tenant must fail")
	}

	// No brand set → platform default.
	b, err := svc.GetBrand(context.Background(), fixtureTenantID)
	if err != nil {
		t.Fatalf("GetBrand: %v", err)
	}
	if b.AppName != "NemoWallet" || b.Colors["primary"] != "#FF6A3D" {
		t.Errorf("default brand = %+v, want NemoWallet deep-water", b)
	}

	// Set + read back.
	custom := model.BrandPack{AppName: "AcmeBank", Colors: map[string]string{"primary": "#123456"}}
	if _, err := svc.SetBrand(context.Background(), fixtureTenantID, custom); err != nil {
		t.Fatalf("SetBrand: %v", err)
	}
	got, err := svc.GetBrand(context.Background(), fixtureTenantID)
	if err != nil {
		t.Fatalf("GetBrand after set: %v", err)
	}
	if got.AppName != "AcmeBank" || got.Colors["primary"] != "#123456" {
		t.Errorf("round-trip brand = %+v", got)
	}

	// Validation: bad hex and empty name rejected.
	if _, err := svc.SetBrand(context.Background(), fixtureTenantID,
		model.BrandPack{AppName: "X", Colors: map[string]string{"primary": "red"}}); err == nil {
		t.Error("non-hex color must be rejected")
	}
	if _, err := svc.SetBrand(context.Background(), fixtureTenantID,
		model.BrandPack{Colors: map[string]string{}}); err == nil {
		t.Error("empty appName must be rejected")
	}
	// Unknown tenant on set → not found.
	if _, err := svc.SetBrand(context.Background(), "ghost", custom); err == nil {
		t.Error("SetBrand for unknown tenant must fail")
	}
}
