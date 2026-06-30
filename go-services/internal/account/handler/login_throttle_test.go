package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func testThrottle(maxFail int) *loginThrottle {
	return newLoginThrottle(loginThrottleConfig{
		Enabled:         true,
		MaxFailures:     maxFail,
		FailureWindow:   15 * time.Minute,
		LockoutDuration: 15 * time.Minute,
		MaxKeys:         1000,
	})
}

// After MaxFailures, the key is locked and Locked reports a positive retry.
func TestLoginThrottle_LocksAfterMaxFailures(t *testing.T) {
	th := testThrottle(3)
	key := "u:admin"

	for i := 0; i < 2; i++ {
		th.RecordFailure(key)
		if locked, _ := th.Locked(key); locked {
			t.Fatalf("should not be locked after %d failures", i+1)
		}
	}
	th.RecordFailure(key) // 3rd
	locked, retry := th.Locked(key)
	if !locked {
		t.Fatal("expected lockout after 3 failures")
	}
	if retry <= 0 {
		t.Fatalf("expected positive retry duration, got %v", retry)
	}
}

// A successful login (Reset) clears accumulated failures.
func TestLoginThrottle_ResetClearsState(t *testing.T) {
	th := testThrottle(3)
	key := "u:admin"
	th.RecordFailure(key)
	th.RecordFailure(key)
	th.Reset(key)
	th.RecordFailure(key)
	th.RecordFailure(key)
	if locked, _ := th.Locked(key); locked {
		t.Fatal("reset should have cleared the counter; not locked after 2 fresh failures")
	}
}

// Lockout expires after LockoutDuration (verified via injected clock).
func TestLoginThrottle_LockoutExpires(t *testing.T) {
	th := newLoginThrottle(loginThrottleConfig{
		Enabled: true, MaxFailures: 1, FailureWindow: time.Minute,
		LockoutDuration: time.Minute, MaxKeys: 10,
	})
	base := time.Now()
	th.now = func() time.Time { return base }
	th.RecordFailure("u:x")
	if locked, _ := th.Locked("u:x"); !locked {
		t.Fatal("should be locked")
	}
	th.now = func() time.Time { return base.Add(2 * time.Minute) }
	if locked, _ := th.Locked("u:x"); locked {
		t.Fatal("lockout should have expired")
	}
}

// A disabled throttle never locks.
func TestLoginThrottle_Disabled(t *testing.T) {
	th := newLoginThrottle(loginThrottleConfig{Enabled: false, MaxFailures: 1})
	for i := 0; i < 100; i++ {
		th.RecordFailure("u:x")
	}
	if locked, _ := th.Locked("u:x"); locked {
		t.Fatal("disabled throttle must never lock")
	}
}

// Memory stays bounded under a flood of distinct keys.
func TestLoginThrottle_BoundedMemory(t *testing.T) {
	th := newLoginThrottle(loginThrottleConfig{
		Enabled: true, MaxFailures: 5, FailureWindow: time.Minute,
		LockoutDuration: time.Minute, MaxKeys: 16,
	})
	for i := 0; i < 500; i++ {
		th.RecordFailure("ip:" + itoa(i))
	}
	th.mu.Lock()
	n := len(th.entries)
	th.mu.Unlock()
	if n > 16 {
		t.Fatalf("entries exceeded MaxKeys: %d > 16", n)
	}
}

// ---------------------------------------------------------------------------
// Login handler integration: generic lockout response, no username leak, reset.
// ---------------------------------------------------------------------------

func newTestAuthHandler(t *testing.T, logger *zap.Logger, maxFail int) *AuthHandler {
	t.Helper()
	t.Setenv("LMS_AUTH_ALLOW_DEFAULT_PASSWORDS", "")
	t.Setenv("LMS_AUTH_ADMIN_PASSWORD", "Str0ng-P@ss")
	t.Setenv("LMS_AUTH_MANAGER_PASSWORD", "")
	t.Setenv("LMS_AUTH_OFFICER_PASSWORD", "")
	t.Setenv("LMS_AUTH_TELLER_PASSWORD", "")
	h, err := NewAuthHandler(testSecret(), nil, logger)
	if err != nil {
		t.Fatalf("NewAuthHandler: %v", err)
	}
	// Replace the env-built throttle with a deterministic one.
	h.throttle = testThrottle(maxFail)
	return h
}

