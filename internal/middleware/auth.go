package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/utils"
)

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	UsernameKey contextKey = "username"
	IsAdminKey  contextKey = "is_admin"
)

// AuthMiddleware validates JWT tokens and injects user info into the context.
// It checks Authorization header first, then falls back to access_token cookie.
func AuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""

			// Try Authorization header first
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
					tokenString = parts[1]
				}
			}

			// Fall back to cookie
			if tokenString == "" {
				if c, err := r.Cookie("access_token"); err == nil {
					tokenString = c.Value
				}
			}

			if tokenString == "" {
				utils.WriteError(w, http.StatusUnauthorized, "missing authorization")
				return
			}

			claims, err := jwtManager.ValidateToken(tokenString)
			if err != nil {
				utils.WriteError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UsernameKey, claims.Username)
			ctx = context.WithValue(ctx, IsAdminKey, claims.IsAdmin)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthMiddleware is like AuthMiddleware but doesn't require a token.
func OptionalAuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
					if claims, err := jwtManager.ValidateToken(parts[1]); err == nil {
						ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
						ctx = context.WithValue(ctx, UsernameKey, claims.Username)
						ctx = context.WithValue(ctx, IsAdminKey, claims.IsAdmin)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID extracts the user ID from context.
func GetUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDKey).(int64)
	return id, ok
}

// GetUsername extracts the username from context.
func GetUsername(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(UsernameKey).(string)
	return name, ok
}

// IsAdmin checks if the current user is an admin.
func IsAdmin(ctx context.Context) bool {
	admin, ok := ctx.Value(IsAdminKey).(bool)
	return ok && admin
}
