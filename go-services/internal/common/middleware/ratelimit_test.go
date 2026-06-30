package middleware

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func newTestLimiter(t *testing.T, cfg RateLimiterConfig) *RateLimiter {
	t.Helper()
	rl := NewRateLimiter(cfg, zap.NewNop())
	t.Cleanup(rl.Stop)
	return rl
}

// Under the burst limit, all requests are allowed.
func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 1, Burst: 5})
	h := rl.Middleware(okHandler())

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/lms/api/v1/accounts/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}
}

// Once the burst is exhausted, the limiter returns 429 with a Retry-After header.
func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 1, Burst: 3})
	h := rl.Middleware(okHandler())

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/lms/api/v1/accounts/", nil)
		req.RemoteAddr = "10.0.0.2:1234"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	for i := 0; i < 3; i++ {
		if rr := send(); rr.Code != http.StatusOK {
			t.Fatalf("burst request %d should pass, got %d", i+1, rr.Code)
		}
	}
	rr := send() // 4th — over burst
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if ra := rr.Header().Get("Retry-After"); ra == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

// Different IPs have independent buckets.
func TestRateLimiter_PerIPIsolation(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 1, Burst: 1})
	h := rl.Middleware(okHandler())

	send := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = ip + ":1000"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if c := send("10.0.0.10"); c != http.StatusOK {
		t.Fatalf("first IP first call should pass, got %d", c)
	}
	if c := send("10.0.0.10"); c != http.StatusTooManyRequests {
		t.Fatalf("first IP second call should be limited, got %d", c)
	}
	if c := send("10.0.0.11"); c != http.StatusOK {
		t.Fatalf("second IP should have its own bucket, got %d", c)
	}
}

// A JWT subject, when present, keys the bucket per-user regardless of IP.
func TestRateLimiter_KeysBySubject(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 1, Burst: 1})
	h := rl.Middleware(okHandler())

	// Minimal unsigned JWT with sub=alice (signature not validated for keying).
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice"}`))
	token := "Bearer " + base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." + payload + ".sig"

	send := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = ip + ":1000"
		req.Header.Set("Authorization", token)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if c := send("10.0.0.20"); c != http.StatusOK {
		t.Fatalf("first call should pass, got %d", c)
	}
	// Same subject, different IP — still the same bucket, so limited.
	if c := send("10.0.0.21"); c != http.StatusTooManyRequests {
		t.Fatalf("same subject from a new IP should share the bucket, got %d", c)
	}
}

// A disabled limiter is a transparent pass-through.
func TestRateLimiter_DisabledPassesThrough(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: false, RPS: 1, Burst: 1})
	h := rl.Middleware(okHandler())
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.0.0.30:1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("disabled limiter should pass all, got %d on %d", rr.Code, i)
		}
	}
}

// The health probe path is never throttled.
func TestRateLimiter_HealthExempt(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 1, Burst: 1})
	h := rl.Middleware(okHandler())
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
		req.RemoteAddr = "10.0.0.40:1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("health must be exempt, got %d", rr.Code)
		}
	}
}

// Tokens refill over time, so a blocked client is allowed again after waiting.
func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 100, Burst: 1})
	h := rl.Middleware(okHandler())
	send := func() int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.0.0.50:1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if c := send(); c != http.StatusOK {
		t.Fatalf("first should pass, got %d", c)
	}
	if c := send(); c != http.StatusTooManyRequests {
		t.Fatalf("second should be limited, got %d", c)
	}
	time.Sleep(20 * time.Millisecond) // at 100 rps, ~2 tokens refill
	if c := send(); c != http.StatusOK {
		t.Fatalf("after refill should pass, got %d", c)
	}
}

// MaxKeys bounds memory: the tracked-key map never exceeds the cap even under a
// flood of distinct IPs.
func TestRateLimiter_BoundedMemory(t *testing.T) {
	rl := newTestLimiter(t, RateLimiterConfig{Enabled: true, RPS: 1, Burst: 1, MaxKeys: 16})
	h := rl.Middleware(okHandler())
	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.1." + itoa(i/256) + "." + itoa(i%256) + ":1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
	}
	rl.mu.Lock()
	n := len(rl.buckets)
	rl.mu.Unlock()
	if n > 16 {
		t.Fatalf("bucket map exceeded MaxKeys: %d > 16", n)
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
