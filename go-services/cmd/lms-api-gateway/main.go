package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/metrics"
	"github.com/athena-lms/go-services/internal/common/tracing"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
)

// ---------------------------------------------------------------------------
// Route configuration
// ---------------------------------------------------------------------------

// RouteConfig defines a single gateway route mapping.
type RouteConfig struct {
	// ID is a human-readable name for the route (used in logs and circuit breaker keys).
	ID string
	// PathPrefix is the path prefix to match (e.g. "/lms/api/v1/accounts/").
	PathPrefix string
	// TargetURL is the upstream service URL including scheme and port.
	TargetURL string
	// EnvOverride is the environment variable that can override TargetURL
	// for strangler-fig switching (e.g. "ROUTE_ACCOUNT_SERVICE_URL").
	EnvOverride string
	// Public indicates the route does NOT require authentication.
	Public bool
}

// defaultRoutes returns all gateway routes matching the Java Spring Cloud Gateway config.
// The order matters: more-specific prefixes must come before less-specific ones so that
// "/lms/api/v1/loan-applications/" matches before "/lms/api/v1/loans/".
func defaultRoutes() []RouteConfig {
	return []RouteConfig{
		{
			ID:          "account-service",
			PathPrefix:  "/lms/api/v1/accounts/",
			TargetURL:   "http://account-service.lms.svc.cluster.local:8086",
			EnvOverride: "ROUTE_ACCOUNT_SERVICE_URL",
		},
		{
			ID:          "account-service-customers",
			PathPrefix:  "/lms/api/v1/customers/",
			TargetURL:   "http://account-service.lms.svc.cluster.local:8086",
			EnvOverride: "ROUTE_ACCOUNT_SERVICE_URL",
		},
		{
			ID:          "account-service-auth",
			PathPrefix:  "/lms/api/auth/",
			TargetURL:   "http://account-service.lms.svc.cluster.local:8086",
			EnvOverride: "ROUTE_ACCOUNT_SERVICE_URL",
			Public:      true,
		},
		{
			ID:          "product-service",
			PathPrefix:  "/lms/api/v1/products/",
			TargetURL:   "http://product-service.lms.svc.cluster.local:8087",
			EnvOverride: "ROUTE_PRODUCT_SERVICE_URL",
		},
		{
			ID:          "loan-origination-service",
			PathPrefix:  "/lms/api/v1/loan-applications/",
			TargetURL:   "http://loan-origination-service.lms.svc.cluster.local:8088",
			EnvOverride: "ROUTE_LOAN_ORIGINATION_SERVICE_URL",
		},
		{
			ID:          "loan-management-service",
			PathPrefix:  "/lms/api/v1/loans/",
			TargetURL:   "http://loan-management-service.lms.svc.cluster.local:8089",
			EnvOverride: "ROUTE_LOAN_MANAGEMENT_SERVICE_URL",
		},
		{
			ID:          "payment-service",
			PathPrefix:  "/lms/api/v1/payments/",
			TargetURL:   "http://payment-service.lms.svc.cluster.local:8090",
			EnvOverride: "ROUTE_PAYMENT_SERVICE_URL",
		},
		{
			ID:          "accounting-service",
			PathPrefix:  "/lms/api/v1/accounting/",
			TargetURL:   "http://accounting-service.lms.svc.cluster.local:8091",
			EnvOverride: "ROUTE_ACCOUNTING_SERVICE_URL",
		},
		{
			ID:          "float-service",
			PathPrefix:  "/lms/api/v1/float/",
			TargetURL:   "http://float-service.lms.svc.cluster.local:8092",
			EnvOverride: "ROUTE_FLOAT_SERVICE_URL",
		},
		{
			ID:          "collections-service",
			PathPrefix:  "/lms/api/v1/collections/",
			TargetURL:   "http://collections-service.lms.svc.cluster.local:8093",
			EnvOverride: "ROUTE_COLLECTIONS_SERVICE_URL",
		},
		{
			ID:          "compliance-service",
			PathPrefix:  "/lms/api/v1/compliance/",
			TargetURL:   "http://compliance-service.lms.svc.cluster.local:8094",
			EnvOverride: "ROUTE_COMPLIANCE_SERVICE_URL",
		},
		{
			// A2 self-service onboarding lives on compliance-service; this
			// route exposes the officer queue/decision endpoints to the staff
			// portal (JWT). The mobile BFF does NOT use it — it calls
			// compliance-service directly with the service key, which this
			// public gateway deliberately strips (CRIT-1).
			ID:          "compliance-service-onboarding",
			PathPrefix:  "/lms/api/v1/onboarding/",
			TargetURL:   "http://compliance-service.lms.svc.cluster.local:8094",
			EnvOverride: "ROUTE_COMPLIANCE_SERVICE_URL",
		},
		{
			ID:          "reporting-service",
			PathPrefix:  "/lms/api/v1/reports/",
			TargetURL:   "http://reporting-service.lms.svc.cluster.local:8095",
			EnvOverride: "ROUTE_REPORTING_SERVICE_URL",
		},
		{
			ID:          "ai-scoring-service",
			PathPrefix:  "/lms/api/v1/scoring/",
			TargetURL:   "http://ai-scoring-service.lms.svc.cluster.local:8096",
			EnvOverride: "ROUTE_AI_SCORING_SERVICE_URL",
		},
		{
			ID:          "overdraft-service",
			PathPrefix:  "/lms/api/v1/wallets/",
			TargetURL:   "http://overdraft-service.lms.svc.cluster.local:8097",
			EnvOverride: "ROUTE_OVERDRAFT_SERVICE_URL",
		},
		{
			ID:          "media-service",
			PathPrefix:  "/lms/api/v1/media/",
			TargetURL:   "http://media-service.lms.svc.cluster.local:8098",
			EnvOverride: "ROUTE_MEDIA_SERVICE_URL",
		},
		{
			// B1 card issuing: staff card operations (issue/freeze/limits).
			ID:          "card-service",
			PathPrefix:  "/lms/api/v1/cards/",
			TargetURL:   "http://card-service.lms.svc.cluster.local:8107",
			EnvOverride: "ROUTE_CARD_SERVICE_URL",
		},
		{
			ID:          "fraud-detection-service",
			PathPrefix:  "/lms/api/fraud/",
			TargetURL:   "http://fraud-detection-service.lms.svc.cluster.local:8100",
			EnvOverride: "ROUTE_FRAUD_DETECTION_SERVICE_URL",
		},
	}
}

