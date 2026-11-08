package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/service"
)

// ─── fake service ────────────────────────────────────────────────────────────

type fakeSvc struct {
	invoices map[uuid.UUID]*model.Invoice
	profiles map[string]bool // sellerID → exists in account-service
	demo     bool

	lastUpload *service.UploadInput
	lastList   *model.ListFilter
	cancelled  []uuid.UUID
}

func newFake() *fakeSvc {
	return &fakeSvc{invoices: map[uuid.UUID]*model.Invoice{}, profiles: map[string]bool{}}
}

func (f *fakeSvc) add(seller, status string) *model.Invoice {
	inv := &model.Invoice{ID: uuid.New(), TenantID: "default", SellerCustomerID: seller, Status: status, FileName: "inv.pdf", MediaID: "m1"}
	f.invoices[inv.ID] = inv
	return inv
}

func (f *fakeSvc) Config() service.Config {
	c := service.DefaultConfig()
	c.DemoMode = f.demo
	return c
}
func (f *fakeSvc) ConfirmURL(token string) string { return "http://portal/confirm/" + token }

func (f *fakeSvc) Upload(ctx context.Context, in service.UploadInput, _ time.Duration) (*model.UploadResponse, error) {
	f.lastUpload = &in
	inv := &model.Invoice{ID: uuid.New(), SellerCustomerID: in.SellerCustomerID, Status: model.StatusUploaded, CreatedBy: auth.UserIDFromContext(ctx)}
	f.invoices[inv.ID] = inv
	resp := &model.UploadResponse{Invoice: inv}
	if f.demo {
		resp.ConfirmURL = f.ConfirmURL("tok")
		resp.DemoOTP = "000000"
	}
	return resp, nil
}

func (f *fakeSvc) List(_ context.Context, fl model.ListFilter) (*model.Page[model.Invoice], error) {
	f.lastList = &fl
	page := &model.Page[model.Invoice]{Items: []model.Invoice{}, Size: 20}
	for _, inv := range f.invoices {
		if fl.SellerID != "" && inv.SellerCustomerID != fl.SellerID {
			continue
		}
		if fl.Status != "" && inv.Status != fl.Status {
			continue
		}
		page.Items = append(page.Items, *inv)
	}
	page.TotalItems = int64(len(page.Items))
	return page, nil
}

func (f *fakeSvc) get(id uuid.UUID) (*model.Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return nil, cerrors.NotFound("invoice not found")
	}
	return inv, nil
}

func (f *fakeSvc) Get(_ context.Context, id uuid.UUID) (*model.InvoiceDetail, error) {
	inv, err := f.get(id)
	if err != nil {
		return nil, err
	}
	return &model.InvoiceDetail{Invoice: inv, Payments: []model.Payment{}}, nil
}
func (f *fakeSvc) Timeline(_ context.Context, id uuid.UUID) (*model.Timeline, error) {
	if _, err := f.get(id); err != nil {
		return nil, err
	}
	return &model.Timeline{InvoiceID: id, Events: []model.LedgerEvent{}}, nil
}
func (f *fakeSvc) ResendConfirmation(_ context.Context, id uuid.UUID, _ string) (string, error) {
	if _, err := f.get(id); err != nil {
		return "", err
	}
	return "tok", nil
}
func (f *fakeSvc) AcceptOffer(ctx context.Context, id uuid.UUID) (*model.InvoiceDetail, error) {
	return f.Get(ctx, id)
}
func (f *fakeSvc) Approve(ctx context.Context, id uuid.UUID, _ string) (*model.InvoiceDetail, error) {
	return f.Get(ctx, id)
}
func (f *fakeSvc) Reject(ctx context.Context, id uuid.UUID, _ string) (*model.InvoiceDetail, error) {
	return f.Get(ctx, id)
}
func (f *fakeSvc) Cancel(ctx context.Context, id uuid.UUID, _ string) (*model.InvoiceDetail, error) {
	f.cancelled = append(f.cancelled, id)
	return f.Get(ctx, id)
}
func (f *fakeSvc) Fund(ctx context.Context, id uuid.UUID) (*model.InvoiceDetail, error) {
	return f.Get(ctx, id)
}
func (f *fakeSvc) RecordPayment(ctx context.Context, id uuid.UUID, _ model.PaymentRequest, _ string) (*model.InvoiceDetail, error) {
	return f.Get(ctx, id)
}
func (f *fakeSvc) Queue(context.Context) ([]model.QueueItem, error) { return nil, nil }
func (f *fakeSvc) Stats(context.Context) (*model.Stats, error)      { return &model.Stats{}, nil }
func (f *fakeSvc) Anchors(context.Context) ([]model.Anchor, error)  { return nil, nil }
func (f *fakeSvc) Buyers(context.Context) ([]model.Buyer, error)    { return nil, nil }
func (f *fakeSvc) Buyer(context.Context, uuid.UUID) (*model.Buyer, error) {
	return &model.Buyer{}, nil
}
func (f *fakeSvc) PatchBuyer(context.Context, uuid.UUID, model.BuyerPatch) (*model.Buyer, error) {
	return &model.Buyer{}, nil
}
func (f *fakeSvc) PolicyView(context.Context) model.PolicyView { return model.PolicyView{} }
func (f *fakeSvc) PutPolicy(context.Context, string) (model.PolicyView, error) {
	return model.PolicyView{}, nil
}
func (f *fakeSvc) PublicConfirmView(context.Context, string) (*model.PublicConfirmView, error) {
	return nil, nil
}
func (f *fakeSvc) PublicConfirm(context.Context, string, model.PublicConfirmRequest, string, string) (*model.PublicConfirmView, error) {
	return nil, nil
}
func (f *fakeSvc) PublicVerify(context.Context, string) (*model.PublicVerifyView, error) {
	return nil, nil
}
func (f *fakeSvc) PublicVerifyBytes(context.Context, []byte) (*model.PublicVerifyView, error) {
	return nil, nil
}
func (f *fakeSvc) Owner(_ context.Context, id uuid.UUID) (*model.Invoice, error) { return f.get(id) }
func (f *fakeSvc) SellerProfileExists(_ context.Context, sellerID string) (bool, error) {
	return f.profiles[sellerID], nil
}
func (f *fakeSvc) SellerSummary(_ context.Context, sellerID string) (*model.SellerSummary, error) {
	return &model.SellerSummary{SellerCustomerID: sellerID, Counts: map[string]int64{"FUNDED": 1}, FundedTotal: decimal.NewFromInt(100), Outstanding: decimal.NewFromInt(40)}, nil
}
func (f *fakeSvc) Document(_ context.Context, id uuid.UUID) (string, []byte, error) {
	inv, err := f.get(id)
	if err != nil {
		return "", nil, err
	}
	return inv.FileName, []byte("%PDF-1.4 fake"), nil
}

