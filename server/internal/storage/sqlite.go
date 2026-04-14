package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

func Open(ctx context.Context, driver string, target string) (*sql.DB, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", DriverSQLite:
		return OpenSQLite(ctx, target)
	case DriverPostgres:
		return OpenPostgres(ctx, target)
	default:
		return nil, errors.New("unsupported db driver")
	}
}

func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	if err := ensureSQLiteParentDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func OpenPostgres(ctx context.Context, dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres dsn is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func ensureSQLiteParentDir(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}

	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}

	return os.MkdirAll(dir, 0o755)
}
