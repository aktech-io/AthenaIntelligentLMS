package handler

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// loginThrottleConfig configures per-key login failure tracking and lockout.
//
// SECURITY (HIGH-3): after MaxFailures failed attempts within FailureWindow a
// key is locked for LockoutDuration. Keys are both the attempted username and
// the client IP, so a targeted-username attack and a single-source spray are
// both throttled. Memory is bounded by MaxKeys with idle eviction.
//
// HORIZONTAL-SCALE NOTE: this counter is IN-MEMORY and therefore PER POD /
// PER INSTANCE. With N account-service replicas an attacker effectively gets
// N×MaxFailures before a global lockout. This is acceptable for the current
// low replica count; a shared store (Redis) is the path to a global lockout.
type loginThrottleConfig struct {
	Enabled         bool
	MaxFailures     int
	FailureWindow   time.Duration
	LockoutDuration time.Duration
	MaxKeys         int
}

// loginThrottleConfigFromEnv builds config from env with secure defaults:
// 5 failures within 15 min → 15 min lockout. Disable with
// LMS_LOGIN_LOCKOUT_ENABLED=false (tests / kill-switch).
func loginThrottleConfigFromEnv() loginThrottleConfig {
	enabled := true
	if v := os.Getenv("LMS_LOGIN_LOCKOUT_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			enabled = b
		}
	}
	return loginThrottleConfig{
		Enabled:         enabled,
		MaxFailures:     envInt("LMS_LOGIN_MAX_FAILURES", 5),
		FailureWindow:   time.Duration(envInt("LMS_LOGIN_FAILURE_WINDOW_SECONDS", 900)) * time.Second,
		LockoutDuration: time.Duration(envInt("LMS_LOGIN_LOCKOUT_SECONDS", 900)) * time.Second,
		MaxKeys:         envInt("LMS_LOGIN_LOCKOUT_MAX_KEYS", 100_000),
	}
}

// throttleEntry tracks failures for a single key.
type throttleEntry struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
	lastSeen    time.Time
}

// loginThrottle is a thread-safe, bounded failed-login tracker with lockout.
type loginThrottle struct {
	cfg     loginThrottleConfig
	mu      sync.Mutex
	entries map[string]*throttleEntry
	now     func() time.Time // injectable clock for tests
}

// newLoginThrottle creates a throttle with the given config.
func newLoginThrottle(cfg loginThrottleConfig) *loginThrottle {
	if cfg.MaxFailures < 1 {
		cfg.MaxFailures = 5
	}
	if cfg.FailureWindow <= 0 {
		cfg.FailureWindow = 15 * time.Minute
	}
	if cfg.LockoutDuration <= 0 {
		cfg.LockoutDuration = 15 * time.Minute
	}
	if cfg.MaxKeys < 1 {
		cfg.MaxKeys = 100_000
	}
	return &loginThrottle{
		cfg:     cfg,
		entries: make(map[string]*throttleEntry),
		now:     time.Now,
	}
}

// Locked reports whether ANY of the given keys is currently locked out. When
// locked it returns the duration until the earliest unlock (for Retry-After).
// The response is intentionally identical regardless of which key (or whether a
// username exists), to avoid user enumeration.
func (t *loginThrottle) Locked(keys ...string) (bool, time.Duration) {
	if !t.cfg.Enabled {
		return false, 0
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	locked := false
	var until time.Time
	for _, k := range keys {
		e, ok := t.entries[k]
		if !ok {
			continue
		}
		if now.Before(e.lockedUntil) {
			if !locked || e.lockedUntil.Before(until) {
				until = e.lockedUntil
			}
			locked = true
		}
	}
	if !locked {
		return false, 0
	}
	return true, until.Sub(now)
}

// RecordFailure increments the failure counter for each key and locks the key
// once MaxFailures is reached within FailureWindow.
func (t *loginThrottle) RecordFailure(keys ...string) {
	if !t.cfg.Enabled {
		return
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, k := range keys {
		e, ok := t.entries[k]
		if !ok {
			if len(t.entries) >= t.cfg.MaxKeys {
				t.evictLocked(now)
			}
			e = &throttleEntry{windowStart: now}
			t.entries[k] = e
		}
		// Reset the counting window if it has elapsed (and we are not locked).
		if now.Sub(e.windowStart) > t.cfg.FailureWindow && now.After(e.lockedUntil) {
			e.failures = 0
			e.windowStart = now
		}
		e.failures++
		e.lastSeen = now
		if e.failures >= t.cfg.MaxFailures {
			e.lockedUntil = now.Add(t.cfg.LockoutDuration)
		}
	}
}

// Reset clears the failure/lock state for the given keys (called on successful
// login so a legitimate user is never penalised for past failures).
func (t *loginThrottle) Reset(keys ...string) {
	if !t.cfg.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range keys {
		delete(t.entries, k)
	}
}

// evictLocked drops stale entries to bound memory. Caller must hold t.mu. An
// entry is stale once it is past its lockout and its counting window. If none
// are stale, the least-recently-seen entry is evicted.
func (t *loginThrottle) evictLocked(now time.Time) {
	for k, e := range t.entries {
		if now.After(e.lockedUntil) && now.Sub(e.lastSeen) > t.cfg.FailureWindow {
			delete(t.entries, k)
		}
	}
	if len(t.entries) < t.cfg.MaxKeys {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range t.entries {
		if oldestKey == "" || e.lastSeen.Before(oldest) {
			oldestKey, oldest = k, e.lastSeen
		}
	}
	if oldestKey != "" {
		delete(t.entries, oldestKey)
	}
}
