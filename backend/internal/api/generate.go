package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/docgen"
)

// loadGroupHotels loads hotels for a group from the DB.
func loadGroupHotels(ctx interface{ Done() <-chan struct{} }, db *pgxpool.Pool, groupID string, r *http.Request) ([]ai.HotelEntry, error) {
	hRows, err := db.Query(r.Context(),
		`SELECT h.name_en, COALESCE(h.address,''), COALESCE(h.phone,''), h.city,
		        gh.check_in::text, gh.check_out::text, COALESCE(gh.room_type,''), gh.sort_order
		   FROM group_hotels gh
		   JOIN hotels h ON h.id = gh.hotel_id
		  WHERE gh.group_id = $1
		  ORDER BY gh.sort_order`, groupID)
	if err != nil {
		return nil, err
	}
	defer hRows.Close()
	var hotels []ai.HotelEntry
	for hRows.Next() {
		var h ai.HotelEntry
		if err := hRows.Scan(&h.NameEn, &h.Address, &h.Phone, &h.City,
			&h.CheckIn, &h.CheckOut, &h.RoomType, &h.SortOrder); err != nil {
			return nil, err
		}
		hotels = append(hotels, h)
	}
	return hotels, nil
}

// loadTouristsForSubgroup loads confirmed tourists for a subgroup (or whole group if subgroupID=="").
func loadTouristsForSubgroup(db *pgxpool.Pool, r *http.Request, groupID, subgroupID string) ([]ai.TouristData, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	if subgroupID != "" {
		rows, err = db.Query(r.Context(),
			`SELECT raw_json, matched_sheet_row FROM tourists
			  WHERE group_id = $1 AND subgroup_id = $2 AND match_confirmed = true
			  ORDER BY created_at`, groupID, subgroupID)
	} else {
		rows, err = db.Query(r.Context(),
			`SELECT raw_json, matched_sheet_row FROM tourists
			  WHERE group_id = $1 AND match_confirmed = true
			  ORDER BY created_at`, groupID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tourists []ai.TouristData
	for rows.Next() {
		var rawJSON, matchedRow []byte
		if err := rows.Scan(&rawJSON, &matchedRow); err != nil {
			return nil, err
		}
		t := ai.TouristData{RawJSON: json.RawMessage(rawJSON)}
		if matchedRow != nil {
			var m map[string]string
			_ = json.Unmarshal(matchedRow, &m)
			t.MatchedSheetRow = m
		}
		tourists = append(tourists, t)
	}
	return tourists, nil
}

// GenerateDocuments handles POST /api/groups/:id/generate
// Supports subgroups: each subgroup gets its own folder in output.zip.
func GenerateDocuments(db *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var groupName string
		if err := db.QueryRow(r.Context(), `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&groupName); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Load subgroups.
		sgRows, err := db.Query(r.Context(),
			`SELECT id, name FROM subgroups WHERE group_id = $1 ORDER BY sort_order, created_at`, groupID)
		if err != nil {
			slog.Error("fetch subgroups", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		type subgroupEntry struct{ id, name string }
		var subgroups []subgroupEntry
		for sgRows.Next() {
			var sg subgroupEntry
			if err := sgRows.Scan(&sg.id, &sg.name); err == nil {
				subgroups = append(subgroups, sg)
			}
		}
		sgRows.Close()

		// If no subgroups defined, treat whole group as one subgroup named after the group.
		if len(subgroups) == 0 {
			subgroups = []subgroupEntry{{id: "", name: groupName}}
		}

		// Load shared hotels.
		hotels, err := loadGroupHotels(nil, db, groupID, r)
		if err != nil {
			slog.Error("fetch hotels for generate", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if len(hotels) == 0 {
			writeError(w, http.StatusBadRequest, "no hotels configured for this group")
			return
		}

		guidePhone := r.URL.Query().Get("guide_phone")
		now := time.Now()

		// Clean docs dir before generating.
		docsDir := filepath.Join(uploadsDir, groupID, "docs")
		_ = os.RemoveAll(docsDir)

		var lastPass2JSON json.RawMessage

		for _, sg := range subgroups {
			tourists, err := loadTouristsForSubgroup(db, r, groupID, sg.id)
			if err != nil {
				slog.Error("fetch tourists for subgroup", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
			if len(tourists) == 0 {
				slog.Warn("subgroup has no confirmed tourists, skipping", "subgroup", sg.name)
				continue
			}

			pass2Input := ai.Pass2Input{
				Tourists:   tourists,
				Hotels:     hotels,
				GuidePhone: guidePhone,
				TodayDate:  now.Format("02.01.2006"),
			}

			pass2JSON, err := ai.FormatDocuments(r.Context(), apiKey, pass2Input)
			if err != nil {
				slog.Error("ai pass2", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "AI Pass 2 failed for "+sg.name+": "+err.Error())
				return
			}
			lastPass2JSON = pass2JSON

			if err := docgen.GenerateWithSubgroup(r.Context(), pythonScript, uploadsDir, groupID, sg.name, pass2JSON); err != nil {
				slog.Error("docgen subgroup", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "document generation failed for "+sg.name+": "+err.Error())
				return
			}
		}

		// Pack all docs/{subgroup}/ folders into one ZIP.
		zipPath, err := docgen.ZipDocsDir(uploadsDir, groupID, "output.zip")
		if err != nil {
			slog.Error("zip docs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to create zip: "+err.Error())
			return
		}

		// Save last pass2_json (for finalize step).
		var docID string
		err = db.QueryRow(r.Context(),
			`INSERT INTO documents (group_id, pass2_json, zip_path, generated_at)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			groupID, []byte(lastPass2JSON), zipPath, now,
		).Scan(&docID)
		if err != nil {
			slog.Error("insert document", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

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