// ---------------------------------------------------------------------------
// Circuit Breaker
// ---------------------------------------------------------------------------

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // healthy — requests flow through
	CircuitOpen                         // unhealthy — requests are rejected
	CircuitHalfOpen                     // testing — limited requests allowed
)

// CircuitBreaker implements a simple count-based circuit breaker per service,
// modelled after the Resilience4j config in the Java gateway.
type CircuitBreaker struct {
	mu sync.Mutex

	state     CircuitState
	failures  int
	successes int // successes in half-open state

	// Config — mirrors Resilience4j defaults from Java gateway
	failureThreshold       int           // consecutive failures to open
	halfOpenPermittedCalls int           // successful calls required in half-open to close
	openStateDuration      time.Duration // how long to stay open before half-open
	openStateStart         time.Time
}

// NewCircuitBreaker creates a circuit breaker with the same defaults as the Java gateway's
// Resilience4j config (slidingWindowSize=10, failureRate=50% => ~5 failures, wait=10s, halfOpenCalls=3).
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		state:                  CircuitClosed,
		failureThreshold:       5,
		halfOpenPermittedCalls: 3,
		openStateDuration:      10 * time.Second,
	}
}

// Allow returns true if a request should be forwarded to the upstream service.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.openStateStart) >= cb.openStateDuration {
			cb.state = CircuitHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return false
}

// RecordSuccess records a successful upstream call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failures = 0
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenPermittedCalls {
			cb.state = CircuitClosed
			cb.failures = 0
			cb.successes = 0
		}
	}
}

