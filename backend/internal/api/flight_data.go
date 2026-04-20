package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateFlightData handles PUT /api/tourists/:id/flight_data
// Body: { "arrival": {...}, "departure": {...} }  (departure may be empty)
func UpdateFlightData(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		buf, _ := json.Marshal(body)
		tag, err := db.Exec(r.Context(),
			`UPDATE tourists SET flight_data = $1, updated_at = NOW() WHERE id = $2`, buf, id)
		if err != nil {
			slog.Error("update flight_data", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
