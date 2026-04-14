package service_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

func openConnectionTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:server-connection-service-test?mode=memory&cache=shared")
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

func TestServerConnectionService_ResolveServerReturnsDecryptedPassword(t *testing.T) {
	db := openConnectionTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	credentialRepo := storage.NewServerCredentialRepository(db)

	err := serverRepo.Create(context.Background(), domain.Server{
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
		t.Fatalf("create server: %v", err)
	}

	svc := service.NewServerConnectionService(serverRepo, credentialRepo, "test-master-key")
	server, err := svc.ResolveServer(context.Background(), 1)
	if err != nil {
		t.Fatalf("resolve server: %v", err)
	}

	if server.ID != 1 || server.IP != "10.0.0.21" {
		t.Fatalf("unexpected resolved server: %+v", server)
	}
	if server.Password != "super-secret" {
		t.Fatalf("expected decrypted password, got %q", server.Password)
	}
}

func TestServerConnectionService_ResolveServerRejectsMissingPassword(t *testing.T) {
	db := openConnectionTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	credentialRepo := storage.NewServerCredentialRepository(db)

	err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	svc := service.NewServerConnectionService(serverRepo, credentialRepo, "test-master-key")
	_, err = svc.ResolveServer(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for missing password")
	}
	if err.Error() != "服务器未配置 SSH 密码" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServerConnectionService_ResolveServerRejectsDisabledServer(t *testing.T) {
	db := openConnectionTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	credentialRepo := storage.NewServerCredentialRepository(db)

	err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       false,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	svc := service.NewServerConnectionService(serverRepo, credentialRepo, "test-master-key")
	_, err = svc.ResolveServer(context.Background(), 1)
	if err == nil {
		t.Fatal("expected disabled server error")
	}
	if err != service.ErrServerDisabled {
		t.Fatalf("unexpected error: %v", err)
	}
}
