package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
)

// allowedGroupStatuses is the canonical set of statuses a manager may set.
var allowedGroupStatuses = map[string]bool{
	"draft":       true,
	"in_progress": true,
	"docs_ready":  true,
	"submitted":   true,
	"visa_issued": true,
}

// ListGroups handles GET /api/groups
func ListGroups(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groups, err := db.ListGroups(r.Context(), pool, orgID)
		if err != nil {
			slog.Error("list groups", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, groups)
	}
}

// CreateGroup handles POST /api/groups
func CreateGroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}

		id, err := db.CreateGroup(r.Context(), pool, orgID, body.Name)
		if err != nil {
			slog.Error("insert group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		g, err := db.GetGroup(r.Context(), pool, orgID, id)
		if err != nil || g == nil {
			slog.Error("load created group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, g)
	}
}

// DeleteGroup handles DELETE /api/groups/:id — cascades to tourists, uploads, hotels, documents.
func DeleteGroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		ok, err := db.DeleteGroup(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("delete group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetGroup handles GET /api/groups/:id — returns group with tourists and hotels.
func GetGroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")

		g, err := db.GetGroup(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("get group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if g == nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Tourists.
		tRows, err := pool.Query(r.Context(),
			`SELECT id, group_id, subgroup_id, submission_id, submission_snapshot, flight_data, created_at, updated_at
			   FROM tourists WHERE group_id = $1 ORDER BY created_at`, id)
		if err != nil {
			slog.Error("fetch tourists", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tRows.Close()

		type Tourist struct {
			ID                 string          `json:"id"`
			GroupID            string          `json:"group_id"`
			SubgroupID         *string         `json:"subgroup_id"`
			SubmissionID       *string         `json:"submission_id"`
			SubmissionSnapshot json.RawMessage `json:"submission_snapshot"`
			FlightData         json.RawMessage `json:"flight_data"`
			CreatedAt          time.Time       `json:"created_at"`
			UpdatedAt          time.Time       `json:"updated_at"`
		}

		var tourists []Tourist
		for tRows.Next() {
			var t Tourist
			var snap, flight []byte
			if err := tRows.Scan(&t.ID, &t.GroupID, &t.SubgroupID, &t.SubmissionID, &snap, &flight, &t.CreatedAt, &t.UpdatedAt); err != nil {
				slog.Error("scan tourist", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			if snap != nil {
				t.SubmissionSnapshot = json.RawMessage(snap)
			}
			if flight != nil {
				t.FlightData = json.RawMessage(flight)
			}
			tourists = append(tourists, t)
		}
		if tourists == nil {
			tourists = []Tourist{}
		}

		// Hotels.
		hRows, err := pool.Query(r.Context(),
			`SELECT gh.id, gh.hotel_id, h.name_en, h.city, COALESCE(h.address,''), COALESCE(h.phone,''),
			        gh.check_in::text, gh.check_out::text, COALESCE(gh.room_type,''), gh.sort_order
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.group_id = $1
			  ORDER BY gh.sort_order`, id)
		if err != nil {
			slog.Error("fetch group hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer hRows.Close()

		type GroupHotel struct {
			ID        string `json:"id"`
			HotelID   string `json:"hotel_id"`
			NameEn    string `json:"name_en"`
			City      string `json:"city"`
			Address   string `json:"address"`
			Phone     string `json:"phone"`
			CheckIn   string `json:"check_in"`
			CheckOut  string `json:"check_out"`
			RoomType  string `json:"room_type"`
			SortOrder int    `json:"sort_order"`
		}

		var hotels []GroupHotel
		for hRows.Next() {
			var h GroupHotel
			if err := hRows.Scan(&h.ID, &h.HotelID, &h.NameEn, &h.City, &h.Address, &h.Phone,
				&h.CheckIn, &h.CheckOut, &h.RoomType, &h.SortOrder); err != nil {
				slog.Error("scan group hotel", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			hotels = append(hotels, h)
		}
		if hotels == nil {
			hotels = []GroupHotel{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"group":    g,
			"tourists": tourists,
			"hotels":   hotels,
		})
	}
}

// UpdateGroupStatus handles PUT /api/groups/{id}/status.
// Body: {"status": "in_progress"}. Status is manager-driven only.
func UpdateGroupStatus(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if !allowedGroupStatuses[body.Status] {
			writeError(w, http.StatusBadRequest, "invalid status value")
			return
		}

		ok, err := db.UpdateGroupStatus(r.Context(), pool, orgID, id, body.Status)
		if err != nil {
			slog.Error("update group status", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		g, err := db.GetGroup(r.Context(), pool, orgID, id)
		if err != nil || g == nil {
			slog.Error("load updated group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}

// UpdateGroupName handles PUT /api/groups/{id}/name.
// Body: {"name": "..."}.
func UpdateGroupName(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}
		ok, err := db.UpdateGroupName(r.Context(), pool, orgID, id, body.Name)
		if err != nil {
			slog.Error("update group name", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		g, err := db.GetGroup(r.Context(), pool, orgID, id)
		if err != nil || g == nil {
			slog.Error("load updated group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}

// UpdateGroupNotes handles PUT /api/groups/{id}/notes.
// Body: {"notes": "..."}. Empty string is allowed.
func UpdateGroupNotes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			Notes string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		notes := &body.Notes
		ok, err := db.UpdateGroupNotes(r.Context(), pool, orgID, id, notes)
		if err != nil {
			slog.Error("update group notes", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		g, err := db.GetGroup(r.Context(), pool, orgID, id)
		if err != nil || g == nil {
			slog.Error("load updated group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}

// UpdateGroupProgrammeNotes handles PUT /api/groups/{id}/programme_notes.
// Body: {"notes": "..."}. Empty string clears the notes (stored as SQL NULL).
func UpdateGroupProgrammeNotes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		var body struct {
			Notes string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		var notesPtr *string
		if body.Notes != "" {
			notesPtr = &body.Notes
		}
		ok, err := db.UpdateGroupProgrammeNotes(r.Context(), pool, orgID, id, notesPtr)
		if err != nil {
			slog.Error("update group programme_notes", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		g, err := db.GetGroup(r.Context(), pool, orgID, id)
		if err != nil || g == nil {
			slog.Error("load updated group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}
