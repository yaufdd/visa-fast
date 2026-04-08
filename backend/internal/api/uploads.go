package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/storage"
)

const maxUploadSize = 32 << 20 // 32 MB

var allowedFileTypes = map[string]bool{
	"passport":         true,
	"foreign_passport": true,
	"ticket":           true,
	"voucher":          true,
	"unknown":          true,
}

// ListUploads handles GET /api/groups/:id/uploads
func ListUploads(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")
		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, COALESCE(tourist_id::text,''), file_type, file_path, created_at
			   FROM uploads WHERE group_id = $1 ORDER BY created_at`,
			groupID,
		)
		if err != nil {
			slog.Error("list uploads", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type uploadRow struct {
			ID        string    `json:"id"`
			GroupID   string    `json:"group_id"`
			TouristID string    `json:"tourist_id,omitempty"`
			FileType  string    `json:"file_type"`
			FilePath  string    `json:"file_path"`
			CreatedAt time.Time `json:"created_at"`
		}
		var uploads []uploadRow
		for rows.Next() {
			var u uploadRow
			if err := rows.Scan(&u.ID, &u.GroupID, &u.TouristID, &u.FileType, &u.FilePath, &u.CreatedAt); err != nil {
				slog.Error("scan upload", "err", err)
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

// UploadTouristFile handles POST /api/tourists/:id/uploads
func UploadTouristFile(db *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")

		var groupID string
		if err := db.QueryRow(r.Context(), `SELECT group_id FROM tourists WHERE id = $1`, touristID).Scan(&groupID); err != nil {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}

		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'file' field in form")
			return
		}
		defer file.Close()

		savedPath, err := storage.SaveFile(uploadsDir, groupID, "unknown", header.Filename, file)
		if err != nil {
			slog.Error("save tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save file")
			return
		}

		var uploadID string
		var createdAt time.Time
		err = db.QueryRow(r.Context(),
			`INSERT INTO uploads (group_id, tourist_id, file_type, file_path) VALUES ($1, $2, 'unknown', $3) RETURNING id, created_at`,
			groupID, touristID, savedPath,
		).Scan(&uploadID, &createdAt)
		if err != nil {
			slog.Error("insert tourist upload", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":          uploadID,
			"tourist_id":  touristID,
			"group_id":    groupID,
			"file_path":   savedPath,
			"filename":    header.Filename,
			"created_at":  createdAt,
		})
	}
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

// UploadFile handles POST /api/groups/:id/uploads
func UploadFile(db *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		// Verify group exists.
		var exists bool
		if err := db.QueryRow(r.Context(), `SELECT true FROM groups WHERE id = $1`, groupID).Scan(&exists); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}

		fileType := r.FormValue("file_type")
		if !allowedFileTypes[fileType] {
			writeError(w, http.StatusBadRequest, "file_type must be one of: passport, ticket, voucher")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'file' field in form")
			return
		}
		defer file.Close()

		savedPath, err := storage.SaveFile(uploadsDir, groupID, fileType, header.Filename, file)
		if err != nil {
			slog.Error("save upload file", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to save file")
			return
		}

		var uploadID string
		var createdAt time.Time
		err = db.QueryRow(r.Context(),
			`INSERT INTO uploads (group_id, file_type, file_path) VALUES ($1, $2, $3) RETURNING id, created_at`,
			groupID, fileType, savedPath,
		).Scan(&uploadID, &createdAt)
		if err != nil {
			slog.Error("insert upload record", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         uploadID,
			"group_id":   groupID,
			"file_type":  fileType,
			"file_path":  savedPath,
			"created_at": createdAt,
		})
	}
}
