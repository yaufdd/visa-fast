package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/consent"
	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
	"fujitravel-admin/backend/internal/storage"
)

// copyAttachedSubmissionDocs duplicates ticket/voucher rows from submission_files
// into the tourist-level uploads table after a submission is attached. Bytes
// are physically copied so each tourist owns its own file (independent of the
// submission's lifecycle: archive/erase/replace won't strip docs from an
// already-attached tourist). Best-effort — errors are logged but the attach
// still succeeds.
func copyAttachedSubmissionDocs(ctx context.Context, pool *pgxpool.Pool, uploadsDir, orgID, submissionID, touristID, groupID string) {
	rows, err := pool.Query(ctx,
		`SELECT file_type, file_path, original_name
		   FROM submission_files
		  WHERE submission_id = $1 AND org_id = $2
		    AND file_type IN ('ticket','voucher')`,
		submissionID, orgID,
	)
	if err != nil {
		slog.Warn("copy submission docs: query", "submission_id", submissionID, "err", err)
		return
	}
	defer rows.Close()

	type item struct{ fileType, filePath, originalName string }
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.fileType, &it.filePath, &it.originalName); err != nil {
			slog.Warn("copy submission docs: scan", "err", err)
			return
		}
		items = append(items, it)
	}
	rows.Close()

	for _, it := range items {
		data, err := storage.ReadFile(it.filePath)
		if err != nil {
			slog.Warn("copy submission docs: read",
				"submission_id", submissionID, "file_type", it.fileType, "err", err)
			continue
		}
		savedPath, err := storage.SaveFileBytes(uploadsDir, groupID, it.fileType, it.originalName, data)
		if err != nil {
			slog.Warn("copy submission docs: save",
				"tourist_id", touristID, "file_type", it.fileType, "err", err)
			continue
		}
		tid := touristID
		if _, err := db.InsertUpload(ctx, pool, orgID, groupID, &tid, it.fileType, savedPath); err != nil {
			slog.Warn("copy submission docs: insert",
				"tourist_id", touristID, "file_type", it.fileType, "err", err)
			continue
		}
	}
}

// Required top-level keys in payload for a submission to be considered valid.
//
// internal_series / internal_number used to be required, but the public form
// no longer asks the tourist for those — the manager fills them later from
// the uploaded internal-passport scan (manually or via the magic-button
// recognition). Keeping them out of this list lets the tourist submit
// without ever typing internal-passport numbers.
var requiredPayloadKeys = []string{
	"name_lat", "name_cyr", "gender_ru", "birth_date",
	"passport_number", "issue_date", "expiry_date",
	"phone", "home_address_ru",
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

// validateSubmissionPayload checks required fields and returns the list of
// missing keys, if any.
func validateSubmissionPayload(payload map[string]any) []string {
	var missing []string
	for _, k := range requiredPayloadKeys {
		v, ok := payload[k].(string)
		if !ok || v == "" {
			missing = append(missing, k)
		}
	}
	return missing
}

// CreateSubmissionByManager handles POST /api/submissions — manager "create
// manually" flow. Uses the authenticated session's org_id.
func CreateSubmissionByManager(pool *pgxpool.Pool) http.HandlerFunc {
	type reqBody struct {
		Payload         map[string]any `json:"payload"`
		ConsentAccepted bool           `json:"consent_accepted"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())

		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !body.ConsentAccepted {
			writeError(w, http.StatusBadRequest, "consent not accepted")
			return
		}
		if missing := validateSubmissionPayload(body.Payload); len(missing) > 0 {
			writeErrorWithDetails(w, http.StatusBadRequest, "missing fields", map[string]any{
				"missing": missing,
			})
			return
		}

		payloadBytes, _ := json.Marshal(body.Payload)
		agreement := consent.Current()

		id, err := db.CreateSubmissionForOrg(r.Context(), pool, orgID, payloadBytes, agreement.Version, "manager")
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

// ListSubmissions handles GET /api/submissions?q=&status=
func ListSubmissions(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		q := r.URL.Query().Get("q")
		status := r.URL.Query().Get("status")

		out, err := db.ListSubmissions(r.Context(), pool, orgID, q, status)
		if err != nil {
			slog.Error("list submissions", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetSubmission handles GET /api/submissions/:id
func GetSubmission(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		s, err := db.GetSubmission(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("get submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if s == nil {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		writeJSON(w, http.StatusOK, s)
	}
}

// UpdateSubmission handles PUT /api/submissions/:id — manager edits payload.
func UpdateSubmission(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		var body struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		payloadBytes, _ := json.Marshal(body.Payload)

		ok, err := db.UpdateSubmission(r.Context(), pool, orgID, id, payloadBytes)
		if err != nil {
			slog.Error("update submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ArchiveSubmission handles DELETE /api/submissions/:id — soft archive.
func ArchiveSubmission(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		ok, err := db.ArchiveSubmission(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("archive submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "not found or already archived")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// EraseSubmission handles DELETE /api/submissions/:id/erase — hard delete.
// Clears submission_snapshot on attached tourists in the same transaction.
func EraseSubmission(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		ok, err := db.EraseSubmission(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("erase submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		slog.Info("submission erased", "id", id, "org_id", orgID)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// AttachSubmission handles POST /api/submissions/:id/attach
// Body: { "group_id": "...", "subgroup_id": "..." | null }
//
// uploadsDir is needed because ticket/voucher scans uploaded with the
// submission are physically copied into the tourist-level uploads tree on
// attach — that way the tourist's "Документы" section is pre-populated and
// the docs survive any later submission-side mutation (re-upload / erase).
func AttachSubmission(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	type reqBody struct {
		GroupID    string  `json:"group_id"`
		SubgroupID *string `json:"subgroup_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")

		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupID == "" {
			writeError(w, http.StatusBadRequest, "group_id required")
			return
		}

		touristID, err := db.AttachSubmissionToGroup(r.Context(), pool, orgID, submissionID, body.GroupID, body.SubgroupID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(w, http.StatusNotFound, "submission or group not found")
				return
			}
			if errors.Is(err, db.ErrAlreadyAttached) {
				writeError(w, http.StatusConflict, "submission already attached")
				return
			}
			slog.Error("attach submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		copyAttachedSubmissionDocs(r.Context(), pool, uploadsDir, orgID, submissionID, touristID, body.GroupID)

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
