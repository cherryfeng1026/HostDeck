package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

var postgresIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("postgres dsn is required")
	}

	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	requestedSearchPath := strings.TrimSpace(connConfig.RuntimeParams["search_path"])
	pooled := isLikelyPooledPostgresHost(connConfig.Host)
	if requestedSearchPath == "" && pooled {
		return nil, fmt.Errorf("unsafe pooled postgres dsn without explicit search_path for host %q: append \"&search_path=public\" for the default schema, or configure a dedicated schema/direct connection", connConfig.Host)
	}

	db, err := openPostgresDB(connConfig, requestedSearchPath)
	if err != nil {
		return nil, err
	}
	configurePostgresPool(db, pooled)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	logPostgresConnection(ctx, db, connConfig.Host, requestedSearchPath, pooled)

	return db, nil
}

func openPostgresDB(connConfig *pgx.ConnConfig, requestedSearchPath string) (*sql.DB, error) {
	if strings.TrimSpace(requestedSearchPath) != "" {
		if err := ValidateSearchPath(requestedSearchPath); err != nil {
			return nil, err
		}
	}
	return stdlib.OpenDB(*connConfig), nil
}

func normalizeSearchPath(value string) (string, error) {
	items := strings.Split(value, ",")
	formatted := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			return "", errors.New("invalid postgres search_path")
		}
		if name == "$user" {
			formatted = append(formatted, "$user")
			continue
		}
		if !postgresIdentifierPattern.MatchString(name) {
			return "", fmt.Errorf("invalid postgres search_path item: %s", name)
		}
		formatted = append(formatted, name)
	}
	return strings.Join(formatted, ", "), nil
}

func ValidateSearchPath(value string) error {
	_, err := normalizeSearchPath(value)
	return err
}

func isLikelyPooledPostgresHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.Contains(host, "-pooler.") || strings.Contains(host, ".pooler.")
}

func configurePostgresPool(db *sql.DB, pooled bool) {
	if db == nil {
		return
	}
	if pooled {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(8)
		db.SetConnMaxLifetime(30 * time.Minute)
		db.SetConnMaxIdleTime(5 * time.Minute)
		return
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(15 * time.Minute)
}

func logPostgresConnection(ctx context.Context, db *sql.DB, host string, requestedSearchPath string, pooled bool) {
	var currentSchema string
	var currentSearchPath string
	if err := db.QueryRowContext(ctx, `select current_schema(), current_setting('search_path')`).Scan(&currentSchema, &currentSearchPath); err != nil {
		slog.Warn("inspect postgres connection failed", "host", host, "pooled", pooled, "requestedSearchPath", requestedSearchPath, "error", err)
		return
	}
	slog.Info(
		"postgres connection initialized",
		"host", host,
		"pooled", pooled,
		"requestedSearchPath", requestedSearchPath,
		"currentSchema", currentSchema,
		"searchPath", currentSearchPath,
	)
}
