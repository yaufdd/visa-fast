package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/docgen"
)

// GenerateDocuments handles POST /api/groups/:id/generate
func GenerateDocuments(db *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		// Verify group exists.
		var groupExists bool
		if err := db.QueryRow(r.Context(), `SELECT true FROM groups WHERE id = $1`, groupID).Scan(&groupExists); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Load tourists (must all be confirmed).
		tRows, err := db.Query(r.Context(),
			`SELECT raw_json, matched_sheet_row FROM tourists
			  WHERE group_id = $1 AND match_confirmed = true`, groupID)
		if err != nil {
			slog.Error("fetch tourists for generate", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tRows.Close()

		var tourists []ai.TouristData
		for tRows.Next() {
			var rawJSON, matchedRow []byte
			if err := tRows.Scan(&rawJSON, &matchedRow); err != nil {
				slog.Error("scan tourist for generate", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			t := ai.TouristData{
				RawJSON: json.RawMessage(rawJSON),
			}
			if matchedRow != nil {
				var m map[string]string
				_ = json.Unmarshal(matchedRow, &m)
				t.MatchedSheetRow = m
			}
			tourists = append(tourists, t)
		}
		tRows.Close()

		if len(tourists) == 0 {
			writeError(w, http.StatusBadRequest, "no confirmed tourists found — confirm matches first")
			return
		}

		// Load hotels.
		hRows, err := db.Query(r.Context(),
			`SELECT h.name_en, COALESCE(h.address,''), COALESCE(h.phone,''), h.city,
			        gh.check_in::text, gh.check_out::text, COALESCE(gh.room_type,''), gh.sort_order
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.group_id = $1
			  ORDER BY gh.sort_order`, groupID)
		if err != nil {
			slog.Error("fetch hotels for generate", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer hRows.Close()

		var hotels []ai.HotelEntry
		for hRows.Next() {
			var h ai.HotelEntry
			if err := hRows.Scan(&h.NameEn, &h.Address, &h.Phone, &h.City,
				&h.CheckIn, &h.CheckOut, &h.RoomType, &h.SortOrder); err != nil {
				slog.Error("scan hotel for generate", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			hotels = append(hotels, h)
		}
		hRows.Close()

		if len(hotels) == 0 {
			writeError(w, http.StatusBadRequest, "no hotels configured for this group")
			return
		}

		// guide_phone can be supplied as a query param or will be left empty for Claude to note.
		guidePhone := r.URL.Query().Get("guide_phone")

		pass2Input := ai.Pass2Input{
			Tourists:   tourists,
			Hotels:     hotels,
			GuidePhone: guidePhone,
			TodayDate:  time.Now().Format("02.01.2006"),
		}

		pass2JSON, err := ai.FormatDocuments(r.Context(), apiKey, pass2Input)
		if err != nil {
			slog.Error("ai pass2", "err", err)
			writeError(w, http.StatusInternalServerError, "AI Pass 2 failed: "+err.Error())
			return
		}

		// Call Python docgen.
		zipPath, err := docgen.Generate(r.Context(), pythonScript, uploadsDir, groupID, pass2JSON)
		if err != nil {
			slog.Error("docgen", "err", err)
			writeError(w, http.StatusInternalServerError, "document generation failed: "+err.Error())
			return
		}

		// Save document record.
		now := time.Now()
		var docID string
		err = db.QueryRow(r.Context(),
			`INSERT INTO documents (group_id, pass2_json, zip_path, generated_at)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			groupID, []byte(pass2JSON), zipPath, now,
		).Scan(&docID)
		if err != nil {
			slog.Error("insert document", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Update group status.
		if _, err := db.Exec(r.Context(),
			`UPDATE groups SET status = 'completed', updated_at = now() WHERE id = $1`, groupID); err != nil {
			slog.Error("update group completed", "err", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"document_id":  docID,
			"zip_path":     zipPath,
			"generated_at": now,
		})
	}
}

// FinalizeGroup handles POST /api/groups/:id/finalize
// Generates group-level documents (для Инны в ВЦ, заявка ВЦ) for the whole group.
func FinalizeGroup(db *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		// Load latest pass2_json from documents table.
		var pass2JSON []byte
		err := db.QueryRow(r.Context(),
			`SELECT pass2_json FROM documents WHERE group_id = $1 ORDER BY generated_at DESC LIMIT 1`,
			groupID,
		).Scan(&pass2JSON)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no generated documents found — run generate first")
			return
		}

		zipPath, err := docgen.GenerateFinal(r.Context(), pythonScript, uploadsDir, groupID, json.RawMessage(pass2JSON))
		if err != nil {
			slog.Error("docgen final", "err", err)
			writeError(w, http.StatusInternalServerError, "final document generation failed: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"zip_path": zipPath})
	}
}

// GetDocuments handles GET /api/groups/:id/documents
func GetDocuments(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, pass2_json, zip_path, generated_at, created_at
			   FROM documents WHERE group_id = $1 ORDER BY created_at DESC`, groupID)
		if err != nil {
			slog.Error("fetch documents", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type Doc struct {
			ID          string          `json:"id"`
			GroupID     string          `json:"group_id"`
			Pass2JSON   json.RawMessage `json:"pass2_json"`
			ZipPath     string          `json:"zip_path"`
			GeneratedAt *time.Time      `json:"generated_at"`
			CreatedAt   time.Time       `json:"created_at"`
		}

		var docs []Doc
		for rows.Next() {
			var d Doc
			var pass2JSON []byte
			if err := rows.Scan(&d.ID, &d.GroupID, &pass2JSON, &d.ZipPath, &d.GeneratedAt, &d.CreatedAt); err != nil {
				slog.Error("scan document", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			if pass2JSON != nil {
				d.Pass2JSON = json.RawMessage(pass2JSON)
			}
			docs = append(docs, d)
		}
		if docs == nil {
			docs = []Doc{}
		}
		writeJSON(w, http.StatusOK, docs)
	}
}

// DownloadFinalZIP handles GET /api/groups/:id/download/final
func DownloadFinalZIP(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")
		var zipPath string
		err := db.QueryRow(r.Context(),
			`SELECT zip_path FROM documents WHERE group_id = $1 ORDER BY generated_at DESC LIMIT 1`,
			groupID,
		).Scan(&zipPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "no documents found")
			return
		}
		// Replace output.zip with final.zip in same dir
		finalPath := filepath.Join(filepath.Dir(zipPath), "final.zip")
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="final.zip"`)
		http.ServeFile(w, r, finalPath)
	}
}

// DownloadZIP handles GET /api/groups/:id/download
func DownloadZIP(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var zipPath string
		err := db.QueryRow(r.Context(),
			`SELECT zip_path FROM documents WHERE group_id = $1 ORDER BY generated_at DESC LIMIT 1`,
			groupID,
		).Scan(&zipPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "no documents found for this group")
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(zipPath)+`"`)
		http.ServeFile(w, r, zipPath)
	}
}