var _ Service = (*fakeSvc)(nil)

// ─── harness ─────────────────────────────────────────────────────────────────

// testSecret is a 32-byte key, base64 encoded (what JWT_SECRET carries).
var testSecret = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32))

func newRouter(t *testing.T, f *fakeSvc) (http.Handler, *auth.JWTUtil) {
	t.Helper()
	jwtUtil, err := auth.NewJWTUtil(testSecret)
	require.NoError(t, err)
	h := New(f, 0, false, zap.NewNop())
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.NewMiddleware(jwtUtil, "svc-key", zap.NewNop()).Handler)
		h.RegisterRoutes(r)
	})
	return r, jwtUtil
}

// sellerToken mints exactly what bff-gateway's generateAuthResponse mints.
func sellerToken(t *testing.T, j *auth.JWTUtil, customerID string) string {
	t.Helper()
	tok, err := j.GenerateToken("+254700000001", []string{"MOBILE_USER"}, customerID, "default", uuid.NewString(), time.Hour)
	require.NoError(t, err)
	return tok
}

// staffToken mints an account-service style staff token (no customerId).
func staffToken(t *testing.T, j *auth.JWTUtil, roles ...string) string {
	t.Helper()
	tok, err := j.GenerateToken("officer", roles, "", "default", "", time.Hour)
	require.NoError(t, err)
	return tok
}

func do(r http.Handler, method, path, token string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	if body == nil {
		body = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func multipartBody(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "inv.pdf")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("%PDF-1.4 test"))
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	require.NoError(t, mw.Close())
	return &buf, mw.FormDataContentType()
}

// ─── token derivation ────────────────────────────────────────────────────────

func TestPrincipalFromCustomerToken(t *testing.T) {
	j, err := auth.NewJWTUtil(testSecret)
	require.NoError(t, err)
	tok := sellerToken(t, j, "MOB-1A2B3C4D")
	claims, err := j.ParseToken(tok)
	require.NoError(t, err)
	// What a customer token carries.
	assert.Equal(t, "+254700000001", claims.Username)
	assert.Equal(t, []string{"MOBILE_USER"}, claims.Roles)
	assert.Equal(t, "MOB-1A2B3C4D", claims.CustomerIDStr)
	assert.Nil(t, claims.CustomerID, "MOB- ids are not numeric")
	assert.NotEmpty(t, claims.MobileUserID)

	ctx := auth.WithRoles(context.Background(), claims.Roles)
	ctx = auth.WithCustomerIDStr(ctx, claims.CustomerIDStr)
	ctx = auth.WithMobileUserID(ctx, claims.MobileUserID)
	p := principalFrom(ctx)
	assert.True(t, p.Seller())
	assert.False(t, p.Staff)
	assert.Equal(t, "MOB-1A2B3C4D", p.SellerID)
	assert.Equal(t, claims.MobileUserID, p.MobileUserID)
}

