package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

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

	// ── Router ────────────────────────────────────────────────────────────────
	r := server.NewRouter(pool, anthropicKey, uploadsDir, pythonScript)

	slog.Info("starting server", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
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
