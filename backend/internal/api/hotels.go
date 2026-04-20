package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
)

// strPtr returns nil for empty strings, otherwise a pointer to s.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ListHotels handles GET /api/hotels
func ListHotels(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		hotels, err := db.ListHotels(r.Context(), pool, orgID)
		if err != nil {
			slog.Error("list hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, hotels)
	}
}

// CreateHotel handles POST /api/hotels
func CreateHotel(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
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

		id, err := db.CreateHotel(r.Context(), pool, orgID, db.Hotel{
			NameEn:  body.NameEn,
			NameRu:  strPtr(body.NameRu),
			City:    strPtr(body.City),
			Address: strPtr(body.Address),
			Phone:   strPtr(body.Phone),
		})
		if err != nil {
			slog.Error("insert hotel", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		h, err := db.GetHotel(r.Context(), pool, orgID, id)
		if err != nil || h == nil {
			slog.Error("load created hotel", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, h)
	}
}

// GetHotel handles GET /api/hotels/:id
func GetHotel(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
		h, err := db.GetHotel(r.Context(), pool, orgID, id)
		if err != nil {
			slog.Error("get hotel", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if h == nil {
			writeError(w, http.StatusNotFound, "hotel not found")
			return
		}
		writeJSON(w, http.StatusOK, h)
	}
}

// UpdateHotel handles PUT /api/hotels/:id
// Only the calling org's private hotels can be updated; global hotels
// (org_id IS NULL) are read-only and return 404.
func UpdateHotel(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		id := chi.URLParam(r, "id")
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
		if body.NameEn == "" {
			writeError(w, http.StatusBadRequest, "field 'name_en' is required")
			return
		}

		ok, err := db.UpdateHotel(r.Context(), pool, orgID, id, db.Hotel{
			NameEn:  body.NameEn,
			NameRu:  strPtr(body.NameRu),
			City:    strPtr(body.City),
			Address: strPtr(body.Address),
			Phone:   strPtr(body.Phone),
		})
		if err != nil {
			slog.Error("update hotel", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "hotel not found")
			return
		}
		h, err := db.GetHotel(r.Context(), pool, orgID, id)
		if err != nil || h == nil {
			slog.Error("load updated hotel", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, h)
	}
}

// ListGroupHotels handles GET /api/groups/:id/hotels
func ListGroupHotels(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		// Scope by verifying the group belongs to this org. If not, 404.
		var exists bool
		if err := pool.QueryRow(r.Context(),
			`SELECT true FROM groups WHERE id = $1 AND org_id = $2`, groupID, orgID).Scan(&exists); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		rows, err := pool.Query(r.Context(),
			`SELECT gh.id, gh.hotel_id, h.name_en, COALESCE(h.name_ru,''), COALESCE(h.city,''),
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
			ID          string `json:"id"`
			HotelID     string `json:"hotel_id"`
			HotelName   string `json:"hotel_name"`
			HotelNameRu string `json:"hotel_name_ru"`
			City        string `json:"city"`
			Address     string `json:"address"`
			Phone       string `json:"phone"`
			CheckIn     string `json:"check_in"`
			CheckOut    string `json:"check_out"`
			RoomType    string `json:"room_type"`
			SortOrder   int    `json:"sort_order"`
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

// ListSubgroupHotels handles GET /api/subgroups/:id/hotels
func ListSubgroupHotels(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		subgroupID := chi.URLParam(r, "id")

		// Scope: the subgroup's parent group must belong to this org.
		var exists bool
		if err := pool.QueryRow(r.Context(),
			`SELECT true FROM subgroups s
			   JOIN groups g ON g.id = s.group_id
			  WHERE s.id = $1 AND g.org_id = $2`, subgroupID, orgID).Scan(&exists); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}

		rows, err := pool.Query(r.Context(),
			`SELECT gh.id, gh.hotel_id, h.name_en, COALESCE(h.name_ru,''), COALESCE(h.city,''),
			        COALESCE(h.address,''), COALESCE(h.phone,''),
			        gh.check_in::text, gh.check_out::text, COALESCE(gh.room_type,''), gh.sort_order
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.subgroup_id = $1
			  ORDER BY gh.sort_order`,
			subgroupID,
		)
		if err != nil {
			slog.Error("list subgroup hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type row struct {
			ID          string `json:"id"`
			HotelID     string `json:"hotel_id"`
			HotelName   string `json:"hotel_name"`
			HotelNameRu string `json:"hotel_name_ru"`
			City        string `json:"city"`
			Address     string `json:"address"`
			Phone       string `json:"phone"`
			CheckIn     string `json:"check_in"`
			CheckOut    string `json:"check_out"`
			RoomType    string `json:"room_type"`
			SortOrder   int    `json:"sort_order"`
		}
		var result []row
		for rows.Next() {
			var gh row
			if err := rows.Scan(
				&gh.ID, &gh.HotelID, &gh.HotelName, &gh.HotelNameRu, &gh.City,
				&gh.Address, &gh.Phone, &gh.CheckIn, &gh.CheckOut, &gh.RoomType, &gh.SortOrder,
			); err != nil {
				slog.Error("scan subgroup hotel", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			result = append(result, gh)
		}
		if result == nil {
			result = []row{}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// UpsertSubgroupHotels handles POST /api/subgroups/:id/hotels
func UpsertSubgroupHotels(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		subgroupID := chi.URLParam(r, "id")

		var groupID string
		if err := pool.QueryRow(r.Context(),
			`SELECT s.group_id FROM subgroups s
			   JOIN groups g ON g.id = s.group_id
			  WHERE s.id = $1 AND g.org_id = $2`, subgroupID, orgID).Scan(&groupID); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
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

		// Validate every referenced hotel is visible to the org
		// (global or private to this org).
		for _, e := range entries {
			var ok bool
			if err := pool.QueryRow(r.Context(),
				`SELECT true FROM hotels
				  WHERE id = $1 AND (org_id IS NULL OR org_id = $2)`,
				e.HotelID, orgID).Scan(&ok); err != nil {
				writeError(w, http.StatusBadRequest, "hotel not accessible")
				return
			}
		}

		tx, err := pool.Begin(r.Context())
		if err != nil {
			slog.Error("begin tx", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tx.Rollback(r.Context())

		// Replace all existing entries for this subgroup.
		if _, err := tx.Exec(r.Context(), `DELETE FROM group_hotels WHERE subgroup_id = $1`, subgroupID); err != nil {
			slog.Error("delete subgroup hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		for _, e := range entries {
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO group_hotels (group_id, subgroup_id, hotel_id, check_in, check_out, room_type, sort_order)
				 VALUES ($1, $2, $3, $4::date, $5::date, $6, $7)`,
				groupID, subgroupID, e.HotelID, e.CheckIn, e.CheckOut, e.RoomType, e.SortOrder,
			); err != nil {
				slog.Error("insert subgroup hotel", "err", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			slog.Error("commit subgroup hotels", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"inserted": len(entries)})
	}
}

// UpsertGroupHotels handles POST /api/groups/:id/hotels
// Body: [{hotel_id, check_in, check_out, room_type, sort_order}]
func UpsertGroupHotels(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		var groupExists bool
		if err := pool.QueryRow(r.Context(),
			`SELECT true FROM groups WHERE id = $1 AND org_id = $2`,
			groupID, orgID).Scan(&groupExists); err != nil {
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

		// Validate every referenced hotel is visible to the org.
		for _, e := range entries {
			var ok bool
			if err := pool.QueryRow(r.Context(),
				`SELECT true FROM hotels
				  WHERE id = $1 AND (org_id IS NULL OR org_id = $2)`,
				e.HotelID, orgID).Scan(&ok); err != nil {
				writeError(w, http.StatusBadRequest, "hotel not accessible")
				return
			}
		}

		tx, err := pool.Begin(r.Context())
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
