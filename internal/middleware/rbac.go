package middleware

import (
	"context"
	"net/http"
)

type roleContextKey string

const (
	RoleKey roleContextKey = "role"
)

// RequireAdmin rejects non-admin users.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			http.Error(w, `{"error":"forbidden: admin role required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission checks for specific capabilities (stub for now).
// In production, this would query the role_capabilities table.
func RequirePermission(capability string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin always passes
			if IsAdmin(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			// TODO: query role_capabilities table for the user's roles
			http.Error(w, `{"error":"forbidden: missing capability `+capability+`"}`, http.StatusForbidden)
		})
	}
}

// GetRole extracts the role from context (stub).
func GetRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(RoleKey).(string)
	return role, ok
}
