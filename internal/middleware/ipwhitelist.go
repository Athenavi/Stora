package middleware

import (
	"net/http"
	"strings"
)

// IPWhitelistMiddleware restricts access to specific IPs.
// Whitelist is read from system_settings table each request.
// Settings key: admin_ip_whitelist (comma-separated IPs)
type IPWhitelistMiddleware struct {
	GetWhitelist func() []string
}

func NewIPWhitelistMiddleware(getWhitelist func() []string) *IPWhitelistMiddleware {
	return &IPWhitelistMiddleware{GetWhitelist: getWhitelist}
}

func (m *IPWhitelistMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		whitelist := m.GetWhitelist()
		if len(whitelist) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		clientIP := r.RemoteAddr
		if idx := strings.LastIndex(clientIP, ":"); idx > 0 {
			clientIP = clientIP[:idx]
		}
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			clientIP = strings.Split(fwd, ",")[0]
		}
		for _, allowed := range whitelist {
			if clientIP == allowed {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"success":false,"message":"access denied"}`, http.StatusForbidden)
	})
}
