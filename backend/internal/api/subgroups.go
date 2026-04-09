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
)

type Subgroup struct {
	ID          string     `json:"id"`
	GroupID     string     `json:"group_id"`
	Name        string     `json:"name"`
	SortOrder   int        `json:"sort_order"`
	CreatedAt   time.Time  `json:"created_at"`
	HasZip      bool       `json:"has_zip"`
	GeneratedAt *time.Time `json:"generated_at,omitempty"`
}

// ListSubgroups handles GET /api/groups/:id/subgroups
func ListSubgroups(db *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")
		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, name, sort_order, created_at
			   FROM subgroups WHERE group_id = $1 ORDER BY sort_order, created_at`,
			groupID)
		if err != nil {
			slog.Error("list subgroups", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var subgroups []Subgroup
		for rows.Next() {
			var s Subgroup
			if err := rows.Scan(&s.ID, &s.GroupID, &s.Name, &s.SortOrder, &s.CreatedAt); err != nil {
				slog.Error("scan subgroup", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			// Check if a generated ZIP exists on disk.
			zipPath := filepath.Join(uploadsDir, s.GroupID, "subgroup_"+s.ID+".zip")
			if info, err := os.Stat(zipPath); err == nil && !info.IsDir() {
				s.HasZip = true
				mt := info.ModTime()
				s.GeneratedAt = &mt
			}
			subgroups = append(subgroups, s)
		}
		if subgroups == nil {
			subgroups = []Subgroup{}
		}
		writeJSON(w, http.StatusOK, subgroups)
	}
}

// CreateSubgroup handles POST /api/groups/:id/subgroups
func CreateSubgroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var body struct {
			Name      string `json:"name"`
			SortOrder int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}

		var s Subgroup
		err := db.QueryRow(r.Context(),
			`INSERT INTO subgroups (group_id, name, sort_order)
			 VALUES ($1, $2, $3) RETURNING id, group_id, name, sort_order, created_at`,
			groupID, body.Name, body.SortOrder,
		).Scan(&s.ID, &s.GroupID, &s.Name, &s.SortOrder, &s.CreatedAt)
		if err != nil {
			slog.Error("create subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, s)
	}
}

// UpdateSubgroup handles PUT /api/subgroups/:id
func UpdateSubgroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}

		tag, err := db.Exec(r.Context(),
			`UPDATE subgroups SET name = $1 WHERE id = $2`, body.Name, id)
		if err != nil {
			slog.Error("update subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteSubgroup handles DELETE /api/subgroups/:id
// Tourists in this subgroup have their subgroup_id set to NULL (they stay in the group).
func DeleteSubgroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tag, err := db.Exec(r.Context(), `DELETE FROM subgroups WHERE id = $1`, id)
		if err != nil {
			slog.Error("delete subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// AssignTouristSubgroup handles PUT /api/tourists/:id/subgroup
// Body: {"subgroup_id": "uuid"} or {"subgroup_id": null} to unassign.
func AssignTouristSubgroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")

		var body struct {
			SubgroupID *string `json:"subgroup_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}

		tag, err := db.Exec(r.Context(),
			`UPDATE tourists SET subgroup_id = $1, updated_at = now() WHERE id = $2`,
			body.SubgroupID, touristID)
		if err != nil {
			slog.Error("assign tourist subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
