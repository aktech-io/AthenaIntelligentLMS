package handler

import (
	"encoding/base64"
	"testing"

	"go.uber.org/zap"
)

// a valid 32+ byte base64 secret for NewJWTUtil/NewAuthHandler.
func testSecret() string {
	return base64.StdEncoding.EncodeToString([]byte("test-secret-key-that-is-long-enough-32+"))
}

// CRIT-2: with no admin password env and defaults NOT allowed, the handler must
// refuse to start rather than silently use admin/admin123.
func TestNewAuthHandler_FailsClosedWithoutCreds(t *testing.T) {
	t.Setenv("LMS_AUTH_ALLOW_DEFAULT_PASSWORDS", "")
	t.Setenv("LMS_AUTH_ADMIN_PASSWORD", "")

	if _, err := NewAuthHandler(testSecret(), nil, zap.NewNop()); err == nil {
		t.Fatal("expected NewAuthHandler to fail when no admin password is configured and defaults are not allowed")
	}
}

// With an explicit admin password (no dev flag), admin logs in with it and the
// demo default is NOT accepted.
func TestNewAuthHandler_UsesEnvPassword(t *testing.T) {
	t.Setenv("LMS_AUTH_ALLOW_DEFAULT_PASSWORDS", "")
	t.Setenv("LMS_AUTH_ADMIN_PASSWORD", "Str0ng-P@ss")
	t.Setenv("LMS_AUTH_MANAGER_PASSWORD", "")
	t.Setenv("LMS_AUTH_OFFICER_PASSWORD", "")
	t.Setenv("LMS_AUTH_TELLER_PASSWORD", "")

	h, err := NewAuthHandler(testSecret(), nil, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u := h.users["admin"]; u == nil || u.Password != "Str0ng-P@ss" {
		t.Fatalf("admin should use the env password")
	}
	if u := h.users["admin"]; u != nil && u.Password == "admin123" {
		t.Fatal("admin must not accept the demo default when defaults are disallowed")
	}
	// Optional accounts with no configured password must not exist.
	if _, ok := h.users["manager"]; ok {
		t.Fatal("manager must not be created without a configured password")
	}
	if _, ok := h.users["teller@athena.com"]; ok {
		t.Fatal("teller must not be created without a configured password")
	}
}

// Dev opt-in: demo defaults are available only when explicitly enabled.
func TestNewAuthHandler_DevDefaultsOptIn(t *testing.T) {
	t.Setenv("LMS_AUTH_ALLOW_DEFAULT_PASSWORDS", "true")
	t.Setenv("LMS_AUTH_ADMIN_PASSWORD", "")
	t.Setenv("LMS_AUTH_MANAGER_PASSWORD", "")
	t.Setenv("LMS_AUTH_OFFICER_PASSWORD", "")
	t.Setenv("LMS_AUTH_TELLER_PASSWORD", "")

	h, err := NewAuthHandler(testSecret(), nil, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u := h.users["admin"]; u == nil || u.Password != "admin123" {
		t.Fatal("dev opt-in should provide the admin demo default")
	}
	if u := h.users["teller@athena.com"]; u == nil || u.Password != "teller123" {
		t.Fatal("dev opt-in should provide the teller demo default")
	}
}
