package auth

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// Middleware returns a chi-compatible JWT authentication middleware.
// Port of LmsJwtAuthenticationFilter.java.
//
// Supports two auth modes:
//  1. Bearer JWT token in Authorization header
//  2. Internal service key in X-Service-Key header (service-to-service)
type Middleware struct {
	jwt                *JWTUtil
	internalServiceKey string
	logger             *zap.Logger
}

// NewMiddleware creates a new auth middleware.
func NewMiddleware(jwt *JWTUtil, internalServiceKey string, logger *zap.Logger) *Middleware {
	return &Middleware{
		jwt:                jwt,
		internalServiceKey: internalServiceKey,
		logger:             logger,
	}
}

// Handler returns the HTTP middleware handler.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health endpoints
		if r.URL.Path == "/actuator/health" {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := m.jwt.ParseToken(token)
			if err != nil {
				m.logger.Warn("JWT validation failed", zap.Error(err))
				httputil.WriteErrorJSON(w, http.StatusUnauthorized, "Unauthorized",
					"Authentication required. Provide a valid Bearer token or service key.", r.URL.Path)
				return
			}

			ctx = WithTenantID(ctx, claims.TenantID)
			ctx = WithUserID(ctx, claims.Username)
			ctx = WithRoles(ctx, claims.Roles)
			if claims.PermissionsSet {
				ctx = WithPermissions(ctx, claims.Permissions)
			}
			if claims.CustomerIDStr != "" {
				ctx = WithCustomerIDStr(ctx, claims.CustomerIDStr)
				if claims.CustomerID != nil {
					ctx = WithCustomerID(ctx, *claims.CustomerID)
				}
			}
			if claims.MobileUserID != "" {
				ctx = WithMobileUserID(ctx, claims.MobileUserID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fallback: internal service key authentication
		serviceKey := r.Header.Get("X-Service-Key")
		if m.internalServiceKey != "" && serviceKey == m.internalServiceKey {
			tenantID := r.Header.Get("X-Service-Tenant")
			if tenantID == "" {
				tenantID = "default"
			}
			serviceUser := r.Header.Get("X-Service-User")
			if serviceUser == "" {
				serviceUser = "internal-service"
			}

			ctx = WithTenantID(ctx, tenantID)
			ctx = WithUserID(ctx, serviceUser)
			ctx = WithRoles(ctx, []string{"SERVICE", "ADMIN"})

			m.logger.Debug("Authenticated internal service call",
				zap.String("user", serviceUser),
				zap.String("tenant", tenantID))

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// No valid credentials
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "Unauthorized",
			"Authentication required. Provide a valid Bearer token or service key.", r.URL.Path)
	})
}

// RequireRole returns a chi middleware that authorises the request only if the
// caller holds at least one of the allowed roles (case-insensitive). It must be
// chained AFTER Handler (which populates roles in context).
//
// Internal service-to-service calls (which carry the SERVICE role) always pass,
// so system flows — e.g. a loan disbursement crediting an account — are never
// blocked. A caller lacking an allowed role gets HTTP 403.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	allow := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		allow[strings.ToUpper(strings.TrimSpace(r))] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, role := range RolesFromContext(r.Context()) {
				ru := strings.ToUpper(strings.TrimSpace(role))
				if ru == "SERVICE" || allow[ru] {
					next.ServeHTTP(w, r)
					return
				}
			}
			httputil.WriteErrorJSON(w, http.StatusForbidden, "Forbidden",
				"You do not have permission to perform this operation.", r.URL.Path)
		})
	}
}

// RequirePermission returns a chi middleware that authorises the request only if
// the caller holds the given RBAC permission. It must be chained AFTER Handler.
//
// Authorisation precedence:
//  1. Internal service-to-service calls (SERVICE role) always pass — system
//     flows are never blocked.
//  2. If the token carries a permissions claim (RBAC-aware), the caller passes
//     iff that claim contains `permission`.
//  3. If the token carries NO permissions claim (e.g. issued before the matrix
//     existed, or by a service that doesn't stamp permissions), it falls back to
//     a role check against fallbackRoles — preserving the previous behaviour so
//     the rollout is safe. Pass the same roles the route used with RequireRole.
//
// A caller who satisfies none of these gets HTTP 403.
func RequirePermission(permission string, fallbackRoles ...string) func(http.Handler) http.Handler {
	fallback := make(map[string]bool, len(fallbackRoles))
	for _, r := range fallbackRoles {
		fallback[strings.ToUpper(strings.TrimSpace(r))] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// (1) Internal service calls always pass.
			for _, role := range RolesFromContext(ctx) {
				if strings.EqualFold(strings.TrimSpace(role), "SERVICE") {
					next.ServeHTTP(w, r)
					return
				}
			}

			perms, present := PermissionsFromContext(ctx)
			if present {
				// (2) RBAC-aware token: check the permission claim.
				for _, p := range perms {
					if p == permission {
						next.ServeHTTP(w, r)
						return
					}
				}
			} else if len(fallback) > 0 {
				// (3) Legacy token: fall back to a role check.
				for _, role := range RolesFromContext(ctx) {
					if fallback[strings.ToUpper(strings.TrimSpace(role))] {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			httputil.WriteErrorJSON(w, http.StatusForbidden, "Forbidden",
				"You do not have permission to perform this operation.", r.URL.Path)
		})
	}
}
