package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
	"fujitravel-admin/backend/internal/privacy"
	"fujitravel-admin/backend/internal/storage"
)

const maxUploadSize = 32 << 20 // 32 MB

// Only ticket/voucher uploads are accepted in the custom-form workflow.
var allowedTouristFileTypes = map[string]bool{
	"ticket":  true,
	"voucher": true,
}

// UploadTouristFile handles POST /api/tourists/:id/uploads.
// Two-step flow: this handler ONLY saves the file (and pre-uploads the
// redacted copy to Anthropic Files so a later /parse call is cheap). It does
// NOT invoke the AI parser — the manager triggers that explicitly via the
// "Распознать" button (ParseTouristUpload).
//
// Redaction still happens here (fail-loud) so the original never leaves the
// server. Only the redacted bytes are ever uploaded to Anthropic.
func UploadTouristFile(pool *pgxpool.Pool, uploadsDir, apiKey, redactScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		var groupID string
		if err := pool.QueryRow(r.Context(),
			`SELECT group_id FROM tourists WHERE id = $1 AND org_id = $2`,
			touristID, orgID,
		).Scan(&groupID); err != nil {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}

		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}

		fileType := r.FormValue("file_type")
		if !allowedTouristFileTypes[fileType] {
			writeError(w, http.StatusBadRequest, "file_type must be 'ticket' or 'voucher'")
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

		savedPath, err := storage.SaveFileBytes(uploadsDir, groupID, fileType, header.Filename, fileData)
		if err != nil {
			slog.Error("save tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save file")
			return
		}

		// Redact passenger/guest name locally BEFORE anything touches
		// Anthropic. The original stays on disk (savedPath) but never leaves
		// the server; only the redacted copy is used for the AI path.
		//
		// Fail-loud policy: if the redactor can't find a name label we refuse
		// to ship the scan to AI and surface an error to the manager so they
		// can redact manually or enter fields by hand.
		redacted, redactErr := privacy.RedactScan(r.Context(), redactScript, header.Filename, fileData)
		if redactErr != nil {
			msg := redactErr.Error()
			if errors.Is(redactErr, privacy.ErrNoLabelsFound) {
				msg = "не удалось определить область имени на скане — загрузите более чёткое изображение или заполните поля вручную"
			}
			slog.Warn("scan redact failed", "tourist_id", touristID, "err", redactErr)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"id":           "",
				"tourist_id":   touristID,
				"file_path":    savedPath,
				"redact_error": msg,
				"error":        msg,
			})
			return
		}
		uploadBytes := redacted.RedactedBytes
		uploadName := privacy.OutputFilename(header.Filename, redacted.OutputPath)

		// Upload the REDACTED copy to Anthropic (non-fatal — inline fallback
		// at parse time).
		var anthropicFileID string
		if fid, err := ai.UploadFileToAnthropic(apiKey, uploadName, uploadBytes); err != nil {
			slog.Warn("anthropic file upload failed, will use inline fallback", "err", err)
		} else {
			anthropicFileID = fid
		}

		tid := touristID
		uploadID, err := db.InsertUpload(r.Context(), pool, orgID, groupID, &tid, fileType, savedPath)
		if err != nil {
			slog.Error("insert tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if anthropicFileID != "" {
			if err := db.SetUploadAnthropicID(r.Context(), pool, orgID, uploadID, anthropicFileID); err != nil {
				slog.Warn("set upload anthropic id", "err", err)
			}
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         uploadID,
			"tourist_id": touristID,
			"group_id":   groupID,
			"file_type":  fileType,
			"file_path":  savedPath,
			"filename":   header.Filename,
			"created_at": time.Now(),
			"parsed_at":  nil,
		})
	}
}

