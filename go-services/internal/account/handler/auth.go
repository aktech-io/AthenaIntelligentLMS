package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// PermissionResolver resolves the effective RBAC permissions for a set of roles
// and the current matrix version. Implemented by the account rbac.Store.
type PermissionResolver interface {
	PermissionsForRoles(ctx context.Context, roles []string) (perms []string, version int64, err error)
}

// LmsUser represents an in-memory user for portal authentication.
type LmsUser struct {
	Username string   `json:"username"`
	Password string   `json:"-"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	TenantID string   `json:"tenantId"`
	Roles    []string `json:"roles"`
}

// AuthHandler handles login and JWT token generation.
type AuthHandler struct {
	users     map[string]*LmsUser
	jwtSecret []byte
	perms     PermissionResolver // optional; nil = don't stamp permissions
	logger    *zap.Logger
}

// NewAuthHandler creates an auth handler with default users. perms may be nil,
// in which case tokens carry no permission claims (enforcement falls back to
// role checks).
func NewAuthHandler(base64Secret string, perms PermissionResolver, logger *zap.Logger) (*AuthHandler, error) {
	key, err := base64.StdEncoding.DecodeString(base64Secret)
	if err != nil {
		return nil, fmt.Errorf("decode jwt secret: %w", err)
	}

	tenantID := envOr("LMS_AUTH_TENANT_ID", "admin")

	// SECURITY (CRIT-2): no hardcoded password defaults. Passwords are supplied
	// per account via env (sourced from a k8s Secret in production). Known demo
	// defaults are used ONLY when LMS_AUTH_ALLOW_DEFAULT_PASSWORDS=true is set
	// explicitly (local/dev/CI) — production leaves it unset, so a missing
	// password means that account is not created, and a missing ADMIN password
	// is a fatal misconfiguration rather than a silent "admin/admin123".
	allowDefaults := os.Getenv("LMS_AUTH_ALLOW_DEFAULT_PASSWORDS") == "true"
	pwd := func(envKey, devDefault string) string {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
		if allowDefaults {
			return devDefault
		}
		return ""
	}

	adminPwd := pwd("LMS_AUTH_ADMIN_PASSWORD", "admin123")
	if adminPwd == "" {
		return nil, fmt.Errorf("LMS_AUTH_ADMIN_PASSWORD is required (or set LMS_AUTH_ALLOW_DEFAULT_PASSWORDS=true for dev); refusing to start with a default/empty admin password")
	}
	managerPwd := pwd("LMS_AUTH_MANAGER_PASSWORD", "manager123")
	officerPwd := pwd("LMS_AUTH_OFFICER_PASSWORD", "officer123")
	tellerPwd := pwd("LMS_AUTH_TELLER_PASSWORD", "teller123")

	users := map[string]*LmsUser{}
	addUser := func(usernames []string, password, name, email string, roles []string) {
		if password == "" {
			return // account disabled when no password is configured
		}
		for _, u := range usernames {
			users[u] = &LmsUser{Username: u, Password: password, Name: name, Email: email, TenantID: tenantID, Roles: roles}
		}
	}
	addUser([]string{"admin", "admin@athena.com"}, adminPwd, "System Administrator", "admin@athena.com", []string{"ADMIN", "USER"})
	addUser([]string{"manager", "manager@athena.com"}, managerPwd, "Branch Manager", "manager@athena.com", []string{"MANAGER", "USER"})
	addUser([]string{"officer", "officer@athena.com"}, officerPwd, "Loan Officer", "officer@athena.com", []string{"OFFICER", "USER"})
	addUser([]string{"teller@athena.com"}, tellerPwd, "Senior Teller", "teller@athena.com", []string{"TELLER", "USER"})

	return &AuthHandler{users: users, jwtSecret: key, perms: perms, logger: logger}, nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string   `json:"token"`
	Username  string   `json:"username"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	Roles     []string `json:"roles"`
	TenantID  string   `json:"tenantId"`
	ExpiresIn int64    `json:"expiresIn"`
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	if req.Username == "" || req.Password == "" {
		httputil.WriteBadRequest(w, "Username and password are required", r.URL.Path)
		return
	}

	user, ok := h.users[strings.ToLower(req.Username)]
	if !ok || user.Password != req.Password {
		h.logger.Warn("Failed login attempt", zap.String("username", req.Username))
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "Unauthorized", "Invalid credentials", r.URL.Path)
		return
	}

	token, err := h.generateToken(r.Context(), user)
	if err != nil {
		h.logger.Error("Failed to generate token", zap.Error(err))
		httputil.WriteInternalError(w, "Token generation failed", r.URL.Path)
		return
	}

	h.logger.Info("Successful login", zap.String("username", user.Username))
	httputil.WriteJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		Username:  user.Username,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Roles[0],
		Roles:     user.Roles,
		TenantID:  user.TenantID,
		ExpiresIn: 86400,
	})
}

// Me handles GET /api/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"username":  auth.UserIDFromContext(ctx),
		"tenantId":  auth.TenantIDFromContext(ctx),
		"roles":     auth.RolesFromContext(ctx),
	})
}

func (h *AuthHandler) generateToken(ctx context.Context, user *LmsUser) (string, error) {
	header := base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

	now := time.Now()
	claims := map[string]any{
		"sub":      user.Username,
		"roles":    user.Roles,
		"tenantId": user.TenantID,
		"name":     user.Name,
		"email":    user.Email,
		"iat":      now.Unix(),
		"exp":      now.Add(24 * time.Hour).Unix(),
	}

	// Stamp effective RBAC permissions (+ matrix version) into the token. This
	// is best-effort: if the matrix is unavailable, log and issue a token with
	// no permission claims — enforcement then falls back to role checks, so a
	// matrix outage never blocks login or breaks authorisation.
	if h.perms != nil {
		if perms, version, err := h.perms.PermissionsForRoles(ctx, user.Roles); err != nil {
			h.logger.Warn("RBAC permission resolution failed; issuing token without permissions claim",
				zap.String("user", user.Username), zap.Error(err))
		} else {
			claims["permissions"] = perms
			claims["permVersion"] = version
		}
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64URLEncode(claimsJSON)

	sigInput := header + "." + payload
	mac := hmac.New(sha256.New, h.jwtSecret)
	mac.Write([]byte(sigInput))
	sig := base64URLEncode(mac.Sum(nil))

	return sigInput + "." + sig, nil
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
