package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

// ListTourists handles GET /api/groups/:id/tourists
func ListTourists(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, subgroup_id, submission_id, submission_snapshot, flight_data,
			        created_at, updated_at
			   FROM tourists WHERE group_id = $1 ORDER BY created_at`, groupID)
		if err != nil {
			slog.Error("list tourists", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var tourists []Tourist
		for rows.Next() {
			var t Tourist
			var subID *string
			var snap, flight []byte
			if err := rows.Scan(&t.ID, &t.GroupID, &t.SubgroupID, &subID, &snap, &flight,
				&t.CreatedAt, &t.UpdatedAt); err != nil {
				slog.Error("scan tourist", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			t.SubmissionID = subID
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
		writeJSON(w, http.StatusOK, tourists)
	}
}

// DeleteTourist handles DELETE /api/tourists/:id
func DeleteTourist(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")
		tag, err := db.Exec(r.Context(), `DELETE FROM tourists WHERE id = $1`, touristID)
		if err != nil {
			slog.Error("delete tourist", "err", err)
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
