package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/captcha"
	"fujitravel-admin/backend/internal/consent"
	"fujitravel-admin/backend/internal/db"
	appmw "fujitravel-admin/backend/internal/middleware"
	"fujitravel-admin/backend/internal/storage"
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

// publicSubmitMaxBody caps the entire multipart envelope. With per-file
// cap at 50 MB and up to ~6 files (multiple internal passport pages,
// tickets, vouchers + 1 foreign), 250 MB has comfortable headroom.
const publicSubmitMaxBody = 250 << 20

// pendingTmpFile tracks a scan that has been streamed to a temporary
// file under <uploadsDir>/<orgID>/submissions/.tmp.<rand>.<ext> while
// we wait for the parent submission row to be inserted. After the row
// id is known, each tmp file is renamed to its final path under
// <uploadsDir>/<orgID>/submissions/<submissionID>/, and a
// submission_files row is inserted.
type pendingTmpFile struct {
	tmpPath      string
	fileType     string
	originalName string
	mime         string
	size         int64
}

// PublicSubmit handles POST /api/public/submissions/:slug.
//
// Atomic multipart submit: a single multipart/form-data request carries
// the JSON payload, the consent flag, and every uploaded scan. There is
// no draft mechanism on the public side — the previous "create draft →
// upload files individually → finalize" sequence opened a DDoS-shaped
// hole on /start (one row per request, no rate cost ceiling) and added
// state we don't need.
//
// Multipart form fields:
//   - payload (text)              JSON, same shape as the legacy body.payload
//   - consent_accepted (text)     "true" required
//   - passport_internal (file)    optional, repeating
//   - passport_foreign  (file)    optional, single
//   - ticket            (file)    optional, repeating
//   - voucher           (file)    optional, repeating
//
// Limits: whole body 250 MB (http.MaxBytesReader); per-file 50 MB
// (maxSubmissionFileSize); allowed mime application/pdf, image/jpeg,
// image/png (sniffed from the first 512 bytes — we don't trust the
// client-supplied filename or Content-Type). Files are streamed straight
// to a tmp file on disk so peak per-request memory stays at ~512 bytes
// regardless of body size.
// smartTokenMaxBytes caps the size of the smart-token text part. Real
// SmartCaptcha tokens are well under 512 bytes — anything larger is
// either a misuse or an attempt to burn memory.
const smartTokenMaxBytes = 4 << 10

