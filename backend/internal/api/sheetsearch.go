package api

import (
	"context"
	"log/slog"
	"net/http"

	"fujitravel-admin/backend/internal/matcher"
)

// SheetsSearcher is the interface expected by the sheets search handler.
type SheetsSearcher interface {
	SearchByName(ctx context.Context, query string, n int) ([]matcher.Match, error)
	AllRowsReversed(ctx context.Context) ([]map[string]string, error)
}

// ListSheetRows handles GET /api/sheets/rows?q=
// Returns all rows (latest first). If q is provided, filters by fuzzy name match.
func ListSheetRows(client SheetsSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		ctx := r.Context()

		if q != "" {
			matches, err := client.SearchByName(ctx, q, 20)
			if err != nil {
				slog.Error("sheets search", "err", err)
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			type result struct {
				Score float64           `json:"score"`
				Row   map[string]string `json:"row"`
			}
			out := make([]result, len(matches))
			for i, m := range matches {
				out[i] = result{Score: m.Score, Row: m.Row}
			}
			writeJSON(w, http.StatusOK, out)
			return
		}

		rows, err := client.AllRowsReversed(ctx)
		if err != nil {
			slog.Error("sheets all rows", "err", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if rows == nil {
			rows = []map[string]string{}
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// SearchSheets handles GET /api/sheets/search?q=<name>
func SearchSheets(client SheetsSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
			return
		}

		matches, err := client.SearchByName(r.Context(), q, 5)
		if err != nil {
			slog.Error("sheets search", "err", err)
			writeError(w, http.StatusInternalServerError, "sheets search failed: "+err.Error())
			return
		}

		type result struct {
			Score float64           `json:"score"`
			Row   map[string]string `json:"row"`
		}
		out := make([]result, len(matches))
		for i, m := range matches {
			out[i] = result{Score: m.Score, Row: m.Row}
		}
		writeJSON(w, http.StatusOK, out)
	}
}
