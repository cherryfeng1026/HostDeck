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
