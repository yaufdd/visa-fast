package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

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
//
// Bytes are streamed through MultipartReader -> a sibling temp file
// (".tmp.<rand>.<ext>") rather than buffered in memory, so peak per-
// request RSS stays at the 512-byte mime-sniff buffer regardless of the
// 50 MB body cap. After the DB upsert decides the winning metadata, the
// tmp file is os.Rename'd into place — atomic on the same filesystem.
//
// Concurrency note: two concurrent uploads for the same (submission_id,
// file_type) can both write tmp files; the DB upsert serialises which
// metadata wins, so on-disk bytes match the winning row at the moment
// of rename. There is still a deeper race (A renames, then B's later
// rename overwrites disk while DB still points at A) — fixing that
// would need pg_advisory_xact_lock around the upsert; punted for the
// MVP public form, where the same tourist is filling their own draft.
func PublicUploadSubmissionFile(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		fileType := chi.URLParam(r, "type")
		if !allowedSubmissionFileTypes[fileType] {
			writeError(w, http.StatusBadRequest, "invalid file type")
			return
		}

		// Cap the entire request body at the network layer; protects
		// against an attacker streaming an unbounded multipart envelope.
		// We add a small slack for the multipart boundary + the small
		// submission_id form field.
		r.Body = http.MaxBytesReader(w, r.Body, maxSubmissionFileSize+(1<<16))

		mr, err := r.MultipartReader()
		if err != nil {
			writeError(w, http.StatusBadRequest, "expected multipart/form-data")
			return
		}

		var (
			submissionID  string
			origFilename  string
			tmpPath       string
			finalPath     string
			sniffedMime   string
			sizeBytes     int64
			haveFile      bool
		)
		// Best-effort cleanup if we exit before the rename succeeds.
		defer func() {
			if tmpPath != "" {
				if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					slog.Warn("remove submission tmp file", "path", tmpPath, "err", err)
				}
			}
		}()

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				if err.Error() == "http: request body too large" {
					writeError(w, http.StatusRequestEntityTooLarge, "file too large")
					return
				}
				writeError(w, http.StatusBadRequest, "failed to read multipart")
				return
			}
			switch part.FormName() {
			case "submission_id":
				// Cap to 1 KiB — it's a UUID, anything larger is junk.
				b, err := io.ReadAll(io.LimitReader(part, 1<<10))
				part.Close()
				if err != nil {
					writeError(w, http.StatusBadRequest, "failed to read submission_id")
					return
				}
				submissionID = string(b)
			case "file":
				if haveFile {
					part.Close()
					writeError(w, http.StatusBadRequest, "duplicate 'file' part")
					return
				}
				origFilename = part.FileName()

				// Sniff mime from the first 512 bytes BEFORE we know the
				// final path (the path depends on the extension, which
				// depends on the mime if the original filename has none).
				head := make([]byte, 512)
				n, readErr := io.ReadFull(part, head)
				if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
					part.Close()
					if readErr.Error() == "http: request body too large" {
						writeError(w, http.StatusRequestEntityTooLarge, "file too large")
						return
					}
					writeError(w, http.StatusBadRequest, "failed to read file part")
					return
				}
				head = head[:n]
				sniffedMime = detectScanMime(origFilename, head)
				switch sniffedMime {
				case "application/pdf", "image/jpeg", "image/png":
				default:
					part.Close()
					writeError(w, http.StatusBadRequest, "unsupported mime type")
					return
				}

				// Resolve the org now so we can place the tmp file in the
				// correct per-org dir; we still need submissionID to be
				// set, but the multipart spec doesn't guarantee field
				// order. If submission_id wasn't seen yet we error out;
				// HTML forms send fields in document order, so a
				// well-formed client always puts submission_id first.
				if submissionID == "" {
					part.Close()
					writeError(w, http.StatusBadRequest, "submission_id must precede file part")
					return
				}
				orgID, _, ok := resolveDraftSubmission(r, pool, slug, submissionID)
				if !ok {
					part.Close()
					writeError(w, http.StatusNotFound, "submission not found")
					return
				}

				finalPath, err = storage.BuildSubmissionFilePath(uploadsDir, orgID, submissionID, fileType, origFilename, sniffedMime)
				if err != nil {
					part.Close()
					slog.Error("build submission file path", "err", err)
					writeError(w, http.StatusBadRequest, "invalid path component")
					return
				}
				dir := filepath.Dir(finalPath)
				if err := os.MkdirAll(dir, storage.SubmissionDirPerm); err != nil {
					part.Close()
					slog.Error("mkdir submission dir", "err", err)
					writeError(w, http.StatusInternalServerError, "failed to create dir")
					return
				}

				// Sibling tmp filename: ".tmp.<8 hex>.<ext>". Dot prefix
				// marks it as in-progress so ops glance won't confuse it
				// with a finished upload.
				tmpName, err := submissionTmpName(filepath.Ext(finalPath))
				if err != nil {
					part.Close()
					slog.Error("submission tmp name", "err", err)
					writeError(w, http.StatusInternalServerError, "internal error")
					return
				}
				tmpPath = filepath.Join(dir, tmpName)

				tmp, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, storage.SubmissionFilePerm)
				if err != nil {
					part.Close()
					slog.Error("create submission tmp file", "err", err)
					writeError(w, http.StatusInternalServerError, "failed to create file")
					return
				}

				// Write the sniffed head, then stream the rest, capped.
				if _, err := tmp.Write(head); err != nil {
					tmp.Close()
					part.Close()
					slog.Error("write submission head", "err", err)
					writeError(w, http.StatusInternalServerError, "failed to write")
					return
				}
				// We've already consumed up to 512 bytes; the remaining
				// budget is maxSubmissionFileSize - len(head) + 1 so
				// io.CopyN signals an over-limit body via n > limit.
				remaining := int64(maxSubmissionFileSize) - int64(len(head)) + 1
				n2, err := io.CopyN(tmp, part, remaining)
				closeErr := tmp.Close()
				partCloseErr := part.Close()
				_ = partCloseErr
				if err != nil && err != io.EOF {
					if err.Error() == "http: request body too large" {
						writeError(w, http.StatusRequestEntityTooLarge, "file too large")
						return
					}
					slog.Error("copy submission body", "err", err)
					writeError(w, http.StatusInternalServerError, "failed to write")
					return
				}
				if closeErr != nil {
					slog.Error("close submission tmp", "err", closeErr)
					writeError(w, http.StatusInternalServerError, "failed to write")
					return
				}
				total := int64(len(head)) + n2
				if total > int64(maxSubmissionFileSize) {
					writeError(w, http.StatusRequestEntityTooLarge, "file too large")
					return
				}
				sizeBytes = total
				haveFile = true
			default:
				// Unknown field — drain and discard so the connection
				// stays consistent.
				_, _ = io.Copy(io.Discard, part)
				part.Close()
			}
		}

		if submissionID == "" {
			writeError(w, http.StatusBadRequest, "submission_id required")
			return
		}
		if !haveFile {
			writeError(w, http.StatusBadRequest, "missing 'file' field in form")
			return
		}

		// Re-resolve org for the DB call (the earlier resolution was used
		// to build the path; we need orgID here too and prefer not to
		// thread it out of the multipart loop).
		orgID, _, ok := resolveDraftSubmission(r, pool, slug, submissionID)
		if !ok {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		row := db.SubmissionFile{
			OrgID:        orgID,
			SubmissionID: submissionID,
			FileType:     fileType,
			FilePath:     finalPath,
			OriginalName: origFilename,
			MIMEType:     sniffedMime,
			SizeBytes:    sizeBytes,
		}
		id, oldPath, replaced, err := db.InsertOrReplaceSubmissionFile(r.Context(), pool, row)
		if err != nil {
			slog.Error("insert submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Atomic publish: rename tmp -> final. After this, the deferred
		// cleanup is a no-op because tmpPath no longer exists.
		if err := os.Rename(tmpPath, finalPath); err != nil {
			slog.Error("rename submission tmp", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to publish file")
			return
		}
		tmpPath = ""

		if replaced && oldPath != "" && oldPath != finalPath {
			if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("remove old submission file", "path", oldPath, "err", err)
			}
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":            id,
			"submission_id": submissionID,
			"file_type":     fileType,
			"original_name": origFilename,
			"mime_type":     sniffedMime,
			"size_bytes":    sizeBytes,
		})
	}
}

// submissionTmpName returns ".tmp.<8 hex>.<ext>" for use as a sibling tmp
// filename next to the final submission file. The dot prefix flags it as
// in-progress; the random suffix avoids collisions between concurrent
// uploads for the same (submission_id, file_type).
func submissionTmpName(ext string) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".tmp." + hex.EncodeToString(b[:]) + ext, nil
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

