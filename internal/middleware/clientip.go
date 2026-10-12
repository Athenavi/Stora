package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// trustedIPHeader holds the request header that is allowed to override the
// client address (e.g. "X-Real-IP" when the app sits behind nginx configured
// with `proxy_set_header X-Real-IP $remote_addr`). Empty by default: forwarding
// headers are client-controlled, so they are ignored unless an operator opts in.
var trustedIPHeader atomic.Value // string

// SetTrustedIPHeader configures which header carries the real client IP.
// Call once during startup. An empty name means "trust nothing but the TCP peer".
func SetTrustedIPHeader(name string) {
	trustedIPHeader.Store(strings.TrimSpace(name))
}

// ClientIP returns the caller's address for rate limiting, audit logs and IP
// allow-lists. It honours a header only when one was explicitly trusted via
// SetTrustedIPHeader; otherwise the TCP peer address is authoritative.
//
// Reading X-Forwarded-For unconditionally (the previous behaviour) let anyone
// bypass per-IP rate limits by rotating a forged header value.
func ClientIP(r *http.Request) string {
	if h, _ := trustedIPHeader.Load().(string); h != "" {
		if v := r.Header.Get(h); v != "" {
			// X-Forwarded-For style lists put the client first.
			if idx := strings.IndexByte(v, ','); idx >= 0 {
				v = v[:idx]
			}
			if ip := strings.TrimSpace(v); ip != "" {
				return ip
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
