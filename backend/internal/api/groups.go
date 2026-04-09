package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListGroups handles GET /api/groups
func ListGroups(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(r.Context(),
			`SELECT id, name, status, created_at, updated_at FROM groups ORDER BY created_at DESC`)
		if err != nil {
			slog.Error("list groups query", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var groups []Group
		for rows.Next() {
			var g Group
			if err := rows.Scan(&g.ID, &g.Name, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
				slog.Error("scan group row", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			groups = append(groups, g)
		}
		if groups == nil {
			groups = []Group{}
		}
		writeJSON(w, http.StatusOK, groups)
	}
}

// CreateGroup handles POST /api/groups
func CreateGroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "field 'name' is required")
			return
		}

		var g Group
		err := db.QueryRow(r.Context(),
			`INSERT INTO groups (name) VALUES ($1) RETURNING id, name, status, created_at, updated_at`,
			body.Name,
		).Scan(&g.ID, &g.Name, &g.Status, &g.CreatedAt, &g.UpdatedAt)
		if err != nil {
			slog.Error("insert group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, g)
	}
}

// DeleteGroup handles DELETE /api/groups/:id — cascades to tourists, uploads, hotels, documents.
func DeleteGroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tag, err := db.Exec(r.Context(), `DELETE FROM groups WHERE id = $1`, id)
		if err != nil {
			slog.Error("delete group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetGroup handles GET /api/groups/:id — returns group with tourists and hotels.
func GetGroup(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var g Group
		err := db.QueryRow(r.Context(),
			`SELECT id, name, status, created_at, updated_at FROM groups WHERE id = $1`, id,
		).Scan(&g.ID, &g.Name, &g.Status, &g.CreatedAt, &g.UpdatedAt)
		if err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Tourists.
		tRows, err := db.Query(r.Context(),
			`SELECT id, group_id, raw_json, matched_sheet_row, match_confirmed, created_at, updated_at
			   FROM tourists WHERE group_id = $1 ORDER BY created_at`, id)
		if err != nil {
			slog.Error("fetch tourists", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tRows.Close()

		type Tourist struct {
			ID              string          `json:"id"`
			GroupID         string          `json:"group_id"`
			RawJSON         json.RawMessage `json:"raw_json"`
			MatchedSheetRow json.RawMessage `json:"matched_sheet_row"`
			MatchConfirmed  bool            `json:"match_confirmed"`
			CreatedAt       time.Time       `json:"created_at"`
			UpdatedAt       time.Time       `json:"updated_at"`
		}

		var tourists []Tourist
		for tRows.Next() {
			var t Tourist
			var rawJSON, matchedRow []byte
			if err := tRows.Scan(&t.ID, &t.GroupID, &rawJSON, &matchedRow, &t.MatchConfirmed, &t.CreatedAt, &t.UpdatedAt); err != nil {
				slog.Error("scan tourist", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			if rawJSON != nil {
				t.RawJSON = json.RawMessage(rawJSON)
			}
			if matchedRow != nil {
				t.MatchedSheetRow = json.RawMessage(matchedRow)
			}
			tourists = append(tourists, t)
		}
		if tourists == nil {
			tourists = []Tourist{}
		}

		// Hotels.
		hRows, err := db.Query(r.Context(),
			`SELECT gh.id, gh.hotel_id, h.name_en, COALESCE(h.name_ru,''), h.city, COALESCE(h.address,''), COALESCE(h.phone,''),
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
			NameRu    string `json:"name_ru"`
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
			if err := hRows.Scan(&h.ID, &h.HotelID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone,
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
