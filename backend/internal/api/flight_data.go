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

// UpdateFlightData handles PUT /api/tourists/:id/flight_data
// Body: { "arrival": {...}, "departure": {...} }  (departure may be empty)
func UpdateFlightData(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		buf, _ := json.Marshal(body)

		ok, err := db.UpdateFlightData(r.Context(), pool, orgID, touristID, buf)
		if err != nil {
			slog.Error("update flight_data", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ApplyFlightDataToSubgroup handles POST /api/tourists/:id/flight_data/apply_to_subgroup
// Copies the tourist's current flight_data to every other tourist in the same subgroup.
// Responds 404 if the tourist does not exist, is cross-org, or is not assigned to a subgroup.
func ApplyFlightDataToSubgroup(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		ok, n, err := db.ApplyFlightDataToSubgroup(r.Context(), pool, orgID, touristID)
		if err != nil {
			slog.Error("apply flight_data to subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "tourist not found or not in a subgroup")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
	}
}
