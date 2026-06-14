package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/ashok-shasmal/library-portal/internal/database"
)

type ctxKey string

const (
	ctxUserID   ctxKey = "user_id"
	ctxUserRole ctxKey = "user_role"
)

// UserIDFromContext extracts the authenticated user ID from context.
func UserIDFromContext(ctx context.Context) (int, bool) {
	v := ctx.Value(ctxUserID)
	if v == nil {
		return 0, false
	}
	id, ok := v.(int)
	return id, ok
}

// UserRoleFromContext returns the authenticated user's role.
func UserRoleFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(ctxUserRole)
	if v == nil {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}

// Authenticate validates the Authorization header, loads the user, and injects id/role into context.
func Authenticate(store *database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ah := r.Header.Get("Authorization")
			if ah == "" {
				http.Error(w, "missing authorization", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(ah, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}

			token := parts[1]
			uid, err := ValidateToken(token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			u, err := store.GetUserByID(r.Context(), uid)
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if u == nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserID, int(u.Id))
			ctx = context.WithValue(ctx, ctxUserRole, u.GetRole())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole ensures the caller has the specified role.
func RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userRole, ok := UserRoleFromContext(r.Context())
		if !ok || userRole != role {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IsAdmin returns true if the current request's user is an admin.
func IsAdmin(ctx context.Context) bool {
	role, ok := UserRoleFromContext(ctx)
	return ok && role == "ADMIN"
}
