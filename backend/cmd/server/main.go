package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	dbrepo "fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/server"
)

func main() {
	ctx := context.Background()

	// ── Environment ───────────────────────────────────────────────────────────
	dbURL := mustEnv("DATABASE_URL")
	anthropicKey := mustEnv("ANTHROPIC_API_KEY")
	uploadsDir := envOrDefault("UPLOADS_DIR", "./uploads")
	port := envOrDefault("PORT", "8080")

	appSecret := os.Getenv("APP_SECRET")
	if appSecret == "" {
		slog.Error("APP_SECRET environment variable is required")
		os.Exit(1)
	}
	_ = appSecret // currently unused — will be consumed when Tier 2 adds token signing

	// Resolve uploads dir to absolute path relative to cwd.
	if !filepath.IsAbs(uploadsDir) {
		cwd, err := os.Getwd()
		if err != nil {
			slog.Error("getwd", "err", err)
			os.Exit(1)
		}
		uploadsDir = filepath.Join(cwd, uploadsDir)
	}

	// Python docgen script lives next to the backend directory.
	// Adjust this path if the project layout changes.
	pythonScript := envOrDefault("DOCGEN_SCRIPT", filepath.Join(uploadsDir, "../../docgen/generate.py"))

	// ── Database ──────────────────────────────────────────────────────────────
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping database", "err", err)
		os.Exit(1)
	}
	slog.Info("database connected")

	// ── Auto-migrate ──────────────────────────────────────────────────────────
	migrationsDir := envOrDefault("MIGRATIONS_DIR", "migrations")
	if !filepath.IsAbs(migrationsDir) {
		cwd, _ := os.Getwd()
		migrationsDir = filepath.Join(cwd, migrationsDir)
	}
	if err := runMigrations(ctx, pool, migrationsDir); err != nil {
		slog.Error("run migrations", "err", err)
		os.Exit(1)
	}

	// ── Retention: auto-purge old ai_call_logs rows ───────────────────────────
	// AI_LOG_RETENTION_DAYS controls how long we keep the audit trail. Default
	// 30 days is a pragmatic trade-off — long enough for post-hoc debugging
	// and 152-ФЗ incident investigation, short enough to limit the amount of
	// PII-adjacent content sitting in the table.
	retentionDays := parseRetentionDays(os.Getenv("AI_LOG_RETENTION_DAYS"), 30)
	if retentionDays > 0 {
		go startAILogRetention(ctx, pool, retentionDays)
	} else {
		slog.Warn("AI_LOG_RETENTION_DAYS=0 — ai_call_logs will NOT be auto-purged")
	}

	// ── Router ────────────────────────────────────────────────────────────────
	r := server.NewRouter(pool, anthropicKey, uploadsDir, pythonScript)

	slog.Info("starting server", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// parseRetentionDays returns a safe positive integer; on parse failure or
// non-positive values it falls back to the default. Zero / negative in env
// means "retention disabled".
func parseRetentionDays(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		slog.Warn("AI_LOG_RETENTION_DAYS is not an integer, using default",
			"value", raw, "default", def)
		return def
	}
	if n < 0 {
		return 0 // explicit 0 / negative → disable
	}
	return n
}

// startAILogRetention runs a background loop that purges ai_call_logs rows
// older than `days` days. Runs once at startup (so a container restart
// doesn't skip a tick), then every 24h thereafter.
func startAILogRetention(ctx context.Context, pool *pgxpool.Pool, days int) {
	slog.Info("ai_call_logs retention enabled", "days", days)

	purgeOnce := func() {
		purgeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		n, err := dbrepo.PurgeOldAICallLogs(purgeCtx, pool, days)
		if err != nil {
			slog.Warn("ai_call_logs purge failed", "err", err)
			return
		}
		if n > 0 {
			slog.Info("ai_call_logs purged", "rows", n, "days", days)
		}
	}

	purgeOnce()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purgeOnce()
		}
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable is not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
