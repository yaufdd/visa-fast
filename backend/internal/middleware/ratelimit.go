package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateEntry tracks attempts for a single key within a window.
type rateEntry struct {
	count int
	reset time.Time
}

// RateLimiter is an in-memory sliding-window-ish limiter keyed by IP.
// Intentionally simple: per-handler instance, no Redis, loses state on restart.
// Fine for MVP.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
}

// NewRateLimiter returns a limiter allowing `limit` requests per `window`.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateEntry),
		limit:   limit,
		window:  window,
	}
	go rl.gc()
	return rl
}

// Middleware is the chi-compatible http.Handler middleware.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "900") // 15 minutes
				w.WriteHeader(429)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":       "too many attempts",
					"retry_after": 900,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	e, ok := rl.entries[key]
	if !ok || now.After(e.reset) {
		rl.entries[key] = &rateEntry{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if e.count >= rl.limit {
		return false
	}
	e.count++
	return true
}

// gc removes expired entries every minute.
func (rl *RateLimiter) gc() {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for range tick.C {
		rl.mu.Lock()
		now := time.Now()
		for k, e := range rl.entries {
			if now.After(e.reset) {
				delete(rl.entries, k)
			}
		}
		rl.mu.Unlock()
	}
}

// ClientIP returns the genuine client IP for rate-limiting and
// captcha-verification purposes.
//
// Behind a trusted reverse-proxy chain (traefik → frontend nginx →
// backend in production) the TCP-level RemoteAddr is the proxy's
// internal docker IP — identical for every visitor and useless as a
// rate-limit key. The outermost trusted proxy populates X-Real-Ip with
// the actual client IP; nginx is configured (see nginx.conf) to forward
// that value through verbatim instead of overwriting it. We trust this
// header because the backend is not exposed outside the docker network
// — every request necessarily passes through traefik first, and traefik
// always replaces any client-supplied X-Real-Ip with the real one it
// observed. RemoteAddr remains the fallback for direct connections
// (tests, local dev without nginx in front).
func ClientIP(r *http.Request) string {
	if real := strings.TrimSpace(r.Header.Get("X-Real-Ip")); real != "" {
		return real
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
