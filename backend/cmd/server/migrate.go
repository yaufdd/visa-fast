package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// runMigrations applies all pending .up.sql files from migrationsDir.
// It uses a schema_migrations table (compatible with golang-migrate) to track
// the current version and dirty state.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	// Ensure schema_migrations table exists.
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version  BIGINT  NOT NULL PRIMARY KEY,
			dirty    BOOLEAN NOT NULL DEFAULT false
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Read current version.
	var currentVersion int64
	err := pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&currentVersion)
	if err != nil {
		// No rows — version 0 (no migrations applied yet).
		currentVersion = 0
	}

	// Check dirty state.
	var dirty bool
	err = pool.QueryRow(ctx, `SELECT dirty FROM schema_migrations WHERE version = $1`, currentVersion).Scan(&dirty)
	if err == nil && dirty {
		return fmt.Errorf("migration version %d is dirty — resolve manually before restarting", currentVersion)
	}

	// Discover .up.sql files.
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", migrationsDir, err)
	}

	type migration struct {
		version int64
		path    string
	}
	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		// File format: 000001_name.up.sql — extract leading digits as version.
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		migrations = append(migrations, migration{version: v, path: filepath.Join(migrationsDir, e.Name())})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	applied := 0
	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		sql, err := os.ReadFile(m.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.path, err)
		}

		// Mark as dirty before applying.
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version, dirty) VALUES ($1, true)
			 ON CONFLICT (version) DO UPDATE SET dirty = true`, m.version); err != nil {
			return fmt.Errorf("mark migration %d dirty: %w", m.version, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, filepath.Base(m.path), err)
		}

		// Mark as clean.
		if _, err := pool.Exec(ctx,
			`UPDATE schema_migrations SET dirty = false WHERE version = $1`, m.version); err != nil {
			return fmt.Errorf("mark migration %d clean: %w", m.version, err)
		}

		applied++
		slog.Info("applied migration", "version", m.version, "file", filepath.Base(m.path))
	}

	if applied == 0 {
		slog.Info("database is up to date", "version", currentVersion)
	} else {
		slog.Info("migrations complete", "applied", applied)
	}
	return nil
}
