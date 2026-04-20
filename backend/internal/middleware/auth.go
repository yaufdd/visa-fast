// Package middleware holds HTTP middleware for the admin API.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
)

type ctxKey int

const (
	ctxKeyUserID ctxKey = iota
	ctxKeyOrgID
)

const (
	sessionCookieName = "session"
	sessionTTL        = 30 * 24 * time.Hour
	sessionRefreshIf  = 15 * 24 * time.Hour
)

// UserID returns the authenticated user ID from context.
// Panics if called outside a request that went through RequireAuth.
func UserID(ctx context.Context) string {
	v, ok := ctx.Value(ctxKeyUserID).(string)
	if !ok {
		panic("middleware.UserID called without RequireAuth middleware")
	}
	return v
}

// OrgID returns the authenticated user's org ID from context.
// Panics if called outside a request that went through RequireAuth.
func OrgID(ctx context.Context) string {
	v, ok := ctx.Value(ctxKeyOrgID).(string)
	if !ok {
		panic("middleware.OrgID called without RequireAuth middleware")
	}
	return v
}

// RequireAuth reads the session cookie, looks up the session + user,
// places user_id and org_id in request context, and chains to the next
// handler. Unauthenticated requests get 401.
func RequireAuth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				writeAuthError(w, 401, "not authenticated")
				return
			}

			sess, err := db.LookupSession(r.Context(), pool, cookie.Value)
			if err != nil {
				writeAuthError(w, 500, "db error")
				return
			}
			if sess == nil {
				writeAuthError(w, 401, "session expired")
				return
			}

			extend := time.Until(sess.ExpiresAt) < sessionRefreshIf
			_ = db.TouchSession(r.Context(), pool, cookie.Value, extend, sessionTTL)

			ctx := context.WithValue(r.Context(), ctxKeyUserID, sess.UserID)
			ctx = context.WithValue(ctx, ctxKeyOrgID, sess.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
