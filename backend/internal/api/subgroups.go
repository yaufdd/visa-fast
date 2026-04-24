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

	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
)

type Subgroup struct {
	ID             string     `json:"id"`
	GroupID        string     `json:"group_id"`
	Name           string     `json:"name"`
	SortOrder      int        `json:"sort_order"`
	ProgrammeNotes *string    `json:"programme_notes"`
	CreatedAt      time.Time  `json:"created_at"`
	HasZip         bool       `json:"has_zip"`
	GeneratedAt    *time.Time `json:"generated_at,omitempty"`
}

// ListSubgroups handles GET /api/groups/:id/subgroups
func ListSubgroups(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		items, err := db.ListSubgroups(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("list subgroups", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		subgroups := make([]Subgroup, 0, len(items))
		for _, it := range items {
			s := Subgroup{
				ID:             it.ID,
				GroupID:        it.GroupID,
				Name:           it.Name,
				SortOrder:      it.SortOrder,
				ProgrammeNotes: it.ProgrammeNotes,
				CreatedAt:      it.CreatedAt,
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
		writeJSON(w, http.StatusOK, subgroups)
	}
}

// CreateSubgroup handles POST /api/groups/:id/subgroups
func CreateSubgroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		var body struct {
			Name      string `json:"name"`
			SortOrder int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}

		id, err := db.CreateSubgroup(r.Context(), pool, orgID, groupID, body.Name)
		if err != nil {
			slog.Error("create subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if id == "" {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		writeJSON(w, http.StatusCreated, Subgroup{
			ID:        id,
			GroupID:   groupID,
			Name:      body.Name,
			SortOrder: body.SortOrder,
			CreatedAt: time.Now(),
		})
	}
}

// UpdateSubgroup handles PUT /api/subgroups/:id
func UpdateSubgroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		var body struct {
			Name      string `json:"name"`
			SortOrder int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}

		ok, err := db.UpdateSubgroup(r.Context(), pool, orgID, id, body.Name, body.SortOrder)
		if err != nil {
			slog.Error("update subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// UpdateSubgroupProgrammeNotes handles PUT /api/subgroups/:id/programme_notes
// Body: {"notes": "..."} (empty string clears the notes).
func UpdateSubgroupProgrammeNotes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			Notes string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		var notesPtr *string
		if body.Notes != "" {
			notesPtr = &body.Notes
		}
		ok, err := db.UpdateSubgroupProgrammeNotes(r.Context(), pool, orgID, id, notesPtr)
		if err != nil {
			slog.Error("update subgroup programme_notes", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteSubgroup handles DELETE /api/subgroups/:id
// Tourists in this subgroup have their subgroup_id set to NULL (they stay in the group).
func DeleteSubgroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		ok, err := db.DeleteSubgroup(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("delete subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// AssignTouristSubgroup handles PUT /api/tourists/:id/subgroup
// Body: {"subgroup_id": "uuid"} or {"subgroup_id": null} to unassign.
func AssignTouristSubgroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		var body struct {
			SubgroupID *string `json:"subgroup_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}

		// If assigning, verify the target subgroup belongs to the same org.
		// If unassigning (nil), just scope the tourist update by org.
		if body.SubgroupID != nil {
			var exists bool
			if err := pool.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM subgroups WHERE id = $1 AND org_id = $2)`,
				*body.SubgroupID, orgID).Scan(&exists); err != nil {
				slog.Error("verify subgroup org", "err", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
			if !exists {
				writeError(w, http.StatusNotFound, "subgroup not found")
				return
			}
		}

		tag, err := pool.Exec(r.Context(),
			`UPDATE tourists SET subgroup_id = $1, updated_at = now()
			   WHERE id = $2 AND org_id = $3`,
			body.SubgroupID, touristID, orgID)
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
