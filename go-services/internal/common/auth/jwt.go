package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
)

// JWTUtil parses and validates JWT tokens.
// Port of Java JwtUtil.java — same HMAC secret, same claims structure.
type JWTUtil struct {
	signingKey []byte
}

// NewJWTUtil creates a JWTUtil from a base64-encoded HMAC secret.
//
// SECURITY (HIGH-2): an empty or too-short secret is rejected rather than
// accepted. Previously an empty JWT_SECRET decoded to an empty HMAC key and was
// used as-is, which is a fail-open: tokens validate against a known/empty key.
// HMAC-SHA256 needs at least a 256-bit (32-byte) key for full strength.
func NewJWTUtil(base64Secret string) (*JWTUtil, error) {
	key, err := base64.StdEncoding.DecodeString(base64Secret)
	if err != nil {
		return nil, fmt.Errorf("decode jwt secret: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("jwt secret too weak: decoded to %d bytes, require >= 32 (256-bit)", len(key))
	}
	return &JWTUtil{signingKey: key}, nil
}

// Claims holds the extracted JWT claims matching the Java token structure.
type Claims struct {
	Username   string
	TenantID   string
	CustomerID *int64  // nil if not present or not numeric (e.g. mobile wallet string IDs)
	CustomerIDStr string // raw string value of customerId claim
	Roles      []string
	Permissions    []string // effective permission keys (RBAC matrix), if stamped
	PermissionsSet bool     // true if the token carried a permissions claim at all
	PermVersion    int64    // matrix version the permissions were resolved from
}

// ParseToken validates the token signature and extracts claims.
func (j *JWTUtil) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	c := &Claims{}

	// Subject = username
	if sub, ok := mapClaims["sub"].(string); ok {
		c.Username = sub
	}

	// TenantID — fall back to subject for single-tenant deployments
	if tid, ok := mapClaims["tenantId"].(string); ok {
		c.TenantID = tid
	} else {
		c.TenantID = c.Username
	}

	// CustomerID — supports both numeric and string IDs (mobile wallet compat)
	if cid, ok := mapClaims["customerId"]; ok && cid != nil {
		cidStr := fmt.Sprintf("%v", cid)
		c.CustomerIDStr = cidStr
		if n, err := strconv.ParseInt(cidStr, 10, 64); err == nil {
			c.CustomerID = &n
		}
	}

	// Roles
	if roles, ok := mapClaims["roles"].([]any); ok {
		for _, r := range roles {
			if s, ok := r.(string); ok {
				c.Roles = append(c.Roles, s)
			}
		}
	}

	// Permissions (RBAC matrix) — optional. Presence is tracked separately so
	// enforcement can fall back to role checks for tokens issued before the
	// matrix existed.
	if perms, ok := mapClaims["permissions"].([]any); ok {
		c.PermissionsSet = true
		for _, p := range perms {
			if s, ok := p.(string); ok {
				c.Permissions = append(c.Permissions, s)
			}
		}
	}
	if v, ok := mapClaims["permVersion"].(float64); ok {
		c.PermVersion = int64(v)
	}

	return c, nil
}
