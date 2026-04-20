package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
	"fujitravel-admin/backend/internal/storage"
)

const maxUploadSize = 32 << 20 // 32 MB

// Only ticket/voucher uploads are accepted in the custom-form workflow.
var allowedTouristFileTypes = map[string]bool{
	"ticket":  true,
	"voucher": true,
}

// UploadTouristFile handles POST /api/tourists/:id/uploads.
// It saves the file to disk, caches it on the Anthropic Files API and then
// SYNCHRONOUSLY invokes the matching parser (ticket or voucher) so that the
// tourist's flight data / group hotels are populated immediately.
//
// If the parser fails we still return 200 with a "parse_error" field — the
// raw scan is already saved and the operator can retry later.
func UploadTouristFile(pool *pgxpool.Pool, uploadsDir, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		var groupID string
		var subgroupID *string
		if err := pool.QueryRow(r.Context(),
			`SELECT group_id, subgroup_id FROM tourists WHERE id = $1 AND org_id = $2`,
			touristID, orgID,
		).Scan(&groupID, &subgroupID); err != nil {
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

		// Upload to Anthropic Files API (non-fatal if it fails — inline fallback).
		var anthropicFileID string
		if fid, err := ai.UploadFileToAnthropic(apiKey, header.Filename, fileData); err != nil {
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
		createdAt := time.Now()

		// Run the matching parser synchronously.
		resp := map[string]any{
			"id":         uploadID,
			"tourist_id": touristID,
			"group_id":   groupID,
			"file_type":  fileType,
			"file_path":  savedPath,
			"filename":   header.Filename,
			"created_at": createdAt,
		}

		fileInput := ai.FileInput{
			AnthropicFileID: anthropicFileID,
			Name:            header.Filename,
			Data:            fileData,
		}

		switch fileType {
		case "ticket":
			if parseErr := parseTicketAndPersist(r.Context(), pool, apiKey, orgID, touristID, fileInput); parseErr != nil {
				slog.Warn("ticket parse failed", "tourist_id", touristID, "err", parseErr)
				resp["parse_error"] = parseErr.Error()
			}
		case "voucher":
			if parseErr := parseVoucherAndPersist(r.Context(), pool, apiKey, orgID, groupID, subgroupID, fileInput); parseErr != nil {
				slog.Warn("voucher parse failed", "tourist_id", touristID, "err", parseErr)
				resp["parse_error"] = parseErr.Error()
			}
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// parseTicketAndPersist calls the ticket parser and writes the result into
// tourists.flight_data (scoped to org).
func parseTicketAndPersist(ctx context.Context, pool *pgxpool.Pool, apiKey, orgID, touristID string, input ai.FileInput) error {
	flights, err := ai.ParseTicket(ctx, apiKey, []ai.FileInput{input})
	if err != nil {
		return err
	}
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
