package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string, wal bool, maxOpenConns int) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("DB_DIR_CREATE_FAILED: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("DB_OPEN_FAILED: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	if wal {
		if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("DB_WAL_FAILED: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("DB_FOREIGN_KEYS_FAILED: %w", err)
	}
	if err := Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("DB_MIGRATIONS_READ_FAILED: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i, name := range names {
		version := i + 1
		var exists int
		_ = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = ?", version).Scan(&exists)
		if exists > 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("DB_MIGRATION_READ_FAILED: %w", err)
		}
		if _, err := db.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("DB_MIGRATION_APPLY_FAILED: %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)", version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("DB_MIGRATION_RECORD_FAILED: %w", err)
		}
	}
	return nil
}
