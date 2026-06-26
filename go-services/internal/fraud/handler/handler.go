package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/fraud/engine"
	"github.com/athena-lms/go-services/internal/fraud/model"
	"github.com/athena-lms/go-services/internal/fraud/service"
)

// Handler handles HTTP requests for fraud detection.
type Handler struct {
	svc    *service.Service
	eng    *engine.Engine
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, eng *engine.Engine, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, eng: eng, logger: logger}
}

// registerFraudRoutes registers all fraud routes on the given sub-router.
func (h *Handler) registerFraudRoutes(r chi.Router) {
	r.Get("/summary", h.GetSummary)
	r.Get("/analytics", h.GetAnalytics)

	r.Get("/alerts", h.ListAlerts)
	r.Get("/alerts/{id}", h.GetAlert)
	r.Post("/alerts/{id}/resolve", h.ResolveAlert)
	r.Put("/alerts/{id}/resolve", h.ResolveAlert)
	r.Post("/alerts/{id}/assign", h.AssignAlert)
	r.Put("/alerts/{id}/assign", h.AssignAlert)
	r.Post("/alerts/bulk/assign", h.BulkAssignAlerts)
	r.Put("/alerts/bulk/assign", h.BulkAssignAlerts)
	r.Post("/alerts/bulk/resolve", h.BulkResolveAlerts)
	r.Put("/alerts/bulk/resolve", h.BulkResolveAlerts)

	r.Get("/rules", h.ListRules)
	r.Get("/rules/{id}", h.GetRule)
	r.Put("/rules/{id}", h.UpdateRule)

	r.Get("/cases", h.ListCases)
	r.Post("/cases", h.CreateCase)
	r.Get("/cases/{id}", h.GetCase)
	r.Put("/cases/{id}", h.UpdateCase)
	r.Get("/cases/{id}/notes", h.ListCaseNotes)
	r.Post("/cases/{id}/notes", h.AddCaseNote)
	r.Get("/cases/{id}/timeline", h.GetCaseTimeline)

	r.Get("/watchlist", h.ListWatchlistEntries)
	r.Post("/watchlist", h.CreateWatchlistEntry)
	r.Get("/watchlist/{id}", h.GetWatchlistEntry)
	r.Put("/watchlist/{id}/deactivate", h.DeactivateWatchlistEntry)
	r.Delete("/watchlist/{id}", h.DeactivateWatchlistEntry)
	r.Post("/watchlist/screen", h.ScreenCustomer)

	r.Get("/events/recent", h.ListRecentEvents)

	// SAR reports — serve on both /sar-reports and /sar paths
	r.Get("/sar-reports", h.ListSarReports)
	r.Post("/sar-reports", h.CreateSarReport)
	r.Get("/sar-reports/{id}", h.GetSarReport)
	r.Put("/sar-reports/{id}", h.UpdateSarReport)
	r.Get("/sar", h.ListSarReports)
	r.Post("/sar", h.CreateSarReport)
	r.Get("/sar/{id}", h.GetSarReport)
	r.Put("/sar/{id}", h.UpdateSarReport)

	// Risk profiles — serve on both canonical and frontend paths
	r.Get("/risk-profiles/high-risk", h.ListHighRiskCustomers)
	r.Get("/risk-profiles/{customerId}", h.GetRiskProfile)
	r.Get("/high-risk-customers", h.ListHighRiskCustomers)
	r.Get("/customer/{customerId}/risk", h.GetRiskProfile)
	r.Get("/customer/{customerId}/alerts", h.ListCustomerAlerts)

	r.Get("/network/{customerId}", h.ListNetworkLinks)

	// Audit log — serve on both /audit and /audit-log
	r.Get("/audit", h.ListAuditLog)
	r.Get("/audit-log", h.ListAuditLog)
	r.Get("/audit/verify", h.VerifyAuditChain)
	r.Get("/audit-log/verify", h.VerifyAuditChain)

	// Screening — alias for watchlist screen
	r.Post("/screening/customer", h.ScreenCustomer)
	r.Post("/screening/batch", h.BatchScreen)

	r.Post("/evaluate", h.EvaluateTransaction)
	r.Get("/rules/documentation", h.GetRulesDocumentation)
}

