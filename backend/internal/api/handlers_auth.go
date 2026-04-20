package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/auth"
	"fujitravel-admin/backend/internal/db"
)

type authUserResp struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name"`
	Role        string  `json:"role"`
}

type authOrgResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type authMeResp struct {
	User authUserResp `json:"user"`
	Org  authOrgResp  `json:"org"`
}

const sessionCookieName = "session"
const sessionTTL = 30 * 24 * time.Hour

// Register handles POST /api/auth/register.
// Body: {org_name, email, password}
func Register(pool *pgxpool.Pool) http.HandlerFunc {
	type req struct {
		OrgName  string `json:"org_name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}
		body.OrgName = strings.TrimSpace(body.OrgName)
		body.Email = strings.ToLower(strings.TrimSpace(body.Email))

		if body.OrgName == "" {
			writeError(w, 400, "org_name required")
			return
		}
		if _, err := mail.ParseAddress(body.Email); err != nil {
			writeError(w, 400, "invalid email")
			return
		}
		if len(body.Password) < 8 {
			writeError(w, 400, "password too short (min 8)")
			return
		}

		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			slog.Error("hash password", "err", err)
			writeError(w, 500, "internal error")
			return
		}

		// Retry slug generation on unique-violation
		var orgID string
		for attempt := 0; attempt < 5; attempt++ {
			slug, err := auth.NewOrgSlug()
			if err != nil {
				writeError(w, 500, "slug gen error")
				return
			}
			orgID, err = db.CreateOrganization(r.Context(), pool, body.OrgName, slug)
			if err == nil {
				break
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue // slug collision — try again
			}
			slog.Error("create organization", "err", err)
			writeError(w, 500, "db error")
			return
		}
		if orgID == "" {
			writeError(w, 500, "could not allocate slug")
			return
		}

		userID, err := db.CreateUser(r.Context(), pool, orgID, body.Email, hash)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, 409, "email already registered")
				return
			}
			slog.Error("create user", "err", err)
			writeError(w, 500, "db error")
			return
		}

		token, err := auth.NewSessionToken()
		if err != nil {
			writeError(w, 500, "token gen error")
			return
		}
		if err := db.CreateSession(r.Context(), pool, userID, token, sessionTTL); err != nil {
			slog.Error("create session", "err", err)
			writeError(w, 500, "db error")
			return
		}
		setSessionCookie(w, token)

		user, _ := db.GetUserByID(r.Context(), pool, userID)
		org, _ := db.GetOrganizationByID(r.Context(), pool, orgID)
		writeJSON(w, 201, authMeResp{
			User: authUserResp{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role},
			Org:  authOrgResp{ID: org.ID, Name: org.Name, Slug: org.Slug},
		})
	}
}

func setSessionCookie(w http.ResponseWriter, token string) {
	secure := os.Getenv("APP_ENV") == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
