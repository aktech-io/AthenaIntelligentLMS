package middleware

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// RateLimiterConfig configures an IP/subject request rate limiter.
//
// SECURITY (HIGH-3): rate limiting protects against login brute-force and
// volumetric API abuse. The limiter is IP-based by default and, when a JWT
// subject can be derived from the request, per-subject so an authenticated
// caller gets a stable bucket regardless of source IP.
//
// HORIZONTAL-SCALE NOTE: state is held IN-MEMORY and is therefore PER POD /
// PER INSTANCE. With N replicas the effective global limit is ~N×RPS. This is
// acceptable for the current deployment (services run few replicas) but the
// correct path to a true global limit is a shared store (e.g. Redis token
// buckets / a sliding-window counter). Do NOT rely on this for hard global
// quotas across a large replica set.
//
// The token-bucket math and bounded, evicting key registry are hand-rolled on
// the standard library — mirroring the existing hand-rolled CircuitBreaker in
// the gateway — to avoid pulling a new third-party dependency into the vendored
// build for such a small primitive.
type RateLimiterConfig struct {
	// Enabled toggles the limiter. When false the middleware is a pass-through
	// (used by tests and as an operational kill-switch).
	Enabled bool
	// RPS is the sustained requests-per-second allowed per key.
	RPS float64
	// Burst is the maximum burst (bucket size) per key. Must be >= 1.
	Burst int
	// IdleTTL is how long an unused key bucket is retained before eviction.
	IdleTTL time.Duration
	// MaxKeys caps the number of tracked keys to bound memory. When the cap is
	// reached the limiter sweeps idle keys and, if still full, evicts the
	// least-recently-seen key before admitting a new one.
	MaxKeys int
	// TrustProxyHeaders, when true, derives the client IP from the leftmost
	// X-Forwarded-For entry instead of RemoteAddr. Leave false unless the
	// gateway sits behind a trusted proxy/ingress that sets XFF, because XFF is
	// client-spoofable and trusting it would let an attacker bypass per-IP
	// limits by forging the header.
	TrustProxyHeaders bool
}

// bucket is a per-key token bucket plus its last-seen time for eviction.
//
// Token state (tokens, refilledAt) is guarded by mu. lastSeen is touched ONLY
// while the owning RateLimiter holds its registry lock, so the eviction sweep
// can read it without racing the request path.
type bucket struct {
	mu         sync.Mutex
	tokens     float64
	refilledAt time.Time
	lastSeen   time.Time
}

// RateLimiter is a thread-safe, memory-bounded IP/subject rate limiter. One
// token bucket is maintained per key; idle buckets are evicted by a background
// janitor and a hard MaxKeys cap.
type RateLimiter struct {
	cfg     RateLimiterConfig
	logger  *zap.Logger
	mu      sync.Mutex
	buckets map[string]*bucket
	stop    chan struct{}
	stopped bool
}

// NewRateLimiter builds a RateLimiter and starts its background eviction janitor.
// Call Stop to release the janitor goroutine (important in tests).
func NewRateLimiter(cfg RateLimiterConfig, logger *zap.Logger) *RateLimiter {
	if cfg.Burst < 1 {
		cfg.Burst = 1
	}
	if cfg.RPS <= 0 {
		// A non-positive rate would never refill; clamp to a tiny positive rate
		// so the bucket still drains/refills deterministically.
		cfg.RPS = 0.0001
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 10 * time.Minute
	}
	if cfg.MaxKeys < 1 {
		cfg.MaxKeys = 100_000
	}
	rl := &RateLimiter{
		cfg:     cfg,
		logger:  logger,
		buckets: make(map[string]*bucket),
		stop:    make(chan struct{}),
	}
	if cfg.Enabled {
		go rl.janitor()
	}
	return rl
}

// janitor periodically evicts idle buckets to bound memory.
func (rl *RateLimiter) janitor() {
	interval := rl.cfg.IdleTTL
	if interval > time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.evictIdle()
		}
	}
}