// ParseTouristUpload handles POST /api/tourists/:id/uploads/:uploadId/parse.
// Runs the matching AI parser (ticket or voucher) on a previously uploaded
// file and stamps parsed_at on success. Idempotent-ish: callers may re-parse,
// though the UI normally hides the button once parsed_at is set.
func ParseTouristUpload(pool *pgxpool.Pool, uploadsDir, apiKey, redactScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")
		uploadID := chi.URLParam(r, "uploadId")

		up, err := db.GetTouristUpload(r.Context(), pool, orgID, touristID, uploadID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "upload not found")
				return
			}
			slog.Error("get tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Fetch subgroup id from tourist (voucher parser needs it).
		var subgroupID *string
		if err := pool.QueryRow(r.Context(),
			`SELECT subgroup_id FROM tourists WHERE id = $1 AND org_id = $2`,
			touristID, orgID,
		).Scan(&subgroupID); err != nil {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}

		// Build the FileInput. Prefer the cached Anthropic file ID; if it
		// wasn't uploaded (Anthropic was down at upload time), re-read the
		// original from disk and re-redact on the fly for an inline fallback.
		fileInput := ai.FileInput{}
		if up.AnthropicFileID != nil && *up.AnthropicFileID != "" {
			fileInput.AnthropicFileID = *up.AnthropicFileID
			fileInput.Name = filenameFromPath(up.FilePath)
		} else {
			origBytes, err := os.ReadFile(up.FilePath)
			if err != nil {
				slog.Error("read upload for parse", "err", err)
				writeError(w, http.StatusInternalServerError, "failed to read upload file")
				return
			}
			redacted, redactErr := privacy.RedactScan(r.Context(), redactScript, filenameFromPath(up.FilePath), origBytes)
			if redactErr != nil {
				msg := redactErr.Error()
				if errors.Is(redactErr, privacy.ErrNoLabelsFound) {
					msg = "не удалось определить область имени на скане — загрузите более чёткое изображение или заполните поля вручную"
				}
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"id":           up.ID,
					"redact_error": msg,
					"error":        msg,
				})
				return
			}
			fileInput.Name = privacy.OutputFilename(filenameFromPath(up.FilePath), redacted.OutputPath)
			fileInput.Data = redacted.RedactedBytes
		}

		sgID := ""
		if subgroupID != nil {
			sgID = *subgroupID
		}
		aiCtx := withAuditCtx(r.Context(), pool, orgID, up.GroupID, sgID)

		resp := map[string]any{
			"id":         up.ID,
			"tourist_id": touristID,
			"group_id":   up.GroupID,
			"file_type":  up.FileType,
		}

		switch up.FileType {
		case "ticket":
			if parseErr := parseTicketAndPersist(aiCtx, pool, apiKey, orgID, touristID, fileInput); parseErr != nil {
				slog.Warn("ticket parse failed", "tourist_id", touristID, "err", parseErr)
				resp["parse_error"] = parseErr.Error()
				writeJSON(w, http.StatusOK, resp)
				return
			}
		case "voucher":
			if parseErr := parseVoucherAndPersist(aiCtx, pool, apiKey, orgID, up.GroupID, subgroupID, fileInput); parseErr != nil {
				slog.Warn("voucher parse failed", "tourist_id", touristID, "err", parseErr)
				resp["parse_error"] = parseErr.Error()
				writeJSON(w, http.StatusOK, resp)
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "unknown file_type on upload")
			return
		}

		if err := db.MarkUploadParsed(r.Context(), pool, orgID, up.ID); err != nil {
			slog.Warn("mark upload parsed", "err", err)
		}
		now := time.Now()
		resp["parsed_at"] = now
		writeJSON(w, http.StatusOK, resp)
	}
}

// filenameFromPath pulls the last path segment (works for both / and \\).
func filenameFromPath(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}

