package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Hotel struct {
	ID        string    `json:"id"`
	NameEn    string    `json:"name_en"`
	NameRu    string    `json:"name_ru"`
	City      string    `json:"city"`
	Address   string    `json:"address"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListHotels handles GET /api/hotels
func ListHotels(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(r.Context(),
			`SELECT id, name_en, COALESCE(name_ru,''), city, COALESCE(address,''), COALESCE(phone,''), created_at, updated_at
			   FROM hotels ORDER BY city, name_en`)
		if err != nil {
			slog.Error("list hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var hotels []Hotel
		for rows.Next() {
			var h Hotel
			if err := rows.Scan(&h.ID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone, &h.CreatedAt, &h.UpdatedAt); err != nil {
				slog.Error("scan hotel", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			hotels = append(hotels, h)
		}
		if hotels == nil {
			hotels = []Hotel{}
		}
		writeJSON(w, http.StatusOK, hotels)
	}
}

// CreateHotel handles POST /api/hotels
func CreateHotel(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			NameEn  string `json:"name_en"`
			NameRu  string `json:"name_ru"`
			City    string `json:"city"`
			Address string `json:"address"`
			Phone   string `json:"phone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.NameEn == "" || body.City == "" {
			writeError(w, http.StatusBadRequest, "fields 'name_en' and 'city' are required")
			return
		}

		var h Hotel
		err := db.QueryRow(r.Context(),
			`INSERT INTO hotels (name_en, name_ru, city, address, phone)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, name_en, COALESCE(name_ru,''), city, COALESCE(address,''), COALESCE(phone,''), created_at, updated_at`,
			body.NameEn, body.NameRu, body.City, body.Address, body.Phone,
		).Scan(&h.ID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone, &h.CreatedAt, &h.UpdatedAt)
		if err != nil {
			slog.Error("insert hotel", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, h)
	}
}

// ListGroupHotels handles GET /api/groups/:id/hotels
func ListGroupHotels(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")
		rows, err := db.Query(r.Context(),
			`SELECT gh.id, gh.hotel_id, h.name_en, COALESCE(h.name_ru,''), h.city,
			        COALESCE(h.address,''), COALESCE(h.phone,''),
			        gh.check_in, gh.check_out, COALESCE(gh.room_type,''), gh.sort_order
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.group_id = $1
			  ORDER BY gh.sort_order`,
			groupID,
		)
		if err != nil {
			slog.Error("list group hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type groupHotelRow struct {
			ID         string `json:"id"`
			HotelID    string `json:"hotel_id"`
			HotelName  string `json:"hotel_name"`
			HotelNameRu string `json:"hotel_name_ru"`
			City       string `json:"city"`
			Address    string `json:"address"`
			Phone      string `json:"phone"`
			CheckIn    string `json:"check_in"`
			CheckOut   string `json:"check_out"`
			RoomType   string `json:"room_type"`
			SortOrder  int    `json:"sort_order"`
		}
		var result []groupHotelRow
		for rows.Next() {
			var gh groupHotelRow
			if err := rows.Scan(
				&gh.ID, &gh.HotelID, &gh.HotelName, &gh.HotelNameRu, &gh.City,
				&gh.Address, &gh.Phone, &gh.CheckIn, &gh.CheckOut, &gh.RoomType, &gh.SortOrder,
			); err != nil {
				slog.Error("scan group hotel", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			result = append(result, gh)
		}
		if result == nil {
			result = []groupHotelRow{}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// UpsertGroupHotels handles POST /api/groups/:id/hotels
// Body: [{hotel_id, check_in, check_out, room_type, sort_order}]
func UpsertGroupHotels(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var groupExists bool
		if err := db.QueryRow(r.Context(), `SELECT true FROM groups WHERE id = $1`, groupID).Scan(&groupExists); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		var entries []struct {
			HotelID   string `json:"hotel_id"`
			CheckIn   string `json:"check_in"`
			CheckOut  string `json:"check_out"`
			RoomType  string `json:"room_type"`
			SortOrder int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON array body")
			return
		}
		if len(entries) == 0 {
			writeError(w, http.StatusBadRequest, "at least one hotel entry is required")
			return
		}

		tx, err := db.Begin(r.Context())
		if err != nil {
			slog.Error("begin tx", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tx.Rollback(r.Context())

		// Replace all existing entries for this group.
		if _, err := tx.Exec(r.Context(), `DELETE FROM group_hotels WHERE group_id = $1`, groupID); err != nil {
			slog.Error("delete group hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		for _, e := range entries {
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO group_hotels (group_id, hotel_id, check_in, check_out, room_type, sort_order)
				 VALUES ($1, $2, $3::date, $4::date, $5, $6)`,
				groupID, e.HotelID, e.CheckIn, e.CheckOut, e.RoomType, e.SortOrder,
			); err != nil {
				slog.Error("insert group hotel", "err", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			slog.Error("commit group hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"inserted": len(entries)})
	}
}
