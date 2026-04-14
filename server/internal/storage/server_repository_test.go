package storage_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:server-repository-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := storage.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return db
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

	err = repo.Update(context.Background(), domain.Server{
		ID:            1,
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.22",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if err != nil {
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
}
