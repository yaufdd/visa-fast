package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/consent"
	"fujitravel-admin/backend/internal/db"
)

// PublicOrg handles GET /api/public/org/:slug. Returns minimal org info
// (name only — do not leak id/email/created_at).
func PublicOrg(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
		if err != nil {
			slog.Error("public org lookup", "err", err)
			writeError(w, 500, "db")
			return
		}
		if org == nil {
			writeError(w, 404, "form not found")
			return
		}
		writeJSON(w, 200, map[string]string{"name": org.Name})
	}
}

// PublicSubmit handles POST /api/public/submissions/:slug.
// Unauthenticated. Resolves slug → org_id and stores the submission.
func PublicSubmit(pool *pgxpool.Pool) http.HandlerFunc {
	type req struct {
		Payload         map[string]any `json:"payload"`
		ConsentAccepted bool           `json:"consent_accepted"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
		if err != nil {
			writeError(w, 500, "db")
			return
		}
		if org == nil {
			writeError(w, 404, "form not found")
			return
		}

		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}
		if !body.ConsentAccepted {
			writeError(w, 400, "consent not accepted")
			return
		}

		var missing []string
		for _, k := range requiredPayloadKeys {
			v, ok := body.Payload[k].(string)
			if !ok || v == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			writeErrorWithDetails(w, 400, "missing fields", map[string]any{"missing": missing})
			return
		}

		payloadBytes, _ := json.Marshal(body.Payload)
		agreement := consent.Current()

		id, err := db.CreateSubmissionForOrg(r.Context(), pool, org.ID, payloadBytes, agreement.Version, "tourist")
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, 409, "duplicate submission")
				return
			}
			slog.Error("create public submission", "err", err)
			writeError(w, 500, "db")
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	}
}