// RecordFailure records a failed upstream call.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.failureThreshold {
			cb.state = CircuitOpen
			cb.openStateStart = time.Now()
		}
	case CircuitHalfOpen:
		cb.state = CircuitOpen
		cb.openStateStart = time.Now()
		cb.successes = 0
	}
}

// State returns the current circuit state (for health/diagnostics).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// ---------------------------------------------------------------------------
// Gateway
// ---------------------------------------------------------------------------

// Gateway holds the proxy infrastructure.
type Gateway struct {
	logger          *zap.Logger
	routes          []RouteConfig
	circuitBreakers map[string]*CircuitBreaker
}

// NewGateway creates the gateway, resolving env-var overrides for each route target.
func NewGateway(logger *zap.Logger) *Gateway {
	routes := defaultRoutes()
	cbs := make(map[string]*CircuitBreaker, len(routes))

	for i := range routes {
		// Env-var override for strangler-fig switching
		if routes[i].EnvOverride != "" {
			if v := os.Getenv(routes[i].EnvOverride); v != "" {
				routes[i].TargetURL = v
			}
		}
		cbs[routes[i].ID] = NewCircuitBreaker()
	}

	return &Gateway{
		logger:          logger,
		routes:          routes,
		circuitBreakers: cbs,
	}
}

// stripPrefix removes the "/lms" prefix from the request path, matching the
// StripPrefix=1 filter from the Java gateway config.
func stripPrefix(path string) string {
	if strings.HasPrefix(path, "/lms/") {
		return "/" + strings.TrimPrefix(path, "/lms/")
	}
	if path == "/lms" {
		return "/"
	}
	return path
}

// newReverseProxy creates a httputil.ReverseProxy for a given target URL.
func (gw *Gateway) newReverseProxy(targetURL string, routeID string) *httputil.ReverseProxy {
	target, err := url.Parse(targetURL)
	if err != nil {
		gw.logger.Fatal("Invalid route target URL", zap.String("route", routeID), zap.String("url", targetURL), zap.Error(err))
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Custom director: rewrite host + strip /lms prefix
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = stripPrefix(req.URL.Path)
		req.URL.RawPath = stripPrefix(req.URL.RawPath)
		req.Host = target.Host
		// SECURITY (CRIT-1): never forward client-supplied internal service-auth
		// headers to backends. The service-key path grants SERVICE+ADMIN on any
		// tenant; allowing a client to smuggle these through the public gateway
		// is a full auth/tenant bypass. Internal service-to-service calls do not
		// transit the gateway, so stripping here is safe.
		req.Header.Del("X-Service-Key")
		req.Header.Del("X-Service-Tenant")
		req.Header.Del("X-Service-User")
	}

	// Custom error handler to record circuit breaker failures
	cb := gw.circuitBreakers[routeID]
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		gw.logger.Error("Upstream error",
			zap.String("route", routeID),
			zap.String("target", targetURL),
			zap.String("path", r.URL.Path),
			zap.Error(err),
		)
		cb.RecordFailure()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"status":  502,
			"error":   "Bad Gateway",
			"message": fmt.Sprintf("Service %s is unavailable", routeID),
			"path":    r.URL.Path,
		})
	}

	// Wrap the default transport to record successes/failures based on response
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 500 {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
		return nil
	}

	return proxy
}

