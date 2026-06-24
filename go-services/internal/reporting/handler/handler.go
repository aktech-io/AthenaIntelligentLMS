package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/reporting/model"
	"github.com/athena-lms/go-services/internal/reporting/service"
)

// Handler exposes reporting HTTP endpoints.
type Handler struct {
	svc         *service.Service
	logger      *zap.Logger
	loanClient  *httputil.ServiceClient
	loanMgmtURL string
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// SetLoanManagementClient wires the loan-management client used for live
// portfolio figures in the summary.
func (h *Handler) SetLoanManagementClient(client *httputil.ServiceClient, baseURL string) {
	h.loanClient = client
	h.loanMgmtURL = baseURL
}

// Routes registers all reporting routes on the given chi.Router.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/api/v1/reporting", func(r chi.Router) {
		r.Get("/events", h.getEvents)
		r.Get("/snapshots", h.getSnapshots)
		r.Get("/snapshots/latest", h.getLatestSnapshot)
		r.Get("/metrics", h.getMetrics)
		r.Get("/summary", h.getSummary)
		r.Post("/snapshots/generate", h.generateSnapshot)
	})
}

func (h *Handler) getEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	eventType := r.URL.Query().Get("eventType")
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 50)

	var from, to *time.Time
	if fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid 'from' timestamp: "+err.Error(), r.URL.Path)
			return
		}
		from = &t
	}
	if toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid 'to' timestamp: "+err.Error(), r.URL.Path)
			return
		}
		to = &t
	}

	resp, err := h.svc.GetEvents(r.Context(), tenantID, eventType, from, to, page, size)
	if err != nil {
		h.logger.Error("Failed to get events", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to retrieve events", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getSnapshots(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 30)

	resp, err := h.svc.GetSnapshots(r.Context(), tenantID, page, size)
	if err != nil {
		h.logger.Error("Failed to get snapshots", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to retrieve snapshots", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.GetLatestSnapshot(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get latest snapshot", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to retrieve latest snapshot", r.URL.Path)
		return
	}
	if resp == nil {
		httputil.WriteNotFound(w, "No portfolio snapshot found for tenant: "+tenantID, r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		httputil.WriteBadRequest(w, "'from' and 'to' date parameters are required", r.URL.Path)
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid 'from' date: "+err.Error(), r.URL.Path)
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid 'to' date: "+err.Error(), r.URL.Path)
		return
	}

	metrics, err := h.svc.GetMetrics(r.Context(), tenantID, from, to)
	if err != nil {
		h.logger.Error("Failed to get metrics", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to retrieve metrics", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, metrics)
}

func (h *Handler) getSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	today := time.Now().UTC().Format("2006-01-02")

	summary := model.PortfolioSummaryResponse{
		TenantID: tenantID,
		AsOfDate: today,
	}

	// Prefer LIVE portfolio totals from loan-management (the event-snapshot
	// pipeline is a lagging/secondary source). Falls through to the snapshot for
	// risk metrics (PAR, staging) below.
	liveLoaded := false
	if h.loanClient != nil && h.loanMgmtURL != "" {
		var stats struct {
			TotalLoans       int             `json:"totalLoans"`
			ActiveLoans      int             `json:"activeLoans"`
			ClosedLoans      int             `json:"closedLoans"`
			DefaultedLoans   int             `json:"defaultedLoans"`
			TotalDisbursed   decimal.Decimal `json:"totalDisbursed"`
			TotalOutstanding decimal.Decimal `json:"totalOutstanding"`
		}
		url := h.loanMgmtURL + "/api/v1/loans/portfolio-stats"
		if err := h.loanClient.Get(r.Context(), url, &stats); err != nil {
			h.logger.Warn("Could not fetch live portfolio stats; falling back to snapshot",
				zap.String("tenant", tenantID), zap.Error(err))
		} else {
			summary.TotalLoans = stats.TotalLoans
			summary.ActiveLoans = stats.ActiveLoans
			summary.ClosedLoans = stats.ClosedLoans
			summary.DefaultedLoans = stats.DefaultedLoans
			summary.TotalDisbursed = stats.TotalDisbursed.InexactFloat64()
			summary.TotalOutstanding = stats.TotalOutstanding.InexactFloat64()
			liveLoaded = true
		}
	}

	latest, err := h.svc.GetLatestSnapshot(r.Context(), tenantID)
	if err != nil {
		h.logger.Warn("No latest snapshot available, returning empty summary",
			zap.String("tenant", tenantID), zap.Error(err))
	}

	if latest != nil {
		// Portfolio totals: only from the snapshot if we couldn't load them live.
		if !liveLoaded {
			summary.TotalLoans = latest.TotalLoans
			summary.ActiveLoans = latest.ActiveLoans
			summary.ClosedLoans = latest.ClosedLoans
			summary.DefaultedLoans = latest.DefaultedLoans
			summary.TotalDisbursed = latest.TotalDisbursed
			summary.TotalOutstanding = latest.TotalOutstanding
		}
		// Risk metrics always come from the snapshot.
		summary.TotalCollected = latest.TotalCollected
		summary.Par30 = latest.Par30
		summary.Par90 = latest.Par90
		summary.WatchLoans = latest.WatchLoans
		summary.SubstandardLoans = latest.SubstandardLoans
		summary.DoubtfulLoans = latest.DoubtfulLoans
		summary.LossLoans = latest.LossLoans
	} else if !liveLoaded {
		summary.TotalDisbursed = 0
		summary.TotalOutstanding = 0
		summary.TotalCollected = 0
		summary.Par30 = 0
		summary.Par90 = 0
	}

	// Fetch today's metrics (logged, not included in response — matches Java)
	todayDate, _ := time.Parse("2006-01-02", today)
	todayMetrics, _ := h.svc.GetMetrics(r.Context(), tenantID, todayDate, todayDate)
	h.logger.Debug("Today's metrics",
		zap.Int("count", len(todayMetrics)),
		zap.String("tenant", tenantID),
	)

	httputil.WriteJSON(w, http.StatusOK, summary)
}

func (h *Handler) generateSnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	if err := h.svc.GenerateDailySnapshot(r.Context(), tenantID); err != nil {
		h.logger.Error("Failed to generate snapshot", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to generate snapshot", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
