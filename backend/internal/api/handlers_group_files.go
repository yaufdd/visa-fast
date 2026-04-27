package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
	appmw "fujitravel-admin/backend/internal/middleware"
)

// GroupTouristFileCounts handles GET /api/groups/{id}/tourist_file_counts.
//
// Returns a map of submission_id → file count for every submission
// currently attached to a tourist in this group. Submissions without
// files (or tourists without a submission_id) are omitted entirely so
// the frontend can do `counts[submission_id] || 0` without worrying
// about zero-valued entries.
//
// Designed as a single round-trip alternative to per-tourist N+1 calls
// to /api/submissions/{id}/files when the group view renders 10–20
// cards. Cross-org access collapses to 404 via the GetGroup precheck —
// same convention as every other authenticated handler.
func GroupTouristFileCounts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := appmw.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		// Existence + ownership check. 404 on miss to avoid leaking
		// whether the id exists in another org.
		g, err := db.GetGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("get group for file counts", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if g == nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// One aggregated query: every tourist in this group that has a
		// submission_id, joined to submission_files, grouped by
		// submission, filtered to those with at least one file.
		rows, err := pool.Query(r.Context(),
			`SELECT t.submission_id, COUNT(sf.id)
			   FROM tourists t
			   JOIN submission_files sf
			     ON sf.submission_id = t.submission_id
			    AND sf.org_id = t.org_id
			  WHERE t.group_id = $1
			    AND t.org_id = $2
			    AND t.submission_id IS NOT NULL
			  GROUP BY t.submission_id`,
			groupID, orgID)
		if err != nil {
			slog.Error("query tourist file counts", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		out := map[string]int{}
		for rows.Next() {
			var submissionID string
			var count int
			if err := rows.Scan(&submissionID, &count); err != nil {
				slog.Error("scan tourist file count", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			out[submissionID] = count
		}
		writeJSON(w, http.StatusOK, out)
	}
}
