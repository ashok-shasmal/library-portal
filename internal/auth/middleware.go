package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey string

const ctxUserID ctxKey = "user_id"

// UserIDFromContext extracts the authenticated user ID from context.
func UserIDFromContext(ctx context.Context) (int, bool) {
	v := ctx.Value(ctxUserID)
	if v == nil {
		return 0, false
	}
	id, ok := v.(int)
	return id, ok
}

// JWTMiddleware validates the Authorization header and injects user id into context.
func JWTMiddleware(next http.Handler) http.Handler {
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

		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
