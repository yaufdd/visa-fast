package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/db"
	appmw "fujitravel-admin/backend/internal/middleware"
	"fujitravel-admin/backend/internal/storage"
)

// Manager-facing counterparts of the public upload + parse-passport
// endpoints. The wizard component on the dashboard reuses the same hook
// set as the public form, so we expose the same set of mutating
// endpoints — just session-auth'd and willing to accept submissions in
// any status (managers may attach scans to drafts as well as already-
// pending or attached submissions).
//
// Enumeration safety: same convention as the rest of the manager
// handlers — wrong-org / wrong-id collapse to 404, never 403.

// UploadSubmissionFile handles POST /api/submissions/{id}/files/{type}.
//
// Multipart form: field "file" (the bytes). The submission_id is taken
// from the URL — managers always have an existing handle, no need to
// repeat it in the form payload.
//
// Same on-disk layout, perms, mime validation, and 50 MB cap as the
// public sibling (handlers_public_files.go). The handler streams the
// body to a sibling temp file and renames into place atomically after
// the DB upsert decides the winning metadata.
func UploadSubmissionFile(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")
		fileType := chi.URLParam(r, "type")
		if !allowedSubmissionFileTypes[fileType] {
			writeError(w, http.StatusBadRequest, "invalid file type")
			return
		}

		// Verify the submission belongs to this org. Managers may attach
		// files in any status, so we don't gate on draft-only.
		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		// Cap the entire request body at the network layer; small slack
		// for the multipart boundary.
		r.Body = http.MaxBytesReader(w, r.Body, maxSubmissionFileSize+(1<<16))

		mr, err := r.MultipartReader()
		if err != nil {
			writeError(w, http.StatusBadRequest, "expected multipart/form-data")
			return
		}

		var (
			origFilename string
			tmpPath      string
			finalPath    string
			sniffedMime  string
			sizeBytes    int64
			haveFile     bool
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
			case "file":
				if haveFile {
					part.Close()
					writeError(w, http.StatusBadRequest, "duplicate 'file' part")
					return
				}
				origFilename = part.FileName()

				// Sniff mime from the first 512 bytes BEFORE we know the
				// final path (the path depends on extension which depends
				// on mime if filename has none).
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

				if multiFileTypes[fileType] {
					suffix, sErr := randomFileSuffix()
					if sErr != nil {
						part.Close()
						slog.Error("random file suffix", "err", sErr)
						writeError(w, http.StatusInternalServerError, "internal error")
						return
					}
					finalPath, err = storage.BuildSubmissionMultiFilePath(uploadsDir, orgID, submissionID, fileType, suffix, origFilename, sniffedMime)
				} else {
					finalPath, err = storage.BuildSubmissionFilePath(uploadsDir, orgID, submissionID, fileType, origFilename, sniffedMime)
				}
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

				if _, err := tmp.Write(head); err != nil {
					tmp.Close()
					part.Close()
					slog.Error("write submission head", "err", err)
					writeError(w, http.StatusInternalServerError, "failed to write")
					return
				}
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
				// Unknown field — drain so the connection stays consistent.
				_, _ = io.Copy(io.Discard, part)
				part.Close()
			}
		}

		if !haveFile {
			writeError(w, http.StatusBadRequest, "missing 'file' field in form")
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
		var (
			id       string
			oldPath  string
			replaced bool
		)
		if multiFileTypes[fileType] {
			id, err = db.InsertSubmissionFile(r.Context(), pool, row)
		} else {
			id, oldPath, replaced, err = db.InsertOrReplaceSubmissionFile(r.Context(), pool, row)
		}
		if err != nil {
			slog.Error("insert submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Atomic publish.
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

// ParseSubmissionPassport handles POST /api/submissions/{id}/parse-passport.
//
// Body: {"file_id":"uuid","type":"internal"|"foreign"}
//
// Streams the previously-uploaded passport scan through the same Yandex
// pipeline used by the public-form sibling and returns the structured
// PassportFields so the dashboard wizard can pre-fill the form. Side-
// effect free — the handler does NOT mutate the submission's payload.
func ParseSubmissionPassport(pool *pgxpool.Pool, ocr ai.OCRRecognizer, translator ai.Translator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")

		// Cap the body — it's a tiny JSON envelope.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<14)

		var body struct {
			FileID string `json:"file_id"`
			Type   string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if body.FileID == "" {
			writeError(w, http.StatusBadRequest, "file_id required")
			return
		}

		var (
			pType        ai.PassportType
			expectedType string
		)
		switch body.Type {
		case "internal":
			pType = ai.PassportInternal
			expectedType = "passport_internal"
		case "foreign":
			pType = ai.PassportForeign
			expectedType = "passport_foreign"
		default:
			writeError(w, http.StatusBadRequest, "type must be 'internal' or 'foreign'")
			return
		}

		// Verify ownership of the parent submission.
		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		f, err := db.GetSubmissionFile(r.Context(), pool, orgID, body.FileID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "file not found")
				return
			}
			slog.Error("get submission file", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		// Defence in depth: file row must belong to this submission.
		if f.SubmissionID != submissionID {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		if f.FileType != expectedType {
			writeError(w, http.StatusBadRequest, "file is not a "+expectedType+" scan")
			return
		}

		scanBytes, err := os.ReadFile(f.FilePath)
		if err != nil {
			slog.Error("read passport scan for parse", "err", err, "path", f.FilePath)
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		mime := detectScanMime(f.FilePath, scanBytes)

		// Audit context — org-only. The submission may not be attached to
		// any group/subgroup yet (managers can run parse during creation
		// just like the tourist does on the public form).
		aiCtx := withAuditCtx(r.Context(), pool, orgID, "", "")

		fields, err := ai.ParsePassportScan(aiCtx, ocr, translator, scanBytes, mime, pType)
		if err != nil {
			slog.Error("admin passport parse", "err", err, "type", body.Type)
			writeError(w, http.StatusInternalServerError, "failed to parse passport")
			return
		}

		writeJSON(w, http.StatusOK, fields)
	}
}

// ParseSubmissionTicket handles POST /api/submissions/{id}/parse-ticket.
//
// Body: {"file_id":"uuid"}
//
// Same shape as ParseSubmissionPassport — reads the previously-uploaded
// ticket scan, runs it through the Yandex Vision + GPT pipeline, and
// returns {arrival, departure} so the dashboard wizard can show / store
// the recognised flights. Side-effect free.
func ParseSubmissionTicket(pool *pgxpool.Pool, ocr ai.OCRRecognizer, translator ai.Translator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")

		r.Body = http.MaxBytesReader(w, r.Body, 1<<14)
		var body struct {
			FileID string `json:"file_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if body.FileID == "" {
			writeError(w, http.StatusBadRequest, "file_id required")
			return
		}

		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		f, err := db.GetSubmissionFile(r.Context(), pool, orgID, body.FileID)
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
		if f.FileType != "ticket" {
			writeError(w, http.StatusBadRequest, "file is not a ticket scan")
			return
		}

		scanBytes, err := os.ReadFile(f.FilePath)
		if err != nil {
			slog.Error("read ticket scan for parse", "err", err, "path", f.FilePath)
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		mime := detectScanMime(f.FilePath, scanBytes)

		aiCtx := withAuditCtx(r.Context(), pool, orgID, "", "")
		flights, err := ai.ParseTicketScan(aiCtx, ocr, translator, scanBytes, mime)
		if err != nil {
			slog.Error("admin ticket parse", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to parse ticket")
			return
		}

		flights.Arrival.Airport = ai.NormalizeJapaneseAirport(flights.Arrival.Airport)
		flights.Departure.Airport = ai.NormalizeJapaneseAirport(flights.Departure.Airport)
		writeJSON(w, http.StatusOK, map[string]any{
			"arrival":   flights.Arrival,
			"departure": flights.Departure,
		})
	}
}

// ParseSubmissionVoucher handles POST /api/submissions/{id}/parse-voucher.
//
// Body: {"file_id":"uuid"}
//
// Reads a voucher scan and returns the recognised list of hotel stays.
// Side-effect free — the dashboard wizard merges the result into
// payload.hotels (or shows it for manual review) and the manager can
// then save the submission.
func ParseSubmissionVoucher(pool *pgxpool.Pool, ocr ai.OCRRecognizer, translator ai.Translator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		submissionID := chi.URLParam(r, "id")

		r.Body = http.MaxBytesReader(w, r.Body, 1<<14)
		var body struct {
			FileID string `json:"file_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if body.FileID == "" {
			writeError(w, http.StatusBadRequest, "file_id required")
			return
		}

		if !submissionExistsForOrg(r, pool, orgID, submissionID) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}

		f, err := db.GetSubmissionFile(r.Context(), pool, orgID, body.FileID)
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
		if f.FileType != "voucher" {
			writeError(w, http.StatusBadRequest, "file is not a voucher scan")
			return
		}

		scanBytes, err := os.ReadFile(f.FilePath)
		if err != nil {
			slog.Error("read voucher scan for parse", "err", err, "path", f.FilePath)
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		mime := detectScanMime(f.FilePath, scanBytes)

		aiCtx := withAuditCtx(r.Context(), pool, orgID, "", "")
		hotels, err := ai.ParseVoucherScan(aiCtx, ocr, translator, scanBytes, mime)
		if err != nil {
			slog.Error("admin voucher parse", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to parse voucher")
			return
		}

		writeJSON(w, http.StatusOK, hotels)
	}
}
