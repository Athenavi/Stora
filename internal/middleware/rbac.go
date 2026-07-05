package middleware

import (
	"context"
	"net/http"

	"github.com/Athenavi/Stora/pkg/database"
	"github.com/Athenavi/Stora/pkg/utils"
)

type roleContextKey string

const (
	RoleKey roleContextKey = "role"
)

// RequireAdmin rejects non-admin users.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			utils.WriteError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission checks that the authenticated user's roles include the
// given capability (via role_capabilities + capabilities tables).
// Admin always passes. Returns 403 if the capability is missing.
func RequirePermission(capability string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin always passes
			if IsAdmin(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			userID, ok := GetUserID(r.Context())
			if !ok {
				utils.WriteError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if database.DB == nil {
				http.Error(w, `{"error":"internal: database not available"}`, http.StatusInternalServerError)
				return
			}
			var count int
			err := database.DB.QueryRow(
				`SELECT COUNT(*) FROM user_role_assignments ura
				 JOIN role_capabilities rc ON rc.role_id = ura.role_id
				 JOIN capabilities c ON c.id = rc.capability_id
				 WHERE ura.user_id = $1 AND c.codename = $2`,
				userID, capability,
			).Scan(&count)
			if err != nil || count == 0 {
				utils.WriteError(w, http.StatusForbidden, "forbidden: missing capability")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetRole extracts the role from context (stub).
func GetRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(RoleKey).(string)
	return role, ok
}