func PublicSubmit(pool *pgxpool.Pool, uploadsDir string, captchaVerifier *captcha.Verifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
		if err != nil {
			slog.Error("public submit: org lookup", "err", err)
			writeError(w, http.StatusInternalServerError, "db")
			return
		}
		if org == nil {
			writeError(w, http.StatusNotFound, "form not found")
			return
		}

		// Cap whole-body bytes. http.MaxBytesReader fires the
		// "request body too large" error at the next read after the
		// limit, which we surface as 413.
		r.Body = http.MaxBytesReader(w, r.Body, publicSubmitMaxBody)

		mr, err := r.MultipartReader()
		if err != nil {
			writeError(w, http.StatusBadRequest, "expected multipart/form-data")
			return
		}

		// Per-org temp dir for in-flight scans. We don't yet know the
		// submission id — that's only allocated AFTER all files are
		// validated and the parent row INSERT succeeds.
		tmpDir := filepath.Join(uploadsDir, org.ID, "submissions")
		if err := os.MkdirAll(tmpDir, storage.SubmissionDirPerm); err != nil {
			slog.Error("public submit: mkdir tmp", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to create dir")
			return
		}

		var (
			payloadBytes  []byte
			consentRaw    string
			captchaToken  string
			pending       []pendingTmpFile
		)

		// Best-effort cleanup of every tmp file written so far. Re-pointed
		// to nil after each tmp is renamed into place; on any error path
		// we call this directly and bail.
		cleanupTmp := func() {
			for _, p := range pending {
				if p.tmpPath == "" {
					continue
				}
				if err := os.Remove(p.tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					slog.Warn("public submit: remove tmp", "path", p.tmpPath, "err", err)
				}
			}
		}

		// PASS 1: walk every multipart part. Text fields go in memory
		// (small); file parts stream to a tmp file.
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				cleanupTmp()
				if err.Error() == "http: request body too large" {
					writeError(w, http.StatusRequestEntityTooLarge, "request too large")
					return
				}
				writeError(w, http.StatusBadRequest, "failed to read multipart")
				return
			}

			name := part.FormName()
			switch {
			case name == "payload":
				// JSON envelope — cap to 1 MiB. A real form is < 16 KiB;
				// 1 MiB leaves headroom for future fields without giving
				// an attacker a memory-burn lever.
				b, err := io.ReadAll(io.LimitReader(part, 1<<20))
				part.Close()
				if err != nil {
					cleanupTmp()
					writeError(w, http.StatusBadRequest, "failed to read payload")
					return
				}
				payloadBytes = b
			case name == "consent_accepted":
				b, err := io.ReadAll(io.LimitReader(part, 64))
				part.Close()
				if err != nil {
					cleanupTmp()
					writeError(w, http.StatusBadRequest, "failed to read consent_accepted")
					return
				}
				consentRaw = string(b)
			case name == "smart-token":
				// SmartCaptcha token issued by the frontend widget.
				// Required only when the verifier is enabled (i.e.
				// YANDEX_CAPTCHA_SECRET is set in the env). Capped at
				// smartTokenMaxBytes — real tokens are <512 B.
				b, err := io.ReadAll(io.LimitReader(part, smartTokenMaxBytes))
				part.Close()
				if err != nil {
					cleanupTmp()
					writeError(w, http.StatusBadRequest, "failed to read smart-token")
					return
				}
				captchaToken = string(b)
			case allowedSubmissionFileTypes[name]:
				p, status, msg, perr := streamPartToTmp(part, tmpDir, name)
				part.Close()
				if perr != nil {
					cleanupTmp()
					writeError(w, status, msg)
					return
				}
				pending = append(pending, p)
			default:
				// Unknown field — drain so the connection stays
				// consistent. Don't fail: future frontend additions
				// shouldn't break old backends harder than necessary.
				_, _ = io.Copy(io.Discard, part)
				part.Close()
			}
		}

		// VALIDATION
		if len(payloadBytes) == 0 {
			cleanupTmp()
			writeError(w, http.StatusBadRequest, "missing payload")
			return
		}
		if consentRaw != "true" {
			cleanupTmp()
			writeError(w, http.StatusBadRequest, "consent not accepted")
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			cleanupTmp()
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var missing []string
		for _, k := range requiredPayloadKeys {
			v, ok := payload[k].(string)
			if !ok || v == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			cleanupTmp()
			writeErrorWithDetails(w, http.StatusBadRequest, "Заполнены не все обязательные поля", map[string]any{"missing": missing})
			return
		}
		// One foreign passport at most — multiple is a frontend bug we'd
		// rather surface as 400 than silently keep the last.
		var foreignCount int
		for _, p := range pending {
			if p.fileType == "passport_foreign" {
				foreignCount++
			}
		}
		if foreignCount > 1 {
			cleanupTmp()
			writeError(w, http.StatusBadRequest, "at most one passport_foreign allowed")
			return
		}

		// Captcha gate. When the verifier is disabled (no
		// YANDEX_CAPTCHA_SECRET in env, typical local-dev case) Verify
		// is a no-op and returns nil regardless of the token. When
		// enabled it returns a sentinel error we map to a 400 with a
		// Russian-language client message tailored to the failure mode.
		if err := captchaVerifier.Verify(r.Context(), captchaToken, appmw.ClientIP(r)); err != nil {
			cleanupTmp()
			if errors.Is(err, captcha.ErrTokenMissing) {
				writeError(w, http.StatusBadRequest, "Подтвердите, что вы не робот")
				return
			}
			// ErrRejected and ErrTransport both surface as the same
			// generic 400 — we don't expose Yandex internals to the
			// tourist. Transport failures are already logged at Warn
			// inside the verifier; log the rejection cause here at
			// Info so ops can correlate without grep gymnastics.
			slog.Info("public submit: captcha verify failed", "err", err)
			writeError(w, http.StatusBadRequest, "Не удалось подтвердить, что вы не робот, попробуйте ещё раз")
			return
		}

		// Re-marshal payload to canonical JSON for storage.
		canonicalPayload, err := json.Marshal(payload)
		if err != nil {
			cleanupTmp()
			slog.Error("public submit: marshal payload", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// INSERT parent submission row. The dedup partial unique index
		// idx_tourist_submissions_dedup fires on status='pending' rows
		// that share (org_id, passport_number, DATE(created_at AT TIME
		// ZONE 'UTC')) — duplicates surface as PG 23505.
		agreement := consent.Current()
		submissionID, err := db.CreateSubmissionForOrg(r.Context(), pool, org.ID, canonicalPayload, agreement.Version, "tourist")
		if err != nil {
			cleanupTmp()
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, http.StatusConflict, "duplicate submission")
				return
			}
			slog.Error("public submit: create submission", "err", err)
			writeError(w, http.StatusInternalServerError, "db")
			return
		}

		// PASS 2: rename each tmp file to its final path under the
		// new submission's directory, and INSERT a submission_files row
		// for it. Foreign passport collapses to a fixed name; multi-types
		// get a random suffix.
		//
		// Failure semantics: if anything goes wrong below, we leave the
		// parent submission row in place (it's a known minor cost — the
		// manager UI will simply see a row with fewer files than the
		// tourist intended), best-effort delete files renamed so far +
		// remaining tmps, and 500. Going fully transactional with the
		// disk would mean two-phase commits we don't need for this flow.
		finalDir := filepath.Join(uploadsDir, org.ID, "submissions", submissionID)
		if err := os.MkdirAll(finalDir, storage.SubmissionDirPerm); err != nil {
			cleanupTmp()
			slog.Error("public submit: mkdir final", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to create dir")
			return
		}

		// Track files renamed into place so we can clean them up if a
		// later DB insert fails.
		var publishedPaths []string
		failPublish := func(stage string, err error) {
			slog.Error("public submit: "+stage, "err", err)
			for _, fp := range publishedPaths {
				if rmErr := os.Remove(fp); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
					slog.Warn("public submit: remove published on fail", "path", fp, "err", rmErr)
				}
			}
			cleanupTmp()
			writeError(w, http.StatusInternalServerError, "failed to persist files")
		}

		for i, p := range pending {
			var finalPath string
			if multiFileTypes[p.fileType] {
				suffix, sErr := randomFileSuffix()
				if sErr != nil {
					failPublish("random suffix", sErr)
					return
				}
				finalPath, err = storage.BuildSubmissionMultiFilePath(uploadsDir, org.ID, submissionID, p.fileType, suffix, p.originalName, p.mime)
			} else {
				finalPath, err = storage.BuildSubmissionFilePath(uploadsDir, org.ID, submissionID, p.fileType, p.originalName, p.mime)
			}
			if err != nil {
				failPublish("build path", err)
				return
			}
			if err := os.Rename(p.tmpPath, finalPath); err != nil {
				failPublish("rename tmp", err)
				return
			}
			pending[i].tmpPath = "" // mark as published; cleanupTmp skips empty paths
			publishedPaths = append(publishedPaths, finalPath)

			row := db.SubmissionFile{
				OrgID:        org.ID,
				SubmissionID: submissionID,
				FileType:     p.fileType,
				FilePath:     finalPath,
				OriginalName: p.originalName,
				MIMEType:     p.mime,
				SizeBytes:    p.size,
			}
			// Plain INSERT — the submission was just created so there
			// cannot be a (submission_id, file_type) conflict yet, even
			// for single-row types like passport_foreign.
			if _, err := db.InsertSubmissionFile(r.Context(), pool, row); err != nil {
				failPublish("insert submission_file", err)
				return
			}
		}

		writeJSON(w, http.StatusCreated, map[string]string{"id": submissionID})
	}
}

