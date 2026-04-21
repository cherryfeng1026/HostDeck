package storage_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

func TestMigrate_CommandLogsExecutedAtTextUpgradesToTimestamptz(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HOSTDECK_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Skip("skip postgres-backed test: HOSTDECK_TEST_POSTGRES_DSN or DATABASE_URL is required")
	}

	ctx := context.Background()
	adminDB, err := storage.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open admin postgres db: %v", err)
	}
	schemaName := fmt.Sprintf("hostdeck_migrate_test_%d", time.Now().UTC().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create test schema: %v", err)
	}
	testDSN, err := withMigrationTestSearchPath(dsn, schemaName)
	if err != nil {
		_, _ = adminDB.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
		_ = adminDB.Close()
		t.Fatalf("build schema dsn: %v", err)
	}
	db, err := storage.Open(ctx, testDSN)
	if err != nil {
		_, _ = adminDB.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
		_ = adminDB.Close()
		t.Fatalf("open test postgres db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
		_ = adminDB.Close()
	})

	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version BIGINT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE command_logs (
			id BIGSERIAL PRIMARY KEY,
			server_id BIGINT NOT NULL,
			command TEXT NOT NULL,
			stdout TEXT NOT NULL,
			stderr TEXT NOT NULL,
			exit_code INTEGER NOT NULL,
			duration_ms BIGINT NOT NULL,
			executor_username TEXT NOT NULL DEFAULT '',
			executed_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy command_logs: %v", err)
	}
	for version := int64(1); version <= 9; version++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, CURRENT_TIMESTAMP::text)`, version); err != nil {
			t.Fatalf("seed schema version %d: %v", version, err)
		}
	}

	base := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
	legacyValues := []string{
		base.Format(time.RFC3339Nano),
		base.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
	}
	for index, executedAt := range legacyValues {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO command_logs (server_id, command, stdout, stderr, exit_code, duration_ms, executor_username, executed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, 1, fmt.Sprintf("cmd-%d", index+1), "ok", "", 0, 10, "admin", executedAt); err != nil {
			t.Fatalf("insert legacy command log: %v", err)
		}
	}

	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	var dataType string
	if err := db.QueryRowContext(ctx, `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'command_logs' AND column_name = 'executed_at'
	`).Scan(&dataType); err != nil {
		t.Fatalf("query executed_at data type: %v", err)
	}
	if dataType != "timestamp with time zone" {
		t.Fatalf("expected timestamptz column type, got %q", dataType)
	}

	repo := storage.NewCommandLogRepository(db)
	start := base
	end := base.Add(200 * time.Millisecond)
	items, err := repo.ListHistory(ctx, domain.CommandHistoryFilter{StartTime: &start, EndTime: &end, Limit: 10})
	if err != nil {
		t.Fatalf("list history after migration: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 migrated rows, got %d", len(items))
	}
	if items[0].Command != "cmd-2" || items[1].Command != "cmd-1" {
		t.Fatalf("expected migrated rows in desc order, got %+v", items)
	}
	if !items[0].ExecutedAt.Equal(base.Add(100 * time.Millisecond)) || !items[1].ExecutedAt.Equal(base) {
		t.Fatalf("expected subsecond precision to be preserved, got %+v", items)
	}
}

func withMigrationTestSearchPath(dsn string, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
