package testsupport

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/storage"
)

func OpenPostgresTestDB(t *testing.T) *sql.DB {
	t.Helper()

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

	schemaName := fmt.Sprintf("hostdeck_test_%d", time.Now().UTC().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create test schema: %v", err)
	}

	testDSN, err := withSearchPath(dsn, schemaName)
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

	if err := storage.Migrate(ctx, db); err != nil {
		_ = db.Close()
		_, _ = adminDB.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
		_ = adminDB.Close()
		t.Fatalf("migrate test schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
		_ = adminDB.Close()
	})

	return db
}

func withSearchPath(dsn string, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