func doLogin(h *AuthHandler, username, password, ip string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(loginRequest{Username: username, Password: password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.RemoteAddr = ip + ":12345"
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	return rr
}

// After N failed logins the handler returns a generic 429 with Retry-After, and
// the same response is returned for a non-existent user (no enumeration).
func TestLogin_LockoutAfterFailures(t *testing.T) {
	h := newTestAuthHandler(t, zap.NewNop(), 3)

	for i := 0; i < 3; i++ {
		rr := doLogin(h, "admin", "wrong", "1.2.3.4")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}
	// 4th attempt — now locked (by username key) → 429 generic.
	rr := doLogin(h, "admin", "wrong", "1.2.3.4")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after lockout, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
	// Even the correct password is refused while locked (fail closed).
	rr = doLogin(h, "admin", "Str0ng-P@ss", "1.2.3.4")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("locked account must reject even correct password, got %d", rr.Code)
	}
}

// A successful login resets the failure counter.
func TestLogin_SuccessResetsCounter(t *testing.T) {
	h := newTestAuthHandler(t, zap.NewNop(), 3)
	// Two failures, then a success.
	doLogin(h, "admin", "wrong", "5.6.7.8")
	doLogin(h, "admin", "wrong", "5.6.7.8")
	rr := doLogin(h, "admin", "Str0ng-P@ss", "5.6.7.8")
	if rr.Code != http.StatusOK {
		t.Fatalf("valid login should succeed, got %d", rr.Code)
	}
	// Two more failures should NOT lock (counter was reset).
	doLogin(h, "admin", "wrong", "5.6.7.8")
	rr = doLogin(h, "admin", "wrong", "5.6.7.8")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("counter should have reset; expected 401, got %d", rr.Code)
	}
}

// LOW-5: the raw attempted username must never appear in logs on failure.
func TestLogin_DoesNotLogRawUsername(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	h := newTestAuthHandler(t, zap.New(core), 5)

	const secretUser = "victim-username-12345"
	doLogin(h, secretUser, "wrong", "9.9.9.9")

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected a warn log on failed login")
	}
	for _, e := range entries {
		if strings.Contains(e.Message, secretUser) {
			t.Fatalf("log message leaked raw username: %q", e.Message)
		}
		for k, v := range e.ContextMap() {
			if s, ok := v.(string); ok && strings.Contains(s, secretUser) {
				t.Fatalf("log field %q leaked raw username: %q", k, s)
			}
			// The fingerprint field must not equal the raw username either.
			if k == "username" {
				t.Fatalf("failed-login log must not include a raw 'username' field")
			}
		}
	}
}

// A non-existent user yields the same 401 shape as a wrong password (no
// enumeration signal).
func TestLogin_NoUserEnumeration(t *testing.T) {
	h := newTestAuthHandler(t, zap.NewNop(), 5)
	rrMissing := doLogin(h, "does-not-exist", "whatever", "3.3.3.3")
	rrWrong := doLogin(h, "admin", "wrong", "4.4.4.4")
	if rrMissing.Code != rrWrong.Code {
		t.Fatalf("enumeration: missing-user=%d wrong-pass=%d", rrMissing.Code, rrWrong.Code)
	}
	var a, b map[string]any
	json.Unmarshal(rrMissing.Body.Bytes(), &a)
	json.Unmarshal(rrWrong.Body.Bytes(), &b)
	if a["message"] != b["message"] {
		t.Fatalf("enumeration via message: %q vs %q", a["message"], b["message"])
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