// streamPartToTmp consumes one file part of the multipart body, sniffs
// its mime from the first 512 bytes, rejects anything outside the PDF /
// JPEG / PNG whitelist, and writes the bytes to a tmp file under tmpDir.
// The tmp file's name is ".tmp.<rand>.<ext>" so ops glance can tell it's
// in-progress.
//
// Returns (pendingTmpFile, http status, error message, internal error).
// The caller is responsible for cleanup of the tmp file on any non-nil
// error return.
func streamPartToTmp(part *multipart.Part, tmpDir, fileType string) (pendingTmpFile, int, string, error) {
	origFilename := part.FileName()

	// Sniff mime BEFORE allocating the tmp file path — we need the
	// extension to decide the on-disk filename, and the extension is
	// derived from the sniffed mime when the original filename has none.
	head := make([]byte, 512)
	n, readErr := io.ReadFull(part, head)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		if readErr.Error() == "http: request body too large" {
			return pendingTmpFile{}, http.StatusRequestEntityTooLarge, "file too large", readErr
		}
		return pendingTmpFile{}, http.StatusBadRequest, "failed to read file part", readErr
	}
	head = head[:n]
	mime := detectScanMime(origFilename, head)
	switch mime {
	case "application/pdf", "image/jpeg", "image/png":
	default:
		return pendingTmpFile{}, http.StatusBadRequest, "unsupported mime type", errors.New("bad mime")
	}

	// Pick the on-disk extension the same way as the storage builders
	// would — ensures the tmp file name and the eventual final name
	// share the same suffix, which keeps any ops `ls` output coherent.
	ext := extForSubmissionTmp(origFilename, mime)
	tmpName, err := submissionTmpName(ext)
	if err != nil {
		return pendingTmpFile{}, http.StatusInternalServerError, "internal error", err
	}
	tmpPath := filepath.Join(tmpDir, tmpName)
	tmp, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, storage.SubmissionFilePerm)
	if err != nil {
		return pendingTmpFile{}, http.StatusInternalServerError, "failed to create file", err
	}
	if _, err := tmp.Write(head); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return pendingTmpFile{}, http.StatusInternalServerError, "failed to write", err
	}
	// Per-file cap: we already consumed up to 512 bytes; remaining budget
	// is maxSubmissionFileSize - len(head) + 1 so io.CopyN signals an
	// over-limit body via n > limit rather than truncating silently.
	remaining := int64(maxSubmissionFileSize) - int64(len(head)) + 1
	n2, err := io.CopyN(tmp, part, remaining)
	closeErr := tmp.Close()
	if err != nil && err != io.EOF {
		_ = os.Remove(tmpPath)
		if err.Error() == "http: request body too large" {
			return pendingTmpFile{}, http.StatusRequestEntityTooLarge, "file too large", err
		}
		return pendingTmpFile{}, http.StatusInternalServerError, "failed to write", err
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return pendingTmpFile{}, http.StatusInternalServerError, "failed to write", closeErr
	}
	total := int64(len(head)) + n2
	if total > int64(maxSubmissionFileSize) {
		_ = os.Remove(tmpPath)
		return pendingTmpFile{}, http.StatusRequestEntityTooLarge, "file too large", errors.New("over limit")
	}

	return pendingTmpFile{
		tmpPath:      tmpPath,
		fileType:     fileType,
		originalName: origFilename,
		mime:         mime,
		size:         total,
	}, 0, "", nil
}

// extForSubmissionTmp picks the tmp-file extension. Same priority as
// storage.extForSubmissionFile (filename ext first, then mime), but
// kept local so the storage package doesn't need a public export of
// that helper just for one caller.
func extForSubmissionTmp(originalFilename, mime string) string {
	switch ext := lowerExt(originalFilename); ext {
	case ".pdf", ".jpg", ".jpeg", ".png":
		return ext
	}
	switch mime {
	case "application/pdf":
		return ".pdf"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	}
	return ""
}

// lowerExt returns the lowercase dot-prefixed extension of name, or ""
// if there is none. Avoids pulling strings.ToLower(filepath.Ext(...))
// out into a one-liner just so the function name documents intent.
func lowerExt(name string) string {
	ext := filepath.Ext(name)
	out := make([]byte, len(ext))
	for i := 0; i < len(ext); i++ {
		c := ext[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