// RegisterRoutes registers all proxy routes onto the chi router.
// Public routes (like auth endpoints) skip JWT middleware.
// Protected routes are registered with the auth middleware.
//
// loginLimiter, when non-nil, is applied ONLY to the public auth route as an
// extra, stricter rate limit on top of the gateway-wide limiter. SECURITY
// (HIGH-3): the login endpoint is the prime brute-force target, so it gets a
// tighter per-IP bucket than ordinary API traffic. Pass nil to skip it (tests).
func (gw *Gateway) RegisterRoutes(r chi.Router, authMw *auth.Middleware, loginLimiter func(http.Handler) http.Handler) {
	publicRoutes := make([]RouteConfig, 0)
	protectedRoutes := make([]RouteConfig, 0)

	for _, route := range gw.routes {
		if route.Public {
			publicRoutes = append(publicRoutes, route)
		} else {
			protectedRoutes = append(protectedRoutes, route)
		}
	}

	// registerRoute registers both the wildcard and exact-prefix patterns for a route,
	// so that /lms/api/v1/products and /lms/api/v1/products/ both match. An optional
	// per-route middleware (mw) wraps the handler before registration.
	registerRoute := func(r chi.Router, route RouteConfig, proxy *httputil.ReverseProxy, cb *CircuitBreaker, mw func(http.Handler) http.Handler) {
		var handler http.Handler = gw.proxyHandler(proxy, cb, route)
		if mw != nil {
			handler = mw(handler)
		}
		r.Handle(route.PathPrefix+"*", handler)
		// Also register without trailing wildcard (e.g., /lms/api/v1/products)
		bare := strings.TrimRight(route.PathPrefix, "/")
		if bare != route.PathPrefix {
			r.Handle(bare, handler)
		}
	}

	// Public routes — no JWT required
	for _, route := range publicRoutes {
		route := route
		proxy := gw.newReverseProxy(route.TargetURL, route.ID)
		cb := gw.circuitBreakers[route.ID]
		var mw func(http.Handler) http.Handler
		if route.ID == "account-service-auth" {
			mw = loginLimiter
		}
		registerRoute(r, route, proxy, cb, mw)
		gw.logger.Info("Registered public route",
			zap.String("id", route.ID),
			zap.String("prefix", route.PathPrefix),
			zap.String("target", route.TargetURL),
		)
	}

	// Protected routes — JWT required
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		for _, route := range protectedRoutes {
			route := route
			proxy := gw.newReverseProxy(route.TargetURL, route.ID)
			cb := gw.circuitBreakers[route.ID]
			registerRoute(r, route, proxy, cb, nil)
			gw.logger.Info("Registered protected route",
				zap.String("id", route.ID),
				zap.String("prefix", route.PathPrefix),
				zap.String("target", route.TargetURL),
			)
		}
	})
}

// proxyHandler returns an http.HandlerFunc that checks the circuit breaker and proxies the request.
func (gw *Gateway) proxyHandler(proxy *httputil.ReverseProxy, cb *CircuitBreaker, route RouteConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cb.Allow() {
			gw.logger.Warn("Circuit breaker open, rejecting request",
				zap.String("route", route.ID),
				zap.String("path", r.URL.Path),
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status":  503,
				"error":   "Service Unavailable",
				"message": fmt.Sprintf("Service %s circuit breaker is open", route.ID),
				"path":    r.URL.Path,
			})
			return
		}
		proxy.ServeHTTP(w, r)
	}
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

