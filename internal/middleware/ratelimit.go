package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides a simple per-IP sliding-window rate limiter.
// Ponytail: single-node in-memory. For multi-node deployments,
// replace with Redis-based limiter (sliding window via ZSET).
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a rate limiter allowing `limit` requests per `window`.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	// Background cleanup every 10x window
	go func() {
		ticker := time.NewTicker(window * 10)
		defer ticker.Stop()
		for range ticker.C {
			rl.mu.Lock()
			cutoff := time.Now().Add(-window)
			for ip, times := range rl.requests {
				// Filter out old entries
				j := 0
				for _, t := range times {
					if t.After(cutoff) {
						times[j] = t
						j++
					}
				}
				if j == 0 {
					delete(rl.requests, ip)
				} else {
					rl.requests[ip] = times[:j]
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune old entries for this key
	times := rl.requests[key]
	j := 0
	for _, t := range times {
		if t.After(cutoff) {
			times[j] = t
			j++
		}
	}
	times = times[:j]

	if len(times) >= rl.limit {
		return false
	}

	rl.requests[key] = append(times, now)
	return true
}

// HTTPMiddleware returns an HTTP middleware that rate-limits by client IP.
func (rl *RateLimiter) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Use X-Forwarded-For if behind proxy
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		if !rl.Allow(ip) {
			http.Error(w, `{"success":false,"message":"请求过于频繁，请稍后再试"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
