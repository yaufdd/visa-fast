package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbrepo "fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
)

// ListGroupAILogs returns every AI call logged for this group, newest first.
// Scoped to the calling org via middleware.OrgID — cross-org requests
// receive an empty list (never 403) per the SaaS isolation policy.
func ListGroupAILogs(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		logs, err := dbrepo.ListAICallLogsForGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if logs == nil {
			logs = []dbrepo.AICallLogRow{}
		}
		writeJSON(w, http.StatusOK, logs)
	}
}
