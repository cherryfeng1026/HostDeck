package storage_test

import (
	"context"
	"strings"
	"testing"

	"hostdeck/server/internal/storage"
)

func TestOpen_RejectsUnknownDriver(t *testing.T) {
	_, err := storage.Open(context.Background(), "unknown", "ignored")
	if err == nil {
		t.Fatalf("expected unknown driver error")
	}
	if !strings.Contains(err.Error(), "unsupported db driver") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpen_PostgresRequiresDSN(t *testing.T) {
	_, err := storage.Open(context.Background(), storage.DriverPostgres, "")
	if err == nil {
		t.Fatalf("expected empty dsn error")
	}
	if !strings.Contains(err.Error(), "postgres dsn is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
