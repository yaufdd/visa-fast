package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
)

// PgxAILogger persists ai.CallLog entries into the ai_call_logs table.
// Best-effort: a DB error is logged via slog but never surfaced to the
// caller — logging failures must not abort an AI generation.
type PgxAILogger struct {
	Pool *pgxpool.Pool
}

// NewPgxAILogger returns a Logger that writes to the given pool.
func NewPgxAILogger(pool *pgxpool.Pool) *PgxAILogger {
	return &PgxAILogger{Pool: pool}
}

// Log implements ai.Logger. Rows missing org_id, generation_id, or provider
// are skipped silently (test harness calls, one-off utilities) rather than
// failing the NOT NULL / CHECK constraints. Provider validity is enforced
// by the DB CHECK constraint (migration 000019); an unknown value yields a
// logged warning here but never aborts the AI call.
func (l *PgxAILogger) Log(ctx context.Context, e ai.CallLog) error {
	if l == nil || l.Pool == nil {
		return nil
	}
	if e.OrgID == "" || e.GenerationID == "" {
		return nil
	}
	if e.Provider == "" {
		slog.Warn("ai_call_logs insert skipped: empty provider",
			"function", e.FunctionName, "generation_id", e.GenerationID)
		return nil
	}
	// The calling request's ctx may already be cancelled by the time the
	// deferred Log runs (e.g. client disconnects right after the provider
	// returns). Use a detached Background ctx so the audit row still lands.
	bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reqJSON := e.RequestJSON
	if len(reqJSON) == 0 {
		reqJSON = json.RawMessage(`{}`)
	}

	_, err := l.Pool.Exec(bg, `
		INSERT INTO ai_call_logs (
			org_id, group_id, subgroup_id, generation_id,
			function_name, provider, model, request_json, response_text,
			status, error_msg, input_tokens, output_tokens,
			started_at, finished_at, duration_ms
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16
		)`,
		e.OrgID,
		nullUUID(e.GroupID), nullUUID(e.SubgroupID),
		e.GenerationID,
		e.FunctionName, e.Provider, e.Model, reqJSON, nullString(e.ResponseText),
		e.Status, nullString(e.ErrorMsg),
		nullInt(e.InputTokens), nullInt(e.OutputTokens),
		e.StartedAt, e.FinishedAt, e.DurationMs,
	)
	if err != nil {
		slog.Warn("ai_call_logs insert failed", "err", err,
			"function", e.FunctionName, "generation_id", e.GenerationID)
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}

// AICallLogRow is one row returned by ListAICallLogsForGroup.
type AICallLogRow struct {
	ID           string          `json:"id"`
	GenerationID string          `json:"generation_id"`
	GroupID      sql.NullString  `json:"-"`
	SubgroupID   sql.NullString  `json:"-"`
	FunctionName string          `json:"function_name"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	RequestJSON  json.RawMessage `json:"request_json"`
	ResponseText sql.NullString  `json:"-"`
	Status       string          `json:"status"`
	ErrorMsg     sql.NullString  `json:"-"`
	InputTokens  sql.NullInt32   `json:"-"`
	OutputTokens sql.NullInt32   `json:"-"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   sql.NullTime    `json:"-"`
	DurationMs   int             `json:"duration_ms"`
}

// MarshalJSON flattens the sql.Null* fields so the wire JSON is friendly
// (plain strings / numbers instead of {"String":"", "Valid":false}).
func (r AICallLogRow) MarshalJSON() ([]byte, error) {
	type alias AICallLogRow
	out := struct {
		alias
		GroupID      string `json:"group_id,omitempty"`
		SubgroupID   string `json:"subgroup_id,omitempty"`
		ResponseText string `json:"response_text,omitempty"`
		ErrorMsg     string `json:"error_msg,omitempty"`
		InputTokens  int    `json:"input_tokens,omitempty"`
		OutputTokens int    `json:"output_tokens,omitempty"`
		FinishedAt   string `json:"finished_at,omitempty"`
	}{alias: alias(r)}
	if r.GroupID.Valid {
		out.GroupID = r.GroupID.String
	}
	if r.SubgroupID.Valid {
		out.SubgroupID = r.SubgroupID.String
	}
	if r.ResponseText.Valid {
		out.ResponseText = r.ResponseText.String
	}
	if r.ErrorMsg.Valid {
		out.ErrorMsg = r.ErrorMsg.String
	}
	if r.InputTokens.Valid {
		out.InputTokens = int(r.InputTokens.Int32)
	}
	if r.OutputTokens.Valid {
		out.OutputTokens = int(r.OutputTokens.Int32)
	}
	if r.FinishedAt.Valid {
		out.FinishedAt = r.FinishedAt.Time.Format(time.RFC3339)
	}
	return json.Marshal(out)
}

// PurgeOldAICallLogs deletes every ai_call_logs row older than `days` days.
// Called once a day by the retention goroutine in cmd/server/main.go.
// Returns the number of rows deleted.
func PurgeOldAICallLogs(ctx context.Context, pool *pgxpool.Pool, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	tag, err := pool.Exec(ctx,
		`DELETE FROM ai_call_logs WHERE started_at < NOW() - ($1::int || ' days')::INTERVAL`,
		days)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ListAICallLogsForGroup returns every log row associated with this org+group,
// newest first. Use for the GroupDetailPage audit viewer.
func ListAICallLogsForGroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]AICallLogRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, generation_id, group_id, subgroup_id,
		       function_name, provider, model, request_json, response_text,
		       status, error_msg, input_tokens, output_tokens,
		       started_at, finished_at, duration_ms
		  FROM ai_call_logs
		 WHERE org_id = $1 AND group_id = $2
		 ORDER BY started_at DESC
		 LIMIT 500`,
		orgID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AICallLogRow
	for rows.Next() {
		var r AICallLogRow
		if err := rows.Scan(
			&r.ID, &r.GenerationID, &r.GroupID, &r.SubgroupID,
			&r.FunctionName, &r.Provider, &r.Model, &r.RequestJSON, &r.ResponseText,
			&r.Status, &r.ErrorMsg, &r.InputTokens, &r.OutputTokens,
			&r.StartedAt, &r.FinishedAt, &r.DurationMs,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
