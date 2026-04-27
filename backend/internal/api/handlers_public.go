package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
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
//
// Two modes:
//
//   - submission_id absent: legacy behaviour, INSERT a fresh 'pending' row
//     via CreateSubmissionForOrg.
//   - submission_id present: finalize an existing 'draft' row (created by
//     /start so the tourist could attach scans first). The draft is flipped
//     to 'pending' with the final payload; if the row doesn't exist, isn't
//     in this org, or isn't in 'draft' status, we return 404 — same code
//     used elsewhere for cross-tenant access to avoid enumeration leaks.
//
// Response shape is identical in both modes ({"id": "..."}) so the
// frontend doesn't have to branch on which path it took.
func PublicSubmit(pool *pgxpool.Pool) http.HandlerFunc {
	type req struct {
		Payload         map[string]any `json:"payload"`
		ConsentAccepted bool           `json:"consent_accepted"`
		SubmissionID    string         `json:"submission_id,omitempty"`
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

		if body.SubmissionID != "" {
			// Finalize-an-existing-draft path.
			err := db.UpdateSubmissionPayloadByID(r.Context(), pool, org.ID, body.SubmissionID, payloadBytes, agreement.Version)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeError(w, 404, "submission not found")
					return
				}
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					// Dedup index fires when flipping draft → pending and a
					// pending row already exists for this passport today.
					writeError(w, 409, "duplicate submission")
					return
				}
				slog.Error("finalize public submission", "err", err)
				writeError(w, 500, "db")
				return
			}
			writeJSON(w, 201, map[string]string{"id": body.SubmissionID})
			return
		}

		// Legacy "create new pending row" path.
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
