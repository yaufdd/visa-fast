package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/api"
	"fujitravel-admin/backend/internal/sheets"
)

func main() {
	ctx := context.Background()

	// ── Environment ───────────────────────────────────────────────────────────
	dbURL := mustEnv("DATABASE_URL")
	anthropicKey := mustEnv("ANTHROPIC_API_KEY")
	googleCredsPath := mustEnv("GOOGLE_CREDENTIALS_PATH")
	sheetID := mustEnv("GOOGLE_SHEET_ID")
	uploadsDir := envOrDefault("UPLOADS_DIR", "./uploads")
	port := envOrDefault("PORT", "8080")

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

	// ── Google Sheets ─────────────────────────────────────────────────────────
	sheetsClient, err := sheets.New(ctx, googleCredsPath, sheetID)
	if err != nil {
		// Non-fatal: sheets search will fail with a clear error at request time.
		slog.Warn("sheets client init failed", "err", err)
		sheetsClient = nil
	}

	// ── Router ────────────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS — simple permissive policy for local dev.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	r.Route("/api", func(r chi.Router) {
		// Hotels
		r.Get("/hotels", api.ListHotels(pool))
		r.Post("/hotels", api.CreateHotel(pool))

		// Groups
		r.Get("/groups", api.ListGroups(pool))
		r.Post("/groups", api.CreateGroup(pool))
		r.Get("/groups/{id}", api.GetGroup(pool))
		r.Delete("/groups/{id}", api.DeleteGroup(pool))

		// Uploads
		r.Get("/groups/{id}/uploads", api.ListUploads(pool))
		r.Post("/groups/{id}/uploads", api.UploadFile(pool, uploadsDir, anthropicKey))

		// Parse (AI Pass 1 + auto-match)
		if sheetsClient != nil {
			r.Post("/groups/{id}/parse", api.ParseGroup(pool, anthropicKey, sheetsClient))
		} else {
			r.Post("/groups/{id}/parse", api.ParseGroup(pool, anthropicKey))
		}

		// Subgroups
		r.Get("/groups/{id}/subgroups", api.ListSubgroups(pool))
		r.Post("/groups/{id}/subgroups", api.CreateSubgroup(pool))
		r.Put("/subgroups/{id}", api.UpdateSubgroup(pool))
		r.Delete("/subgroups/{id}", api.DeleteSubgroup(pool))
		r.Put("/tourists/{id}/subgroup", api.AssignTouristSubgroup(pool))
		r.Post("/subgroups/{id}/parse", api.ParseSubgroup(pool, anthropicKey))
		r.Get("/subgroups/{id}/hotels", api.ListSubgroupHotels(pool))
		r.Post("/subgroups/{id}/hotels", api.UpsertSubgroupHotels(pool))
		r.Post("/subgroups/{id}/generate", api.GenerateSubgroupDocuments(pool, anthropicKey, uploadsDir, pythonScript))
		r.Get("/subgroups/{id}/download", api.DownloadSubgroupZIP(pool, uploadsDir))

		// Tourists
		r.Get("/groups/{id}/tourists", api.ListTourists(pool))
		r.Post("/groups/{id}/tourists", api.AddTouristFromSheet(pool))
		r.Delete("/tourists/{id}", api.DeleteTourist(pool))
		r.Post("/tourists/{id}/match", api.ConfirmMatch(pool))

		// Per-tourist uploads & parse
		r.Get("/tourists/{id}/uploads", api.ListTouristUploads(pool))
		r.Post("/tourists/{id}/uploads", api.UploadTouristFile(pool, uploadsDir, anthropicKey))
		r.Post("/tourists/{id}/parse", api.ParseTourist(pool, anthropicKey))

		// Group hotels
		r.Get("/groups/{id}/hotels", api.ListGroupHotels(pool))
		r.Post("/groups/{id}/hotels", api.UpsertGroupHotels(pool))

		// Document generation (AI Pass 2 + Python)
		r.Post("/groups/{id}/generate", api.GenerateDocuments(pool, anthropicKey, uploadsDir, pythonScript))
		r.Post("/groups/{id}/finalize", api.FinalizeGroup(pool, anthropicKey, uploadsDir, pythonScript))
		r.Get("/groups/{id}/documents", api.GetDocuments(pool))
		r.Get("/groups/{id}/download", api.DownloadZIP(pool))
		r.Get("/groups/{id}/download/final", api.DownloadFinalZIP(pool))

		// Google Sheets
		if sheetsClient != nil {
			r.Get("/sheets/search", api.SearchSheets(sheetsClient))
			r.Get("/sheets/rows", api.ListSheetRows(sheetsClient))
		} else {
			noSheets := func(w http.ResponseWriter, req *http.Request) {
				http.Error(w, `{"error":"Google Sheets client not configured"}`, http.StatusServiceUnavailable)
			}
			r.Get("/sheets/search", noSheets)
			r.Get("/sheets/rows", noSheets)
		}
	})

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