func TestPrincipalFromStaffAndEdgeTokens(t *testing.T) {
	// Staff role wins even when a customerId is present.
	ctx := auth.WithCustomerIDStr(auth.WithRoles(context.Background(), []string{"officer"}), "MOB-X")
	p := principalFrom(ctx)
	assert.True(t, p.Staff)
	assert.False(t, p.Seller())

	// SERVICE (X-Service-Key) calls are staff.
	assert.True(t, principalFrom(auth.WithRoles(context.Background(), []string{"SERVICE", "ADMIN"})).Staff)

	// No staff role and no customerId: role-less staff (existing gates decide).
	p = principalFrom(auth.WithRoles(context.Background(), []string{"MOBILE_USER"}))
	assert.True(t, p.Staff)
	assert.False(t, p.Seller())
	assert.Empty(t, p.SellerID)
}

// ─── seller scoping ──────────────────────────────────────────────────────────

func TestSellerCanReadOwnInvoiceButNotOthers(t *testing.T) {
	f := newFake()
	mine := f.add("MOB-ME", model.StatusOffered)
	other := f.add("MOB-OTHER", model.StatusOffered)
	r, j := newRouter(t, f)
	tok := sellerToken(t, j, "MOB-ME")

	for _, suffix := range []string{"", "/timeline", "/document"} {
		rec := do(r, http.MethodGet, "/api/v1/invoices/"+mine.ID.String()+suffix, tok, nil, "")
		assert.Equal(t, http.StatusOK, rec.Code, suffix)
		rec = do(r, http.MethodGet, "/api/v1/invoices/"+other.ID.String()+suffix, tok, nil, "")
		assert.Equal(t, http.StatusNotFound, rec.Code, "foreign invoice must 404, not 403: "+suffix)
		rec = do(r, http.MethodGet, "/api/v1/invoices/"+uuid.NewString()+suffix, tok, nil, "")
		assert.Equal(t, http.StatusNotFound, rec.Code, suffix)
	}

	rec := do(r, http.MethodGet, "/api/v1/invoices/"+mine.ID.String()+"/document", tok, nil, "")
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), `inline; filename="inv.pdf"`)
	assert.Equal(t, "%PDF-1.4 fake", rec.Body.String())
}

func TestSellerActionsScopedToOwnInvoice(t *testing.T) {
	f := newFake()
	mine := f.add("MOB-ME", model.StatusOffered)
	other := f.add("MOB-OTHER", model.StatusOffered)
	r, j := newRouter(t, f)
	tok := sellerToken(t, j, "MOB-ME")

	for _, action := range []string{"/offer/accept", "/resend-confirmation", "/cancel"} {
		rec := do(r, http.MethodPost, "/api/v1/invoices/"+mine.ID.String()+action, tok, bytes.NewBufferString(`{}`), "application/json")
		assert.Equal(t, http.StatusOK, rec.Code, action)
		rec = do(r, http.MethodPost, "/api/v1/invoices/"+other.ID.String()+action, tok, bytes.NewBufferString(`{}`), "application/json")
		assert.Equal(t, http.StatusNotFound, rec.Code, action)
	}
	assert.Equal(t, []uuid.UUID{mine.ID}, f.cancelled)
}

func TestSellerCannotCancelFundedInvoice(t *testing.T) {
	f := newFake()
	funded := f.add("MOB-ME", model.StatusFunded)
	r, j := newRouter(t, f)
	rec := do(r, http.MethodPost, "/api/v1/invoices/"+funded.ID.String()+"/cancel", sellerToken(t, j, "MOB-ME"), bytes.NewBufferString(`{"reason":"x"}`), "application/json")
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, f.cancelled)
}

func TestSellerStaffOnlyRoutesForbidden(t *testing.T) {
	f := newFake()
	inv := f.add("MOB-ME", model.StatusDecided)
	r, j := newRouter(t, f)
	tok := sellerToken(t, j, "MOB-ME")

	gets := []string{"/api/v1/invoices", "/api/v1/invoices/queue", "/api/v1/invoices/stats", "/api/v1/invoices/anchors", "/api/v1/buyers", "/api/v1/invoice-policy"}
	for _, p := range gets {
		assert.Equal(t, http.StatusForbidden, do(r, http.MethodGet, p, tok, nil, "").Code, p)
	}
	posts := []string{"/approve", "/reject", "/fund", "/payments"}
	for _, a := range posts {
		rec := do(r, http.MethodPost, "/api/v1/invoices/"+inv.ID.String()+a, tok, bytes.NewBufferString(`{"amount":1}`), "application/json")
		assert.Equal(t, http.StatusForbidden, rec.Code, a)
	}
}

