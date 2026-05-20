package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
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

func TestServerRepository_CreateAndListPrivateKeyCredential(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")

	err := repo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "private_key",
		PrivateKey:    "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----",
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
	if items[0].PasswordConfigured {
		t.Fatalf("expected passwordConfigured to be false for key auth")
	}
	if !items[0].PrivateKeyConfigured {
		t.Fatalf("expected privateKeyConfigured to be true")
	}
	if items[0].PrivateKey != "" {
		t.Fatalf("expected listed private key to stay hidden, got %q", items[0].PrivateKey)
	}
}

func TestServerRepository_DeletePreservesStatusHistory(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")
	statusRepo := storage.NewStatusRepository(db)
	ctx := context.Background()

	if err := repo.Create(ctx, domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	sampledAt := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	if err := statusRepo.AppendHistory(ctx, 1, collector.Snapshot{CPUUsage: 42}, sampledAt); err != nil {
		t.Fatalf("append history failed: %v", err)
	}
	if err := repo.Delete(ctx, 1); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	points, err := statusRepo.ListHistory(ctx, 1, sampledAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("list history failed: %v", err)
	}
	if len(points) != 1 || points[0].CPUUsage != 42 {
		t.Fatalf("expected preserved history, got %+v", points)
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

func TestServerRepository_UpdateRejectsAuthTypeSwitchWithoutCredential(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")
	ctx := context.Background()

	if err := repo.Create(ctx, domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	err := repo.Update(ctx, domain.Server{
		ID:            1,
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "private_key",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if !errors.Is(err, storage.ErrServerCredentialRequired) {
		t.Fatalf("expected ErrServerCredentialRequired, got %v", err)
	}

	items, err := repo.List(ctx, storage.ServerFilter{ID: 1})
	if err != nil {
		t.Fatalf("list after failed update: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 server, got %d", len(items))
	}
	if items[0].AuthType != "password" || !items[0].PasswordConfigured || items[0].PrivateKeyConfigured {
		t.Fatalf("expected original password credential state, got %+v", items[0])
	}
}

func TestServerRepository_UpdateMissingReturnsNotFound(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")

	err := repo.Update(context.Background(), domain.Server{
		ID:            999,
		Name:          "missing",
		Hostname:      "missing",
		IP:            "10.0.0.99",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if !errors.Is(err, storage.ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
}

func TestServerRepository_DeleteMissingReturnsNotFound(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")

	err := repo.Delete(context.Background(), 999)
	if !errors.Is(err, storage.ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
}
