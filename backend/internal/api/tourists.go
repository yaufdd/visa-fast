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
	ID              string          `json:"id"`
	GroupID         string          `json:"group_id"`
	RawJSON         json.RawMessage `json:"raw_json"`
	MatchedSheetRow json.RawMessage `json:"matched_sheet_row"`
	MatchConfirmed  bool            `json:"match_confirmed"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// ListTourists handles GET /api/groups/:id/tourists
func ListTourists(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, raw_json, matched_sheet_row, match_confirmed, created_at, updated_at
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
			var rawJSON, matchedRow []byte
			if err := rows.Scan(&t.ID, &t.GroupID, &rawJSON, &matchedRow,
				&t.MatchConfirmed, &t.CreatedAt, &t.UpdatedAt); err != nil {
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
		writeJSON(w, http.StatusOK, tourists)
	}
}

// AddTouristFromSheet handles POST /api/groups/:id/tourists
// Body: {"sheet_row": {...}} — creates a confirmed tourist from a Google Sheets row.
func AddTouristFromSheet(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var body struct {
			SheetRow json.RawMessage `json:"sheet_row"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.SheetRow) == 0 {
			writeError(w, http.StatusBadRequest, "field 'sheet_row' is required")
			return
		}

		var touristID string
		err := db.QueryRow(r.Context(),
			`INSERT INTO tourists (group_id, matched_sheet_row, match_confirmed)
			 VALUES ($1, $2, true) RETURNING id`,
			groupID, []byte(body.SheetRow),
		).Scan(&touristID)
		if err != nil {
			slog.Error("insert tourist from sheet", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		var t Tourist
		var rawJSON, matchedRow []byte
		err = db.QueryRow(r.Context(),
			`SELECT id, group_id, raw_json, matched_sheet_row, match_confirmed, created_at, updated_at
			   FROM tourists WHERE id = $1`, touristID).
			Scan(&t.ID, &t.GroupID, &rawJSON, &matchedRow, &t.MatchConfirmed, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if rawJSON != nil {
			t.RawJSON = json.RawMessage(rawJSON)
		}
		if matchedRow != nil {
			t.MatchedSheetRow = json.RawMessage(matchedRow)
		}
		writeJSON(w, http.StatusCreated, t)
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

// ConfirmMatch handles POST /api/tourists/:id/match
// Body: {"sheet_row": {...}}
func ConfirmMatch(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")

		var body struct {
			SheetRow map[string]string `json:"sheet_row"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SheetRow == nil {
			writeError(w, http.StatusBadRequest, "field 'sheet_row' is required")
			return
		}

		rowJSON, err := json.Marshal(body.SheetRow)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "marshal error")
			return
		}

		tag, err := db.Exec(r.Context(),
			`UPDATE tourists
			    SET matched_sheet_row = $1, match_confirmed = true, updated_at = now()
			  WHERE id = $2`,
			rowJSON, touristID)
		if err != nil {
			slog.Error("update tourist match", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}

		var t Tourist
		var rawJSON, matchedRow []byte
		err = db.QueryRow(r.Context(),
			`SELECT id, group_id, raw_json, matched_sheet_row, match_confirmed, created_at, updated_at
			   FROM tourists WHERE id = $1`, touristID).
			Scan(&t.ID, &t.GroupID, &rawJSON, &matchedRow, &t.MatchConfirmed, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			slog.Error("fetch tourist after match", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if rawJSON != nil {
			t.RawJSON = json.RawMessage(rawJSON)
		}
		if matchedRow != nil {
			t.MatchedSheetRow = json.RawMessage(matchedRow)
		}
		writeJSON(w, http.StatusOK, t)
	}
}
