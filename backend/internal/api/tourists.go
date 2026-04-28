package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
)

// GetTourist handles GET /api/tourists/:id — returns a single tourist row
// (with its submission_snapshot, flight_data, translations) for the read-only
// detail page. Cross-org / missing rows return 404 to avoid ID enumeration.
func GetTourist(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		t, err := db.GetTouristByID(r.Context(), pool, orgID, touristID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "tourist not found")
				return
			}
			slog.Error("get tourist", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, t)
	}
}

// ListTourists handles GET /api/groups/:id/tourists
func ListTourists(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		tourists, err := db.ListTouristsByGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("list tourists", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, tourists)
	}
}

// DeleteTourist handles DELETE /api/tourists/:id
func DeleteTourist(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		touristID := chi.URLParam(r, "id")

		ok, err := db.DeleteTourist(r.Context(), pool, orgID, touristID)
		if err != nil {
			slog.Error("delete tourist", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
