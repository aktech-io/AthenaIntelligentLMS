// Mobile (customer-facing) JWT support, promoted from the AthenaMobileWallet
// BFF's shared/auth library when the wallet BFF was folded into this monorepo
// (Nemo A1 Phase 0).
//
// The staff-facing side of this package only *validates* tokens (they are
// issued by the LMS auth flow). The mobile BFF gateway additionally *issues*
// tokens for app users: short-lived access tokens carrying the mobile user's
// UUID, customer ID and tenant, plus long-lived refresh tokens carrying a JTI
// for rotation. Both are signed with the same HMAC secret so every service —
// staff or mobile — can validate every token with the shared middleware.
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrTokenExpired is returned when a mobile token is past its expiry.
	ErrTokenExpired = errors.New("token has expired")
	// ErrTokenInvalid is returned for any other validation failure.
	ErrTokenInvalid = errors.New("token is invalid")
)

// MobileClaims is the typed claim set used by mobile (app-user) tokens.
// JSON claim names match the wallet BFF exactly — tokens issued before the
// fold-in keep validating.
type MobileClaims struct {
	jwt.RegisteredClaims
	Roles      []string `json:"roles,omitempty"`
	CustomerID string   `json:"customerId,omitempty"`
	TenantID   string   `json:"tenantId,omitempty"`
	UserID     string   `json:"userId,omitempty"`
}

// GenerateToken creates a mobile access token. subject is the phone number;
// userID is the mobile user's UUID.
func (j *JWTUtil) GenerateToken(subject string, roles []string, customerID, tenantID, userID string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := MobileClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		Roles:      roles,
		CustomerID: customerID,
		TenantID:   tenantID,
		UserID:     userID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.signingKey)
}

// GenerateRefreshToken creates a mobile refresh token with a JTI claim used
// for rotation/revocation.
func (j *JWTUtil) GenerateRefreshToken(subject, jti, tenantID string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := MobileClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		TenantID: tenantID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.signingKey)
}

// ValidateToken parses and validates a mobile token, returning typed claims.
// Distinguishes expiry (ErrTokenExpired) from other failures (ErrTokenInvalid).
func (j *JWTUtil) ValidateToken(tokenStr string) (*MobileClaims, error) {
	claims := &MobileClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.signingKey, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	if !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// ServiceKeyTransport is an http.RoundTripper that stamps outbound requests
// with the X-Service-Key / X-Service-Tenant / X-Service-User service-auth
// headers. The tenant is taken from the request context at call time, so a
// client built once still propagates the caller's tenant per request.
// Promoted from the wallet BFF; equivalent to httputil.ServiceClient for code
// that prefers to keep its own *http.Client.
type ServiceKeyTransport struct {
	Base        http.RoundTripper
	ServiceKey  string
	ServiceName string
}

// RoundTrip implements http.RoundTripper.
func (t *ServiceKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Service-Key", t.ServiceKey)
	req.Header.Set("X-Service-Tenant", TenantIDOrDefault(req.Context()))
	req.Header.Set("X-Service-User", t.ServiceName)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
