package middleware

import "net/http"

// CORS returns a permissive CORS middleware for app-facing (BFF) services.
// It echoes the request Origin (so credentialed requests work — browsers
// reject the "*" wildcard when credentials are sent) and short-circuits
// OPTIONS preflight. Staff-facing services behind the lms-api-gateway use the
// gateway's stricter allow-list instead; the mobile/web BFF endpoints are by
// design callable from any origin, with security enforced by JWT auth rather
// than the origin check.
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-Service-Key, X-Service-Tenant, X-Service-User")
			w.Header().Set("Access-Control-Max-Age", "3600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
