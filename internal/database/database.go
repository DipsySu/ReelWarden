package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type DB struct {
	Path         string
	WAL          bool
	MaxOpenConns int
}

func Open(ctx context.Context, path string, wal bool, maxOpenConns int) (*DB, error) {
	_ = ctx
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("DB_DIR_CREATE_FAILED: %w", err)
	}
	return &DB{Path: path, WAL: wal, MaxOpenConns: maxOpenConns}, nil
}
func (db *DB) Close() error { return nil }