// newCORSMiddleware builds a CORS middleware restricted to an explicit origin
// allowlist. SECURITY (HIGH-1): the previous implementation reflected ANY Origin
// while also sending Access-Control-Allow-Credentials: true, which lets any web
// page make credentialed cross-origin calls. We now only echo an Origin (and set
// credentials) when it is explicitly allowed. A literal "*" entry allows all
// origins but WITHOUT credentials (the only spec-safe wildcard).
//
// Non-browser clients (mobile, USSD) send no Origin and are unaffected.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newCORSMiddleware(allowed []string) func(http.Handler) http.Handler {
	allowAll := false
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		} else if o != "" {
			set[o] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				switch {
				case set[origin]:
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Add("Vary", "Origin")
				case allowAll:
					// Wildcard without credentials (cannot combine "*" + credentials).
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "3600")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Health endpoint
// ---------------------------------------------------------------------------

func healthHandler(gw *Gateway) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		components := make(map[string]any, len(gw.routes))
		for _, route := range gw.routes {
			cb := gw.circuitBreakers[route.ID]
			state := "UP"
			switch cb.State() {
			case CircuitOpen:
				state = "DOWN"
			case CircuitHalfOpen:
				state = "HALF_OPEN"
			}
			components[route.ID] = map[string]any{
				"status": state,
				"target": route.TargetURL,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":     "UP",
			"components": components,
		})
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("lms-api-gateway")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Distributed tracing (H1): no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
	cfg.Port = envInt("PORT", 8105)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// JWT
	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}

	// Gateway
	gw := NewGateway(logger)

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))
	// CORS origin allowlist (comma-separated env, e.g. https://portal.example.com).
	// Defaults to local dev portal origins; set LMS_CORS_ALLOWED_ORIGINS in prod.
	corsOrigins := strings.Split(envOrDefault("LMS_CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001"), ",")
	r.Use(newCORSMiddleware(corsOrigins))

	// SECURITY (HIGH-3): gateway-wide IP/subject rate limiting at ingress, before
	// proxying, to blunt API abuse and brute-force. The /actuator/health path is
	// exempt inside the limiter. NOTE: this is a PER-INSTANCE (per-pod) limiter;
	// see RateLimiterConfig for the shared-store (Redis) path to global limits.
	rateEnabled := envBool("LMS_RATE_LIMIT_ENABLED", true)
	trustProxy := envBool("LMS_RATE_LIMIT_TRUST_PROXY", false)
	apiLimiter := commonmw.NewRateLimiter(commonmw.RateLimiterConfig{
		Enabled:           rateEnabled,
		RPS:               envFloat("LMS_RATE_LIMIT_RPS", 20),
		Burst:             envInt("LMS_RATE_LIMIT_BURST", 40),
		IdleTTL:           time.Duration(envInt("LMS_RATE_LIMIT_TTL_SECONDS", 600)) * time.Second,
		MaxKeys:           envInt("LMS_RATE_LIMIT_MAX_KEYS", 100_000),
		TrustProxyHeaders: trustProxy,
	}, logger)
	defer apiLimiter.Stop()
	r.Use(apiLimiter.Middleware)

	// Stricter limiter applied only to the login/auth route — the prime
	// brute-force target. Defaults to a slow trickle with a small burst so a
	// human can still retry but a flood is rejected. Pairs with the account
	// service's per-username login lockout (HIGH-3 / LOW-5).
	loginLimiter := commonmw.NewRateLimiter(commonmw.RateLimiterConfig{
		Enabled:           rateEnabled,
		RPS:               envFloat("LMS_LOGIN_RATE_LIMIT_RPS", 0.5),
		Burst:             envInt("LMS_LOGIN_RATE_LIMIT_BURST", 10),
		IdleTTL:           time.Duration(envInt("LMS_RATE_LIMIT_TTL_SECONDS", 600)) * time.Second,
		MaxKeys:           envInt("LMS_RATE_LIMIT_MAX_KEYS", 100_000),
		TrustProxyHeaders: trustProxy,
	}, logger)
	defer loginLimiter.Stop()

	// Health endpoint (unauthenticated)
	r.Get("/actuator/health", healthHandler(gw))
	// Prometheus metrics (H2): unauthenticated, scraped in-cluster only.
	r.Handle("/metrics", metrics.Handler())

	// Auth middleware. SECURITY (CRIT-1): the public gateway must NOT honour the
	// internal service key — that path grants SERVICE+ADMIN on a client-supplied
	// tenant and would bypass JWT auth and all RBAC. Service-to-service calls go
	// directly between in-cluster services, never through this ingress, so the
	// gateway is constructed with an empty internal key (JWT only).
	authMw := auth.NewMiddleware(jwtUtil, "", logger)

	// Register all proxy routes (login route gets the stricter limiter)
	gw.RegisterRoutes(r, authMw, loginLimiter.Middleware)

	// Server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      tracing.WrapHandler(r, cfg.ServiceName),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Info("Starting lms-api-gateway",
		zap.Int("port", cfg.Port),
		zap.String("service", cfg.ServiceName),
		zap.Int("routes", len(gw.routes)),
	)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server failed", zap.Error(err))
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return fallback
}

// envFloat reads a positive float env var, falling back on empty/invalid/non-positive.
func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// envBool reads a boolean env var (true/false/1/0). Falls back on empty/invalid.
func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return fallback
}
