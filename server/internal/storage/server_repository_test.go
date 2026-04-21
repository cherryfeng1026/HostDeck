package storage_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func TestServerRepository_CreateAndList(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")

	err := repo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	items, err := repo.List(context.Background(), storage.ServerFilter{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 server, got %d", len(items))
	}
	if !items[0].PasswordConfigured {
		t.Fatalf("expected passwordConfigured to be true")
	}
	if items[0].Password != "" {
		t.Fatalf("expected listed server password to stay hidden, got %q", items[0].Password)
	}
}

func TestServerRepository_UpdateWithoutPasswordKeepsCredentialState(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")

	err := repo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	start := time.Date(2026, 4, 21, 1, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	if err := repo.Update(context.Background(), domain.Server{
		ID:                 1,
		Name:               "prod-web-01",
		Hostname:           "prod-web-01",
		IP:                 "10.0.0.22",
		SSHPort:            22,
		Username:           "root",
		AuthType:           "password",
		CollectorMode:      "ssh_only",
		MaintenanceStartAt: &start,
		MaintenanceEndAt:   &end,
		Enabled:            true,
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	items, err := repo.List(context.Background(), storage.ServerFilter{ID: 1})
	if err != nil {
		t.Fatalf("list after update failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 server, got %d", len(items))
	}
	if !items[0].PasswordConfigured {
		t.Fatalf("expected passwordConfigured to remain true after update without password")
	}
	if items[0].IP != "10.0.0.22" {
		t.Fatalf("expected updated ip, got %q", items[0].IP)
	}
	if items[0].MaintenanceStartAt == nil || !items[0].MaintenanceStartAt.Equal(start) {
		t.Fatalf("expected maintenance start %v, got %v", start, items[0].MaintenanceStartAt)
	}
	if items[0].MaintenanceEndAt == nil || !items[0].MaintenanceEndAt.Equal(end) {
		t.Fatalf("expected maintenance end %v, got %v", end, items[0].MaintenanceEndAt)
	}
}