// evictIdle removes buckets not seen within IdleTTL.
func (rl *RateLimiter) evictIdle() {
	cutoff := time.Now().Add(-rl.cfg.IdleTTL)
	rl.mu.Lock()
	for k, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
	rl.mu.Unlock()
}

// Stop terminates the background janitor. Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.stopped {
		return
	}
	rl.stopped = true
	close(rl.stop)
}

// getBucket returns the bucket for key, creating it if needed, and refreshes its
// last-seen time. It enforces the MaxKeys cap by sweeping idle keys and, if
// still full, evicting the least-recently-seen key.
func (rl *RateLimiter) getBucket(key string, now time.Time) *bucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if b, ok := rl.buckets[key]; ok {
		b.lastSeen = now
		return b
	}
	if len(rl.buckets) >= rl.cfg.MaxKeys {
		rl.evictLocked(now)
	}
	b := &bucket{tokens: float64(rl.cfg.Burst), refilledAt: now, lastSeen: now}
	rl.buckets[key] = b
	return b
}

// evictLocked makes room for a new key. Caller must hold rl.mu.
func (rl *RateLimiter) evictLocked(now time.Time) {
	cutoff := now.Add(-rl.cfg.IdleTTL)
	for k, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
	if len(rl.buckets) < rl.cfg.MaxKeys {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, b := range rl.buckets {
		if oldestKey == "" || b.lastSeen.Before(oldest) {
			oldestKey, oldest = k, b.lastSeen
		}
	}
	if oldestKey != "" {
		delete(rl.buckets, oldestKey)
	}
}

// allow consumes a token if available. When denied it returns the duration
// until the next token becomes available (for Retry-After).
func (b *bucket) allow(now time.Time, rps float64, burst float64) (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.refilledAt).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(burst, b.tokens+elapsed*rps)
		b.refilledAt = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	needed := 1 - b.tokens
	retry := time.Duration(needed / rps * float64(time.Second))
	if retry < time.Second {
		retry = time.Second
	}
	return false, retry
}

// Middleware returns an http.Handler middleware that enforces the rate limit.
// On exceed it returns HTTP 429 with a Retry-After header (seconds). The
// /actuator/health probe path is always exempt so liveness/readiness is never
// throttled.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/actuator/health" {
			next.ServeHTTP(w, r)
			return
		}

		now := time.Now()
		b := rl.getBucket(rl.key(r), now)
		ok, retry := b.allow(now, rl.cfg.RPS, float64(rl.cfg.Burst))
		if !ok {
			secs := int(math.Ceil(retry.Seconds()))
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			rl.logger.Warn("rate limit exceeded",
				zap.String("path", r.URL.Path),
				zap.Int("retryAfterSeconds", secs),
			)
			httputil.WriteTooManyRequests(w, "Rate limit exceeded. Please retry later.", r.URL.Path)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// key derives the rate-limit bucket key: the JWT subject when one can be
// extracted (authenticated callers get a stable per-user bucket), else the
// client IP.
func (rl *RateLimiter) key(r *http.Request) string {
	if sub := subjectFromBearer(r.Header.Get("Authorization")); sub != "" {
		return "sub:" + sub
	}
	return "ip:" + rl.clientIP(r)
}

// clientIP extracts the best-available client IP for keying.
func (rl *RateLimiter) clientIP(r *http.Request) string {
	if rl.cfg.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Leftmost entry is the originating client (per proxy convention).
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// subjectFromBearer extracts the "sub" claim from a Bearer JWT WITHOUT verifying
// the signature. This is intentional and safe for rate-limit bucketing ONLY:
// the value merely selects a counter, so a forged subject just rate-limits the
// forger. It is NEVER used for authentication or authorization.
func subjectFromBearer(authHeader string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimSpace(authHeader[len(prefix):]), ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Sub
}