func TestMineAndSummary(t *testing.T) {
	f := newFake()
	f.profiles["MOB-ME"] = true
	f.add("MOB-ME", model.StatusFunded)
	f.add("MOB-ME", model.StatusOffered)
	f.add("MOB-OTHER", model.StatusFunded)
	r, j := newRouter(t, f)
	tok := sellerToken(t, j, "MOB-ME")

	rec := do(r, http.MethodGet, "/api/v1/invoices/mine?status=funded&sellerId=MOB-OTHER", tok, nil, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var page model.Page[model.Invoice]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	assert.Equal(t, int64(1), page.TotalItems)
	assert.Equal(t, "MOB-ME", f.lastList.SellerID, "sellerId query must be overridden by the token")
	assert.Equal(t, "FUNDED", f.lastList.Status)

	rec = do(r, http.MethodGet, "/api/v1/invoices/mine/summary", tok, nil, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var sum map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sum))
	assert.Equal(t, "MOB-ME", sum["sellerCustomerId"])
	assert.Contains(t, sum, "outstanding")
	assert.Contains(t, sum, "fundedTotal")
	assert.Contains(t, sum, "countsByStatus")

	// Staff may not use the seller-only routes.
	assert.Equal(t, http.StatusForbidden, do(r, http.MethodGet, "/api/v1/invoices/mine", staffToken(t, j, "OFFICER"), nil, "").Code)
}

func TestSellerProfileMissing(t *testing.T) {
	f := newFake()
	r, j := newRouter(t, f)
	tok := sellerToken(t, j, "MOB-NEW")

	rec := do(r, http.MethodGet, "/api/v1/invoices/mine/summary", tok, nil, "")
	assert.Equal(t, http.StatusConflict, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "SELLER_PROFILE_MISSING", body["code"])

	b, ct := multipartBody(t, map[string]string{"sellerCustomerId": "MOB-OTHER"})
	rec = do(r, http.MethodPost, "/api/v1/invoices", tok, b, ct)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Nil(t, f.lastUpload)
}

func TestSellerUploadUsesTokenCustomerAndDemoConfirmURL(t *testing.T) {
	f := newFake()
	f.demo = true
	f.profiles["MOB-ME"] = true
	r, j := newRouter(t, f)

	b, ct := multipartBody(t, map[string]string{"sellerCustomerId": "MOB-OTHER", "buyerEmail": "b@x.io"})
	rec := do(r, http.MethodPost, "/api/v1/invoices", sellerToken(t, j, "MOB-ME"), b, ct)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NotNil(t, f.lastUpload)
	assert.Equal(t, "MOB-ME", f.lastUpload.SellerCustomerID, "form sellerCustomerId must be ignored")
	var resp model.UploadResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "MOB-ME", resp.Invoice.CreatedBy, "createdBy is the seller, not the phone number")
	assert.NotEmpty(t, resp.ConfirmURL, "DEMO_MODE upload response carries confirmUrl for sellers too")
}

func TestStaffBypassesSellerScoping(t *testing.T) {
	f := newFake()
	inv := f.add("MOB-OTHER", model.StatusOffered)
	r, j := newRouter(t, f)
	officer := staffToken(t, j, "OFFICER")
	manager := staffToken(t, j, "MANAGER")

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/api/v1/invoices/"+inv.ID.String(), officer, nil, "").Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/api/v1/invoices/"+inv.ID.String()+"/document", officer, nil, "").Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/api/v1/invoices", officer, nil, "").Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, "/api/v1/invoices/"+inv.ID.String()+"/offer/accept", officer, nil, "").Code)
	// Staff cancel still needs MANAGER.
	assert.Equal(t, http.StatusForbidden, do(r, http.MethodPost, "/api/v1/invoices/"+inv.ID.String()+"/cancel", officer, nil, "").Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, "/api/v1/invoices/"+inv.ID.String()+"/cancel", manager, nil, "").Code)
	// Staff upload keeps the explicit form sellerCustomerId.
	b, ct := multipartBody(t, map[string]string{"sellerCustomerId": "MOB-OTHER"})
	assert.Equal(t, http.StatusCreated, do(r, http.MethodPost, "/api/v1/invoices", officer, b, ct).Code)
	assert.Equal(t, "MOB-OTHER", f.lastUpload.SellerCustomerID)
}
