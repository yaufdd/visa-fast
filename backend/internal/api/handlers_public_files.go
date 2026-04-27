package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/consent"
	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/storage"
)

// maxSubmissionFileSize caps each tourist upload at 50 MB. Public/
// unauthenticated endpoint, so we keep this tighter than internal upload
// flows; passport / ticket / voucher scans are well under this in practice.
const maxSubmissionFileSize = 50 << 20

// allowedSubmissionFileTypes is the set permitted on the public draft
// upload endpoint and matches the DB CHECK on submission_files.file_type.
var allowedSubmissionFileTypes = map[string]bool{
	"passport_internal": true,
	"passport_foreign":  true,
	"ticket":            true,
	"voucher":           true,
}

// resolveDraftSubmission looks up a draft submission by id, scoped to the
// org behind the slug. Returns (orgID, submission, found). If found is
// false, the caller should respond 404 — the row may not exist, may belong
// to another org, or may already be finalised; we collapse all three to
// "not found" to avoid leaking enumeration signal on a public endpoint.
func resolveDraftSubmission(r *http.Request, pool *pgxpool.Pool, slug, submissionID string) (string, *db.TouristSubmission, bool) {
	org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
	if err != nil {
		slog.Error("public submission slug lookup", "err", err)
		return "", nil, false
	}
	if org == nil {
		return "", nil, false
	}
	sub, err := db.GetSubmission(r.Context(), pool, org.ID, submissionID)
	if err != nil {
		slog.Error("public submission lookup", "err", err)
		return org.ID, nil, false
	}
	if sub == nil || sub.Status != "draft" {
		return org.ID, nil, false
	}
	return org.ID, sub, true
}

// PublicSubmissionStart handles POST /api/public/submissions/{slug}/start.
// Creates a placeholder draft submission so the tourist can attach files
// before completing the form. Body is ignored.
func PublicSubmissionStart(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db")
			return
		}
		if org == nil {
			writeError(w, http.StatusNotFound, "form not found")
			return
		}
		// Drain the body so the connection can be reused; the body is
		// reserved for future telemetry but ignored today.
		_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, 1<<10))

		agreement := consent.Current()
		id, err := db.CreateDraftSubmission(r.Context(), pool, org.ID, agreement.Version)
		if err != nil {
			slog.Error("create draft submission", "err", err)
			writeError(w, http.StatusInternalServerError, "db")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"submission_id": id})
	}
}

// PublicUploadSubmissionFile handles
// POST /api/public/submissions/{slug}/files/{type}.
//
// Multipart form: field "file" (the bytes), form value "submission_id"
// (the draft id returned by /start). Stores the file on disk under
// <uploadsDir>/<org>/submissions/<id>/<type><ext> and inserts/replaces
// the corresponding submission_files row. If a previous file existed for
// the same (submission, type) and lived at a different path, the old file
// is removed best-effort.
func PublicUploadSubmissionFile(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		fileType := chi.URLParam(r, "type")
		if !allowedSubmissionFileTypes[fileType] {
			writeError(w, http.StatusBadRequest, "invalid file type")
			return
		}

		// Cap incoming body BEFORE ParseMultipartForm to enforce the size
		// limit at the network layer, not just at the form-field level.
		r.Body = http.MaxBytesReader(w, r.Body, maxSubmissionFileSize)
		if err := r.ParseMultipartForm(maxSubmissionFileSize); err != nil {
			// Detect MaxBytesReader exhaustion and return 413 instead of
			// the generic 400; the chi router doesn't surface a typed err.
			if err.Error() == "http: request body too large" {
				writeError(w, http.StatusRequestEntityTooLarge, "file too large")
				return
			}
			writeError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}

		submissionID := r.FormValue("submission_id")
		if submissionID == "" {
			writeError(w, http.StatusBadRequest, "submission_id required")
			return
		}

		orgID, _, ok := resolveDraftSubmission(r, pool, slug, submissionID)
		if !ok {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'file' field in form")
			return
		}
		defer file.Close()

		fileData, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		if int64(len(fileData)) > maxSubmissionFileSize {
			writeError(w, http.StatusRequestEntityTooLarge, "file too large")
			return
		}

		mime := detectScanMime(header.Filename, fileData)
		switch mime {
		case "application/pdf", "image/jpeg", "image/png":
		default:
			writeError(w, http.StatusBadRequest, "unsupported mime type")
			return
		}

		savedPath, err := storage.SaveSubmissionFile(uploadsDir, orgID, submissionID, fileType, header.Filename, fileData, mime)
		if err != nil {
			slog.Error("save submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save file")
			return
		}

		row := db.SubmissionFile{
			OrgID:        orgID,
			SubmissionID: submissionID,
			FileType:     fileType,
			FilePath:     savedPath,
			OriginalName: header.Filename,
			MIMEType:     mime,
			SizeBytes:    int64(len(fileData)),
		}
		id, oldPath, replaced, err := db.InsertOrReplaceSubmissionFile(r.Context(), pool, row)
		if err != nil {
			// DB write failed — try to clean up the file we just wrote.
			if rmErr := os.Remove(savedPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				slog.Warn("cleanup submission file after db error", "path", savedPath, "err", rmErr)
			}
			slog.Error("insert submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if replaced && oldPath != "" && oldPath != savedPath {
			if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("remove old submission file", "path", oldPath, "err", err)
			}
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":            id,
			"submission_id": submissionID,
			"file_type":     fileType,
			"original_name": header.Filename,
			"mime_type":     mime,
			"size_bytes":    len(fileData),
		})
	}
}

// PublicListSubmissionFiles handles
// GET /api/public/submissions/{slug}/files?submission_id=...
//
// Returns the metadata rows (no file_path) for the draft submission so the
// frontend can show "passport uploaded / not uploaded" badges next to the
// form fields.
func PublicListSubmissionFiles(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		submissionID := r.URL.Query().Get("submission_id")
		if submissionID == "" {
			writeError(w, http.StatusBadRequest, "submission_id required")
			return
		}
		orgID, _, ok := resolveDraftSubmission(r, pool, slug, submissionID)
		if !ok {
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

// PublicDeleteSubmissionFile handles
// DELETE /api/public/submissions/{slug}/files/{id}?submission_id=...
//
// The submission_id query param is defence-in-depth: we already require
// the file row to belong to the slug's org, but cross-checking the parent
// submission stops a leaked file UUID from being used to delete a row
// the caller doesn't actually own a draft handle for.
func PublicDeleteSubmissionFile(pool *pgxpool.Pool, _ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		fileID := chi.URLParam(r, "id")
		submissionID := r.URL.Query().Get("submission_id")
		if submissionID == "" {
			writeError(w, http.StatusBadRequest, "submission_id required")
			return
		}

		orgID, _, ok := resolveDraftSubmission(r, pool, slug, submissionID)
		if !ok {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		// Confirm the file row belongs to this submission before deleting.
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

		path, ok2, err := db.DeleteSubmissionFile(r.Context(), pool, orgID, fileID)
		if err != nil {
			slog.Error("delete submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok2 {
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

