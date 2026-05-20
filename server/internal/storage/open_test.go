package storage_test

import (
	"context"
	"strings"
	"testing"

	"hostdeck/server/internal/storage"
)

func TestOpen_PostgresRequiresDSN(t *testing.T) {
	_, err := storage.Open(context.Background(), "")
	if err == nil {
		t.Fatalf("expected empty dsn error")
	}
	if !strings.Contains(err.Error(), "postgres dsn is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpen_AppliesSearchPathFromURL(t *testing.T) {
	db := storage_testOpenPostgresDB(t, "")
	defer db.Close()

	var currentSchema string
	var searchPath string
	if err := db.QueryRowContext(context.Background(), `select current_schema(), current_setting('search_path')`).Scan(&currentSchema, &searchPath); err != nil {
		t.Fatalf("query search_path: %v", err)
	}
	if !strings.HasPrefix(currentSchema, "hostdeck_test_") {
		t.Fatalf("expected current schema to use temporary hostdeck_test_ prefix, got %q", currentSchema)
	}
	if !strings.Contains(searchPath, currentSchema) {
		t.Fatalf("expected search_path to contain %q, got %q", currentSchema, searchPath)
	}
}

func TestOpen_DoesNotPinSearchPathConnectionsToSingleSession(t *testing.T) {
	db := storage_testOpenPostgresDB(t, "")
	defer db.Close()

	if got := db.Stats().MaxOpenConnections; got <= 1 {
		t.Fatalf("expected search_path connections not to be pinned to one session, got MaxOpenConnections=%d", got)
	}
}

func TestOpen_RejectsUnknownPooledHost(t *testing.T) {
	_, err := storage.Open(
		context.Background(),
		"postgresql://user:password@db-pooler.example.com/neondb?sslmode=require",
	)
	if err == nil {
		t.Fatal("expected unknown pooled host to be rejected")
	}
	if !strings.Contains(err.Error(), "unsafe pooled postgres dsn without explicit search_path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSearchPath_RejectsInvalidSearchPath(t *testing.T) {
	err := storage.ValidateSearchPath("public;drop schema public")
	if err == nil {
		t.Fatal("expected invalid search_path error")
	}
	if !strings.Contains(err.Error(), "invalid postgres search_path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSearchPath_AllowsUserPlaceholder(t *testing.T) {
	if err := storage.ValidateSearchPath(`$user, public`); err != nil {
		t.Fatalf("expected $user placeholder to be accepted, got %v", err)
	}
}
