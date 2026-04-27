package api

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
	appmw "fujitravel-admin/backend/internal/middleware"
)

// Manager-facing counterparts of handlers_public_files.go: list, download,
// and (rarely) delete the files a tourist attached via the public wizard.
//
// Enumeration safety: wrong-org / missing-row collapses to 404 — same
// convention as resolveDraftSubmission used by the public siblings, and
// every other authenticated handler in this codebase. Cross-org access
// must NOT leak existence via 403.
//
// Defence in depth on download/delete: even with a session cookie, we
// re-verify that the file row's submission_id matches the URL path
// segment. A leaked file UUID alone shouldn't be enough to act on a row
// that doesn't belong to the parent submission the URL claims.

// submissionExistsForOrg returns true when the given submission id exists
// inside the given org. The row may be in any status — managers list and
// download files for both 'draft' and finalised submissions.
func submissionExistsForOrg(r *http.Request, pool *pgxpool.Pool, orgID, submissionID string) bool {
	sub, err := db.GetSubmission(r.Context(), pool, orgID, submissionID)
	if err != nil {
		slog.Error("admin submission lookup", "err", err)
		return false
	}
	return sub != nil
}

// ListSubmissionFiles handles GET /api/submissions/{id}/files.
// Returns metadata for every file attached to the submission. file_path
// and org_id are stripped from the response — managers don't need on-disk
// paths, and not exposing them limits the blast radius of any future
// path-traversal slip.
func ListSubmissionFiles(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")

		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		files, err := db.ListSubmissionFiles(r.Context(), pool, orgID, submissionID)
		if err != nil {
			slog.Error("list submission files", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		out := make([]map[string]any, 0, len(files))
		for _, f := range files {
			out = append(out, map[string]any{
				"id":            f.ID,
				"submission_id": f.SubmissionID,
				"file_type":     f.FileType,
				"original_name": f.OriginalName,
				"mime_type":     f.MIMEType,
				"size_bytes":    f.SizeBytes,
				"created_at":    f.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DownloadSubmissionFile handles GET /api/submissions/{id}/files/{file_id}/download.
// Streams the file bytes back with an RFC 2047-encoded Content-Disposition
// so non-ASCII filenames (e.g. "паспорт.pdf") survive the round-trip.
//
// If the DB row exists but the on-disk file is gone, we return 410 Gone
// rather than 500 — the row is out of sync with the filesystem, which is
// unusual but worth signalling distinctly so the manager UI can show a
// "deleted on server" badge instead of a generic error.
func DownloadSubmissionFile(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")
		fileID := chi.URLParam(r, "file_id")

		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		f, err := db.GetSubmissionFile(r.Context(), pool, orgID, fileID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "file not found")
				return
			}
			slog.Error("get submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		// Defence in depth: file row must belong to the URL's submission.
		if f.SubmissionID != submissionID {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}

		file, err := os.Open(f.FilePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				slog.Warn("submission file missing on disk", "path", f.FilePath, "id", f.ID)
				writeError(w, http.StatusGone, "file no longer on disk")
				return
			}
			slog.Error("open submission file", "err", err, "path", f.FilePath)
			writeError(w, http.StatusInternalServerError, "failed to open file")
			return
		}
		defer file.Close()

		// RFC 2047 encoded-word covers non-ASCII. Browsers render this
		// correctly as the suggested save-as name; older clients fall back
		// to the raw encoded form (still readable, just not pretty).
		encoded := mime.QEncoding.Encode("utf-8", f.OriginalName)
		w.Header().Set("Content-Type", f.MIMEType)
		w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
		w.Header().Set("Content-Disposition", `attachment; filename="`+encoded+`"`)
		if _, err := io.Copy(w, file); err != nil {
			// Headers are already on the wire; can't switch to a JSON
			// error. Just log so we notice broken-pipe spikes.
			slog.Warn("stream submission file", "err", err, "id", f.ID)
		}
	}
}

// DeleteSubmissionFile handles DELETE /api/submissions/{id}/files/{file_id}.
// Removes the DB row first; the on-disk file is removed best-effort
// (missing is not an error — same pattern as DeleteTouristUpload).
func DeleteSubmissionFile(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")
		fileID := chi.URLParam(r, "file_id")

		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		// Confirm the file row belongs to this submission before deleting,
		// mirroring PublicDeleteSubmissionFile. Cheaper than relying on the
		// DELETE itself to enforce the relationship since the org-scoped
		// DELETE wouldn't catch a (different submission, same org) leak.
		f, err := db.GetSubmissionFile(r.Context(), pool, orgID, fileID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "file not found")
				return
			}
			slog.Error("get submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if f.SubmissionID != submissionID {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}

		path, ok, err := db.DeleteSubmissionFile(r.Context(), pool, orgID, fileID)
		if err != nil {
			slog.Error("delete submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		if path != "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("remove submission file from disk", "path", path, "err", err)
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