// parseTicketAndPersist calls the ticket parser and writes the result into
// tourists.flight_data (scoped to org).
func parseTicketAndPersist(ctx context.Context, pool *pgxpool.Pool, apiKey, orgID, touristID string, input ai.FileInput) error {
	flights, err := ai.ParseTicket(ctx, apiKey, []ai.FileInput{input})
	if err != nil {
		return err
	}
	flights.Arrival.Airport = ai.NormalizeJapaneseAirport(flights.Arrival.Airport)
	flights.Departure.Airport = ai.NormalizeJapaneseAirport(flights.Departure.Airport)
	buf, _ := json.Marshal(map[string]any{
		"arrival":   flights.Arrival,
		"departure": flights.Departure,
	})
	if _, err := db.UpdateFlightData(ctx, pool, orgID, touristID, buf); err != nil {
		return err
	}
	return nil
}

// convertDate converts DD.MM.YYYY → YYYY-MM-DD for PostgreSQL date fields.
func convertDate(s string) string {
	if len(s) == 10 && s[2] == '.' && s[5] == '.' {
		return s[6:10] + "-" + s[3:5] + "-" + s[0:2]
	}
	return s
}

// parseDate parses "YYYY-MM-DD" or "DD.MM.YYYY" into time.Time.
// Returns zero time on parse failure.
func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	s = convertDate(s)
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseVoucherAndPersist calls the voucher parser and, for each hotel,
// looks up (or creates, scoped to this org) the hotels row and inserts a
// group_hotels row scoped to the tourist's group_id/subgroup_id and org.
func parseVoucherAndPersist(ctx context.Context, pool *pgxpool.Pool, apiKey, orgID, groupID string, subgroupID *string, input ai.FileInput) error {
	hotels, err := ai.ParseVouchers(ctx, apiKey, []ai.FileInput{input})
	if err != nil {
		return err
	}
	for _, h := range hotels {
		// Vouchers tend to emit English CAPS ("TOKYO"); the UI expects
		// canonical Russian ("Токио") so new hotels land consistent.
		h.City = ai.NormalizeCity(h.City)

		var hotelID string
		// Look up in org-visible hotels (global or private to this org).
		err := pool.QueryRow(ctx,
			`SELECT id FROM hotels
			  WHERE LOWER(name_en) = LOWER($1) AND (org_id IS NULL OR org_id = $2)
			  LIMIT 1`, h.Name, orgID,
		).Scan(&hotelID)
		if err != nil && errors.Is(err, pgx.ErrNoRows) {
			// Create a new hotel private to this org.
			err = pool.QueryRow(ctx,
				`INSERT INTO hotels (org_id, name_en, city, address, phone) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				orgID, h.Name, h.City, h.Address, h.Phone,
			).Scan(&hotelID)
		}
		if err != nil {
			slog.Warn("upsert hotel", "name", h.Name, "err", err)
			continue
		}

		checkIn := parseDate(h.CheckIn)
		checkOut := parseDate(h.CheckOut)

		gh := db.GroupHotel{
			GroupID:    groupID,
			SubgroupID: subgroupID,
			HotelID:    hotelID,
			CheckIn:    checkIn,
			CheckOut:   checkOut,
		}
		if err := db.AppendGroupHotel(ctx, pool, orgID, groupID, gh); err != nil {
			slog.Warn("insert group_hotels from voucher", "hotel_id", hotelID, "err", err)
		}
	}
	return nil
}

// ListTouristUploads handles GET /api/tourists/:id/uploads
func ListTouristUploads(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")
		uploads, err := db.ListTouristUploads(r.Context(), pool, orgID, touristID)
		if err != nil {
			slog.Error("list tourist uploads", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, uploads)
	}
}

// DeleteTouristUpload handles DELETE /api/tourists/:id/uploads/:uploadId
// Deletes the DB row and removes the file from disk (best-effort — missing
// file on disk is not an error).
func DeleteTouristUpload(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")
		uploadID := chi.URLParam(r, "uploadId")

		filePath, ok, err := db.DeleteTouristUpload(r.Context(), pool, orgID, touristID, uploadID)
		if err != nil {
			slog.Error("delete tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "upload not found")
			return
		}

		if filePath != "" {
			if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("remove upload file", "path", filePath, "err", err)
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
