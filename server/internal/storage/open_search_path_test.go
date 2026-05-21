package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"hostdeck/server/internal/storage"
)

func storage_testPostgresDSN(t *testing.T) string {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("HOSTDECK_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Skip("skip postgres-backed test: HOSTDECK_TEST_POSTGRES_DSN or DATABASE_URL is required")
	}
	return dsn
}

func storage_testOpenAdminPostgresDB(t *testing.T) (*sql.DB, string) {
	t.Helper()

	dsn := storage_testPostgresDSN(t)
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin postgres db: %v", err)
	}
	if err := adminDB.PingContext(context.Background()); err != nil {
		_ = adminDB.Close()
		t.Fatalf("ping admin postgres db: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Close()
	})
	return adminDB, dsn
}

func storage_testCreateSchema(t *testing.T, adminDB *sql.DB) string {
	t.Helper()

	schemaName := fmt.Sprintf("hostdeck_test_%d", time.Now().UTC().UnixNano())
	if _, err := adminDB.ExecContext(context.Background(), `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	})
	return schemaName
}

func storage_testBuildDSNWithSearchPath(t *testing.T, dsn string, schemaName string, extraQuery string) string {
	t.Helper()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	if extraQuery != "" {
		for _, pair := range strings.Split(extraQuery, "&") {
			if strings.TrimSpace(pair) == "" {
				continue
			}
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				query.Set(parts[0], parts[1])
			}
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func storage_testOpenPostgresDB(t *testing.T, extraQuery string) *sql.DB {
	t.Helper()

	adminDB, dsn := storage_testOpenAdminPostgresDB(t)
	schemaName := storage_testCreateSchema(t, adminDB)
	testDSN := storage_testBuildDSNWithSearchPath(t, dsn, schemaName, extraQuery)
	testDB, err := storage.Open(context.Background(), testDSN)
	if err != nil {
		t.Fatalf("open search_path postgres db: %v", err)
	}
	return testDB
}
