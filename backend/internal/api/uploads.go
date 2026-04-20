package api

import (
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
func UploadTouristFile(db *pgxpool.Pool, uploadsDir, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")

		var groupID string
		var subgroupID *string
		if err := db.QueryRow(r.Context(),
			`SELECT group_id, subgroup_id FROM tourists WHERE id = $1`, touristID,
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

		var uploadID string
		var createdAt time.Time
		err = db.QueryRow(r.Context(),
			`INSERT INTO uploads (group_id, tourist_id, file_type, file_path, anthropic_file_id)
			 VALUES ($1, $2, $3, $4, NULLIF($5,'')) RETURNING id, created_at`,
			groupID, touristID, fileType, savedPath, anthropicFileID,
		).Scan(&uploadID, &createdAt)
		if err != nil {
			slog.Error("insert tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

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
			if parseErr := parseTicketAndPersist(r, db, apiKey, touristID, fileInput); parseErr != nil {
				slog.Warn("ticket parse failed", "tourist_id", touristID, "err", parseErr)
				resp["parse_error"] = parseErr.Error()
			}
		case "voucher":
			if parseErr := parseVoucherAndPersist(r, db, apiKey, touristID, fileInput); parseErr != nil {
				slog.Warn("voucher parse failed", "tourist_id", touristID, "err", parseErr)
				resp["parse_error"] = parseErr.Error()
			}
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// parseTicketAndPersist calls the ticket parser and writes the result into
// tourists.flight_data.
func parseTicketAndPersist(r *http.Request, db *pgxpool.Pool, apiKey, touristID string, input ai.FileInput) error {
	flights, err := ai.ParseTicket(r.Context(), apiKey, []ai.FileInput{input})
	if err != nil {
		return err
	}
	buf, _ := json.Marshal(map[string]any{
		"arrival":   flights.Arrival,
		"departure": flights.Departure,
	})
	if _, err := db.Exec(r.Context(),
		`UPDATE tourists SET flight_data = $1, updated_at = NOW() WHERE id = $2`,
		buf, touristID,
	); err != nil {
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

// parseVoucherAndPersist calls the voucher parser and, for each hotel,
// looks up (or creates) the hotels row and inserts a group_hotels row
// scoped to the tourist's group_id/subgroup_id.
func parseVoucherAndPersist(r *http.Request, db *pgxpool.Pool, apiKey, touristID string, input ai.FileInput) error {
	hotels, err := ai.ParseVouchers(r.Context(), apiKey, []ai.FileInput{input})
	if err != nil {
		return err
	}
	for _, h := range hotels {
		var hotelID string
		err := db.QueryRow(r.Context(),
			`SELECT id FROM hotels WHERE LOWER(name_en) = LOWER($1) LIMIT 1`, h.Name,
		).Scan(&hotelID)
		if err != nil && errors.Is(err, pgx.ErrNoRows) {
			err = db.QueryRow(r.Context(),
				`INSERT INTO hotels (name_en, city, address, phone) VALUES ($1, $2, $3, $4) RETURNING id`,
				h.Name, h.City, h.Address, h.Phone,
			).Scan(&hotelID)
		}
		if err != nil {
			slog.Warn("upsert hotel", "name", h.Name, "err", err)
			continue
		}

		checkIn := convertDate(h.CheckIn)
		checkOut := convertDate(h.CheckOut)

		if _, err := db.Exec(r.Context(),
			`INSERT INTO group_hotels (group_id, subgroup_id, hotel_id, check_in, check_out, sort_order)
			 SELECT t.group_id, t.subgroup_id, $1, NULLIF($2,'')::date, NULLIF($3,'')::date,
			        COALESCE((SELECT MAX(sort_order) + 1 FROM group_hotels gh WHERE gh.group_id = t.group_id), 1)
			   FROM tourists t WHERE t.id = $4`,
			hotelID, checkIn, checkOut, touristID,
		); err != nil {
			slog.Warn("insert group_hotels from voucher", "hotel_id", hotelID, "err", err)
		}
	}
	return nil
}

// ListTouristUploads handles GET /api/tourists/:id/uploads
func ListTouristUploads(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")
		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, COALESCE(tourist_id::text,''), file_type, file_path, created_at
			   FROM uploads WHERE tourist_id = $1 ORDER BY created_at`,
			touristID,
		)
		if err != nil {
			slog.Error("list tourist uploads", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type uploadRow struct {
			ID        string    `json:"id"`
			GroupID   string    `json:"group_id"`
			TouristID string    `json:"tourist_id"`
			FileType  string    `json:"file_type"`
			FilePath  string    `json:"file_path"`
			CreatedAt time.Time `json:"created_at"`
		}
		var uploads []uploadRow
		for rows.Next() {
			var u uploadRow
			if err := rows.Scan(&u.ID, &u.GroupID, &u.TouristID, &u.FileType, &u.FilePath, &u.CreatedAt); err != nil {
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			uploads = append(uploads, u)
		}
		if uploads == nil {
			uploads = []uploadRow{}
		}
		writeJSON(w, http.StatusOK, uploads)
	}
}
