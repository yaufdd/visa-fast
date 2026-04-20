package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/consent"
)

// TouristSubmission mirrors the DB row.
type TouristSubmission struct {
	ID                string          `json:"id"`
	Payload           json.RawMessage `json:"payload"`
	ConsentAccepted   bool            `json:"consent_accepted"`
	ConsentAcceptedAt time.Time       `json:"consent_accepted_at"`
	ConsentVersion    string          `json:"consent_version"`
	Source            string          `json:"source"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// Required top-level keys in payload for a submission to be considered valid.
var requiredPayloadKeys = []string{
	"name_lat", "name_cyr", "gender_ru", "birth_date",
	"passport_number", "issue_date", "expiry_date",
	"internal_series", "internal_number",
	"phone", "home_address_ru",
}

// CreateSubmission handles POST /api/submissions (public, no auth).
func CreateSubmission(db *pgxpool.Pool) http.HandlerFunc {
	type reqBody struct {
		Payload         map[string]any `json:"payload"`
		ConsentAccepted bool           `json:"consent_accepted"`
		Source          string         `json:"source"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !body.ConsentAccepted {
			writeError(w, http.StatusBadRequest, "consent not accepted")
			return
		}
		if body.Source != "tourist" && body.Source != "manager" {
			body.Source = "tourist"
		}

		var missing []string
		for _, k := range requiredPayloadKeys {
			v, ok := body.Payload[k].(string)
			if !ok || v == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			writeErrorWithDetails(w, http.StatusBadRequest, "missing fields", map[string]any{
				"missing": missing,
			})
			return
		}

		payloadBytes, _ := json.Marshal(body.Payload)
		agreement := consent.Current()

		var id string
		err := db.QueryRow(r.Context(),
			`INSERT INTO tourist_submissions
			   (payload, consent_accepted, consent_accepted_at, consent_version, source)
			 VALUES ($1, TRUE, NOW(), $2, $3)
			 RETURNING id`,
			payloadBytes, agreement.Version, body.Source).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, http.StatusConflict, "duplicate submission (same passport, same day)")
				return
			}
			slog.Error("create submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// writeErrorWithDetails is a helper that wraps writeError with extra fields.
func writeErrorWithDetails(w http.ResponseWriter, status int, msg string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]any{"error": msg}
	for k, v := range details {
		payload[k] = v
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ListSubmissions handles GET /api/submissions?q=&status=
func ListSubmissions(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		status := r.URL.Query().Get("status")

		args := []any{}
		where := []string{}
		if q != "" {
			args = append(args, "%"+q+"%")
			where = append(where, fmt.Sprintf("payload ->> 'name_lat' ILIKE $%d", len(args)))
		}
		if status != "" {
			args = append(args, status)
			where = append(where, fmt.Sprintf("status = $%d", len(args)))
		}

		sql := `SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
			           source, status, created_at, updated_at
			      FROM tourist_submissions`
		if len(where) > 0 {
			sql += " WHERE " + strings.Join(where, " AND ")
		}
		sql += " ORDER BY created_at DESC LIMIT 500"

		rows, err := db.Query(r.Context(), sql, args...)
		if err != nil {
			slog.Error("list submissions", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		out := []TouristSubmission{}
		for rows.Next() {
			var s TouristSubmission
			var payload []byte
			if err := rows.Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
				&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
				slog.Error("scan submission", "err", err)
				continue
			}
			s.Payload = json.RawMessage(payload)
			out = append(out, s)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetSubmission handles GET /api/submissions/:id
func GetSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var s TouristSubmission
		var payload []byte
		err := db.QueryRow(r.Context(),
			`SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
			        source, status, created_at, updated_at
			   FROM tourist_submissions WHERE id = $1`, id).
			Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
				&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt)
		if isNoRows(err) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		if err != nil {
			slog.Error("get submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		s.Payload = json.RawMessage(payload)
		writeJSON(w, http.StatusOK, s)
	}
}

// UpdateSubmission handles PUT /api/submissions/:id — manager edits payload.
func UpdateSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		payloadBytes, _ := json.Marshal(body.Payload)
		tag, err := db.Exec(r.Context(),
			`UPDATE tourist_submissions
			     SET payload = $1, updated_at = NOW()
			   WHERE id = $2`, payloadBytes, id)
		if err != nil {
			slog.Error("update submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ArchiveSubmission handles DELETE /api/submissions/:id — soft archive.
func ArchiveSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tag, err := db.Exec(r.Context(),
			`UPDATE tourist_submissions SET status = 'archived', updated_at = NOW()
			    WHERE id = $1 AND status != 'archived'`, id)
		if err != nil {
			slog.Error("archive submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "not found or already archived")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// EraseSubmission handles DELETE /api/submissions/:id/erase — hard delete.
// Clears submission_snapshot on attached tourists in the same transaction.
func EraseSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ctx := r.Context()
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx begin")
			return
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		if _, err := tx.Exec(ctx,
			`UPDATE tourists SET submission_snapshot = NULL, submission_id = NULL
			    WHERE submission_id = $1`, id); err != nil {
			slog.Error("erase tourists snapshot", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		tag, err := tx.Exec(ctx, `DELETE FROM tourist_submissions WHERE id = $1`, id)
		if err != nil {
			slog.Error("delete submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx commit")
			return
		}
		slog.Info("submission erased", "id", id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// AttachSubmission handles POST /api/submissions/:id/attach
// Body: { "group_id": "...", "subgroup_id": "..." | null }
// Transaction + row lock protects against concurrent attach.
func AttachSubmission(db *pgxpool.Pool) http.HandlerFunc {
	type reqBody struct {
		GroupID    string  `json:"group_id"`
		SubgroupID *string `json:"subgroup_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		submissionID := chi.URLParam(r, "id")
		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupID == "" {
			writeError(w, http.StatusBadRequest, "group_id required")
			return
		}

		ctx := r.Context()
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx begin")
			return
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		var payload []byte
		var status string
		err = tx.QueryRow(ctx,
			`SELECT payload, status FROM tourist_submissions
			    WHERE id = $1 FOR UPDATE`, submissionID).Scan(&payload, &status)
		if isNoRows(err) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "select for update")
			return
		}
		if status == "attached" {
			writeError(w, http.StatusConflict, "submission already attached")
			return
		}

		var touristID string
		err = tx.QueryRow(ctx,
			`INSERT INTO tourists (group_id, subgroup_id, submission_id, submission_snapshot)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			body.GroupID, body.SubgroupID, submissionID, payload).Scan(&touristID)
		if err != nil {
			slog.Error("insert tourist on attach", "err", err)
			writeError(w, http.StatusInternalServerError, "insert tourist")
			return
		}
		if _, err := tx.Exec(ctx,
			`UPDATE tourist_submissions SET status = 'attached', updated_at = NOW() WHERE id = $1`,
			submissionID); err != nil {
			writeError(w, http.StatusInternalServerError, "update submission status")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx commit")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"tourist_id": touristID})
	}
}

// GetConsentText returns the current consent agreement text + version.
func GetConsentText() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := consent.Current()
		writeJSON(w, http.StatusOK, map[string]string{
			"version": a.Version,
			"body":    a.Body,
		})
	}
}