// RegisterRoutes registers all fraud detection routes on the given router.
// Serves on both /api/v1/fraud and /api/fraud for frontend compatibility.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/fraud", h.registerFraudRoutes)
	r.Route("/api/fraud", h.registerFraudRoutes)
}

// GetSummary handles GET /api/v1/fraud/summary
func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetSummary(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get fraud summary", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get fraud summary", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetAnalytics handles GET /api/v1/fraud/analytics
func (h *Handler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetAnalytics(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get fraud analytics", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get fraud analytics", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ListAlerts handles GET /api/v1/fraud/alerts
func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var status *model.AlertStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := model.AlertStatus(s)
		status = &st
	}

	alerts, total, err := h.svc.ListAlerts(r.Context(), tenantID, status, page, size)
	if err != nil {
		h.logger.Error("Failed to list alerts", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list alerts", r.URL.Path)
		return
	}
	if alerts == nil {
		alerts = []*model.FraudAlert{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(alerts, page, size, total))
}

// GetAlert handles GET /api/v1/fraud/alerts/{id}
func (h *Handler) GetAlert(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid alert ID", r.URL.Path)
		return
	}

	alert, err := h.svc.GetAlert(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get alert", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get alert", r.URL.Path)
		return
	}
	if alert == nil {
		httputil.WriteNotFound(w, "Alert not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, alert)
}

// ResolveAlert handles POST /api/v1/fraud/alerts/{id}/resolve
func (h *Handler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid alert ID", r.URL.Path)
		return
	}

	var req model.ResolveAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	alert, err := h.svc.ResolveAlert(r.Context(), id, req, tenantID)
	if err != nil {
		h.logger.Error("Failed to resolve alert", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to resolve alert", r.URL.Path)
		return
	}
	if alert == nil {
		httputil.WriteNotFound(w, "Alert not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, alert)
}

// AssignAlert handles POST /api/v1/fraud/alerts/{id}/assign
func (h *Handler) AssignAlert(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid alert ID", r.URL.Path)
		return
	}

	var req model.AssignAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	alert, err := h.svc.AssignAlert(r.Context(), id, req)
	if err != nil {
		h.logger.Error("Failed to assign alert", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to assign alert", r.URL.Path)
		return
	}
	if alert == nil {
		httputil.WriteNotFound(w, "Alert not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, alert)
}

// ListRules handles GET /api/v1/fraud/rules
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	rules, err := h.svc.ListRules(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list rules", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list rules", r.URL.Path)
		return
	}
	if rules == nil {
		rules = []*model.FraudRule{}
	}

	httputil.WriteJSON(w, http.StatusOK, rules)
}

// GetRule handles GET /api/v1/fraud/rules/{id}
func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid rule ID", r.URL.Path)
		return
	}

	rule, err := h.svc.GetRule(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get rule", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get rule", r.URL.Path)
		return
	}
	if rule == nil {
		httputil.WriteNotFound(w, "Rule not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, rule)
}

// UpdateRule handles PUT /api/v1/fraud/rules/{id}
func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid rule ID", r.URL.Path)
		return
	}

	var req model.UpdateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	rule, err := h.svc.UpdateRule(r.Context(), id, req)
	if err != nil {
		h.logger.Error("Failed to update rule", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update rule", r.URL.Path)
		return
	}
	if rule == nil {
		httputil.WriteNotFound(w, "Rule not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, rule)
}

// ListCases handles GET /api/v1/fraud/cases
func (h *Handler) ListCases(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var status *model.CaseStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := model.CaseStatus(s)
		status = &st
	}

	cases, total, err := h.svc.ListCases(r.Context(), tenantID, status, page, size)
	if err != nil {
		h.logger.Error("Failed to list cases", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list cases", r.URL.Path)
		return
	}
	if cases == nil {
		cases = []*model.FraudCase{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(cases, page, size, total))
}

// GetCase handles GET /api/v1/fraud/cases/{id}
func (h *Handler) GetCase(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid case ID", r.URL.Path)
		return
	}

	fraudCase, err := h.svc.GetCase(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get case", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get case", r.URL.Path)
		return
	}
	if fraudCase == nil {
		httputil.WriteNotFound(w, "Case not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, fraudCase)
}

// ListCaseNotes handles GET /api/v1/fraud/cases/{id}/notes
func (h *Handler) ListCaseNotes(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid case ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	notes, err := h.svc.ListCaseNotes(r.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to list case notes", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list case notes", r.URL.Path)
		return
	}
	if notes == nil {
		notes = []*model.CaseNote{}
	}

	httputil.WriteJSON(w, http.StatusOK, notes)
}

// AddCaseNote handles POST /api/v1/fraud/cases/{id}/notes
func (h *Handler) AddCaseNote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid case ID", r.URL.Path)
		return
	}

	var req model.AddCaseNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	note, err := h.svc.AddCaseNote(r.Context(), id, req, tenantID)
	if err != nil {
		h.logger.Error("Failed to add case note", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to add case note", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, note)
}

// ListWatchlistEntries handles GET /api/v1/fraud/watchlist
func (h *Handler) ListWatchlistEntries(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var active *bool
	if a := r.URL.Query().Get("active"); a != "" {
		val := a == "true"
		active = &val
	}

	entries, total, err := h.svc.ListWatchlistEntries(r.Context(), tenantID, active, page, size)
	if err != nil {
		h.logger.Error("Failed to list watchlist entries", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list watchlist entries", r.URL.Path)
		return
	}
	if entries == nil {
		entries = []*model.WatchlistEntry{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(entries, page, size, total))
}

// CreateWatchlistEntry handles POST /api/v1/fraud/watchlist
func (h *Handler) CreateWatchlistEntry(w http.ResponseWriter, r *http.Request) {
	var req model.CreateWatchlistEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	entry, err := h.svc.CreateWatchlistEntry(r.Context(), req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create watchlist entry", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to create watchlist entry", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, entry)
}

// GetWatchlistEntry handles GET /api/v1/fraud/watchlist/{id}
func (h *Handler) GetWatchlistEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid watchlist entry ID", r.URL.Path)
		return
	}

	entry, err := h.svc.GetWatchlistEntry(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get watchlist entry", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get watchlist entry", r.URL.Path)
		return
	}
	if entry == nil {
		httputil.WriteNotFound(w, "Watchlist entry not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, entry)
}

// ListHighRiskCustomers handles GET /api/v1/fraud/risk-profiles/high-risk
func (h *Handler) ListHighRiskCustomers(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	profiles, total, err := h.svc.ListHighRiskCustomers(r.Context(), tenantID, page, size)
	if err != nil {
		h.logger.Error("Failed to list high risk customers", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list high risk customers", r.URL.Path)
		return
	}
	if profiles == nil {
		profiles = []*model.CustomerRiskProfile{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(profiles, page, size, total))
}

// GetRiskProfile handles GET /api/v1/fraud/risk-profiles/{customerId}
func (h *Handler) GetRiskProfile(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	profile, err := h.svc.GetRiskProfile(r.Context(), tenantID, customerID)
	if err != nil {
		h.logger.Error("Failed to get risk profile", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get risk profile", r.URL.Path)
		return
	}
	if profile == nil {
		httputil.WriteNotFound(w, "Risk profile not found for customer: "+customerID, r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, profile)
}

// ListNetworkLinks handles GET /api/v1/fraud/network/{customerId}
func (h *Handler) ListNetworkLinks(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	links, err := h.svc.ListNetworkLinks(r.Context(), tenantID, customerID)
	if err != nil {
		h.logger.Error("Failed to list network links", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list network links", r.URL.Path)
		return
	}
	if links == nil {
		links = []*model.NetworkLink{}
	}

	httputil.WriteJSON(w, http.StatusOK, links)
}

// BulkAssignAlerts handles POST /api/v1/fraud/alerts/bulk/assign
func (h *Handler) BulkAssignAlerts(w http.ResponseWriter, r *http.Request) {
	var req model.BulkAlertActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	count, err := h.svc.BulkAssignAlerts(r.Context(), req)
	if err != nil {
		h.logger.Error("Failed to bulk assign alerts", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to bulk assign alerts", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"assigned": count})
}

// BulkResolveAlerts handles POST /api/v1/fraud/alerts/bulk/resolve
func (h *Handler) BulkResolveAlerts(w http.ResponseWriter, r *http.Request) {
	var req model.BulkAlertActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	count, err := h.svc.BulkResolveAlerts(r.Context(), req)
	if err != nil {
		h.logger.Error("Failed to bulk resolve alerts", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to bulk resolve alerts", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"resolved": count})
}

// CreateCase handles POST /api/v1/fraud/cases
func (h *Handler) CreateCase(w http.ResponseWriter, r *http.Request) {
	var req model.CreateCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	fraudCase, err := h.svc.CreateCase(r.Context(), req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create case", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to create case", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, fraudCase)
}

// UpdateCase handles PUT /api/v1/fraud/cases/{id}
func (h *Handler) UpdateCase(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid case ID", r.URL.Path)
		return
	}

	var req model.UpdateCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	fraudCase, err := h.svc.UpdateCase(r.Context(), id, req)
	if err != nil {
		h.logger.Error("Failed to update case", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update case", r.URL.Path)
		return
	}
	if fraudCase == nil {
		httputil.WriteNotFound(w, "Case not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, fraudCase)
}

// GetCaseTimeline handles GET /api/v1/fraud/cases/{id}/timeline
func (h *Handler) GetCaseTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid case ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	timeline, err := h.svc.GetCaseTimeline(r.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get case timeline", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get case timeline", r.URL.Path)
		return
	}
	if timeline == nil {
		httputil.WriteNotFound(w, "Case not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, timeline)
}

// DeactivateWatchlistEntry handles PUT /api/v1/fraud/watchlist/{id}/deactivate
func (h *Handler) DeactivateWatchlistEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid watchlist entry ID", r.URL.Path)
		return
	}

	entry, err := h.svc.DeactivateWatchlistEntry(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to deactivate watchlist entry", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to deactivate watchlist entry", r.URL.Path)
		return
	}
	if entry == nil {
		httputil.WriteNotFound(w, "Watchlist entry not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, entry)
}

// ScreenCustomer handles POST /api/v1/fraud/watchlist/screen
func (h *Handler) ScreenCustomer(w http.ResponseWriter, r *http.Request) {
	var req model.ScreenCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	matches, err := h.svc.ScreenCustomer(r.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("Failed to screen customer", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to screen customer", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"matches":    matches,
		"matchCount": len(matches),
	})
}

// ListRecentEvents handles GET /api/v1/fraud/events/recent
func (h *Handler) ListRecentEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	events, total, err := h.svc.ListRecentEvents(r.Context(), tenantID, page, size)
	if err != nil {
		h.logger.Error("Failed to list recent events", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list recent events", r.URL.Path)
		return
	}
	if events == nil {
		events = []*model.FraudEvent{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(events, page, size, total))
}

// ListSarReports handles GET /api/v1/fraud/sar-reports
func (h *Handler) ListSarReports(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var status *model.SarStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := model.SarStatus(s)
		status = &st
	}

	var reportType *model.SarReportType
	if rt := r.URL.Query().Get("reportType"); rt != "" {
		t := model.SarReportType(rt)
		reportType = &t
	}

	reports, total, err := h.svc.ListSarReports(r.Context(), tenantID, status, reportType, page, size)
	if err != nil {
		h.logger.Error("Failed to list SAR reports", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list SAR reports", r.URL.Path)
		return
	}
	if reports == nil {
		reports = []*model.SarReport{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(reports, page, size, total))
}

// CreateSarReport handles POST /api/v1/fraud/sar-reports
func (h *Handler) CreateSarReport(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSarReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	report, err := h.svc.CreateSarReport(r.Context(), req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create SAR report", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to create SAR report", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, report)
}

// GetSarReport handles GET /api/v1/fraud/sar-reports/{id}
func (h *Handler) GetSarReport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid SAR report ID", r.URL.Path)
		return
	}

	report, err := h.svc.GetSarReport(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get SAR report", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get SAR report", r.URL.Path)
		return
	}
	if report == nil {
		httputil.WriteNotFound(w, "SAR report not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, report)
}

// UpdateSarReport handles PUT /api/v1/fraud/sar-reports/{id}
func (h *Handler) UpdateSarReport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid SAR report ID", r.URL.Path)
		return
	}

	var req model.UpdateSarReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	report, err := h.svc.UpdateSarReport(r.Context(), id, req)
	if err != nil {
		h.logger.Error("Failed to update SAR report", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update SAR report", r.URL.Path)
		return
	}
	if report == nil {
		httputil.WriteNotFound(w, "SAR report not found: "+id.String(), r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, report)
}

// ListCustomerAlerts handles GET /api/v1/fraud/customer/{customerId}/alerts
func (h *Handler) ListCustomerAlerts(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	alerts, total, err := h.svc.ListCustomerAlerts(r.Context(), tenantID, customerID, page, size)
	if err != nil {
		h.logger.Error("Failed to list customer alerts", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list customer alerts", r.URL.Path)
		return
	}
	if alerts == nil {
		alerts = []*model.FraudAlert{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(alerts, page, size, total))
}

// BatchScreen handles POST /api/v1/fraud/screening/batch
func (h *Handler) BatchScreen(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.svc.BatchScreen(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to run batch screening", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to run batch screening", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

// GetRulesDocumentation handles GET /api/v1/fraud/rules/documentation
func (h *Handler) GetRulesDocumentation(w http.ResponseWriter, r *http.Request) {
	docs := []map[string]interface{}{
		{"ruleCode": "LARGE_SINGLE_TXN", "category": "TRANSACTION", "summary": "Flags any single transaction exceeding the CTR reporting threshold", "howItWorks": "Compares transaction amount against configured threshold. Any transaction above the limit triggers an immediate alert.", "example": "A deposit of KES 2,500,000 exceeds the 1,000,000 threshold", "regulatoryBasis": "Central Bank of Kenya AML/CFT regulations require reporting of cash transactions above KES 1,000,000", "params": []map[string]interface{}{{"key": "threshold", "label": "Amount Threshold (KES)", "default": 1000000, "type": "number"}}},
		{"ruleCode": "STRUCTURING", "category": "AML", "summary": "Detects smurfing — multiple transactions below threshold aggregating above it", "howItWorks": "Counts transactions below ceiling in time window. If 3+ transactions sum above threshold, indicates structuring.", "example": "5 deposits of KES 300,000 each within 24h totalling KES 1,500,000", "regulatoryBasis": "FATF Recommendation 20", "params": []map[string]interface{}{{"key": "threshold", "label": "Aggregate Threshold (KES)", "default": 1000000, "type": "number"}, {"key": "windowHours", "label": "Time Window (hours)", "default": 24, "type": "hours"}, {"key": "minTransactions", "label": "Min Transactions", "default": 3, "type": "count"}, {"key": "perTxnCeiling", "label": "Per-Txn Ceiling (KES)", "default": 999999, "type": "number"}}},
		{"ruleCode": "ROUND_AMOUNT_PATTERN", "category": "TRANSACTION", "summary": "Detects unusual frequency of round-number transactions", "params": []map[string]interface{}{{"key": "minRoundTxns", "label": "Min Round Txns", "default": 5, "type": "count"}, {"key": "windowHours", "label": "Window (hours)", "default": 24, "type": "hours"}, {"key": "roundThreshold", "label": "Round Divisor (KES)", "default": 10000, "type": "number"}}},
		{"ruleCode": "HIGH_VELOCITY_1H", "category": "VELOCITY", "summary": "Flags excessive transactions in 1-hour window", "params": []map[string]interface{}{{"key": "maxTransactions", "label": "Max Transactions", "default": 10, "type": "count"}, {"key": "windowMinutes", "label": "Window (min)", "default": 60, "type": "minutes"}}},
		{"ruleCode": "HIGH_VELOCITY_24H", "category": "VELOCITY", "summary": "Flags excessive transactions over 24 hours", "params": []map[string]interface{}{{"key": "maxTransactions", "label": "Max Transactions", "default": 50, "type": "count"}, {"key": "windowMinutes", "label": "Window (min)", "default": 1440, "type": "minutes"}}},
		{"ruleCode": "RAPID_FUND_MOVEMENT", "category": "AML", "summary": "Detects pass-through pattern — funds in and out within minutes", "regulatoryBasis": "FATF layering indicator", "params": []map[string]interface{}{{"key": "windowMinutes", "label": "Window (min)", "default": 15, "type": "minutes"}}},
		{"ruleCode": "APPLICATION_STACKING", "category": "APPLICATION", "summary": "Detects multiple loan applications in short period", "params": []map[string]interface{}{{"key": "maxApplications", "label": "Max Applications", "default": 5, "type": "count"}, {"key": "windowDays", "label": "Window (days)", "default": 30, "type": "days"}}},
		{"ruleCode": "EARLY_PAYOFF_SUSPICIOUS", "category": "AML", "summary": "Flags loans paid off suspiciously quickly after disbursement", "regulatoryBasis": "AML typology for trade-based money laundering", "params": []map[string]interface{}{{"key": "minDaysForAlert", "label": "Min Days", "default": 30, "type": "days"}}},
		{"ruleCode": "LOAN_CYCLING", "category": "AML", "summary": "Detects rapid loan close-and-reapply patterns", "params": []map[string]interface{}{{"key": "windowDays", "label": "Window (days)", "default": 7, "type": "days"}}},
		{"ruleCode": "DORMANT_REACTIVATION", "category": "ACCOUNT", "summary": "Flags activity on accounts dormant for extended period", "params": []map[string]interface{}{{"key": "dormantDays", "label": "Dormant Period (days)", "default": 180, "type": "days"}}},
		{"ruleCode": "KYC_BYPASS_ATTEMPT", "category": "COMPLIANCE", "summary": "Flags transactions on accounts with pending/failed KYC", "regulatoryBasis": "POCAMLA KYC requirements", "params": []map[string]interface{}{}},
		{"ruleCode": "OVERDRAFT_RAPID_DRAW", "category": "OVERDRAFT", "summary": "Flags immediate full drawdown of newly approved overdraft", "params": []map[string]interface{}{{"key": "drawdownThresholdPercent", "label": "Drawdown Threshold (%)", "default": 90, "type": "percentage"}, {"key": "windowMinutes", "label": "Window (min)", "default": 60, "type": "minutes"}}},
		{"ruleCode": "BNPL_ABUSE", "category": "APPLICATION", "summary": "Detects rapid sequential BNPL approvals", "params": []map[string]interface{}{{"key": "maxApprovals", "label": "Max Approvals", "default": 3, "type": "count"}, {"key": "windowDays", "label": "Window (days)", "default": 7, "type": "days"}}},
		{"ruleCode": "PAYMENT_REVERSAL_ABUSE", "category": "TRANSACTION", "summary": "Flags high ratio of reversed to completed payments", "params": []map[string]interface{}{{"key": "maxReversalPercent", "label": "Max Reversal Rate (%)", "default": 30, "type": "percentage"}, {"key": "minPayments", "label": "Min Payments", "default": 5, "type": "count"}}},
		{"ruleCode": "OVERPAYMENT", "category": "AML", "summary": "Flags payments exceeding outstanding loan balance", "regulatoryBasis": "FATF typology for ML through lending", "params": []map[string]interface{}{{"key": "overpaymentThresholdPercent", "label": "Threshold (%)", "default": 110, "type": "percentage"}}},
		{"ruleCode": "SUSPICIOUS_WRITEOFF", "category": "INTERNAL", "summary": "Flags write-offs on loans with recent activity", "params": []map[string]interface{}{{"key": "recentPaymentDays", "label": "Recent Activity Window (days)", "default": 30, "type": "days"}}},
		{"ruleCode": "WATCHLIST_MATCH", "category": "COMPLIANCE", "summary": "Flags customers matching PEP, sanctions, or blacklists", "regulatoryBasis": "UN Security Council, OFAC, POCAMLA", "params": []map[string]interface{}{}},
	}
	httputil.WriteJSON(w, http.StatusOK, docs)
}

// EvaluateTransaction handles POST /api/v1/fraud/evaluate
func (h *Handler) EvaluateTransaction(w http.ResponseWriter, r *http.Request) {
	var req model.EvaluateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.eng.Evaluate(r.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("Failed to evaluate transaction", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to evaluate transaction", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ListAuditLog handles GET /api/v1/fraud/audit
func (h *Handler) ListAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var entityType *string
	if et := r.URL.Query().Get("entityType"); et != "" {
		entityType = &et
	}

	var entityID *uuid.UUID
	if eid := r.URL.Query().Get("entityId"); eid != "" {
		parsed, err := uuid.Parse(eid)
		if err == nil {
			entityID = &parsed
		}
	}

	resp, err := h.svc.ListAuditLog(r.Context(), tenantID, entityType, entityID, page, size)
	if err != nil {
		h.logger.Error("Failed to list audit log", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list audit log", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// VerifyAuditChain handles GET /api/v1/fraud/audit/verify — reports whether the
// tamper-evident audit trail is intact, or the seq of the first altered or
// missing entry, so an auditor can prove the log was not edited or back-dated.
func (h *Handler) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.svc.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to verify audit chain", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to verify audit chain", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}
