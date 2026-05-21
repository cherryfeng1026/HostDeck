package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

func openConnectionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
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
	if err.Error() != "服务器未配置 SSH 凭据" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServerConnectionService_ResolveServerReturnsDecryptedPrivateKey(t *testing.T) {
	db := openConnectionTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	credentialRepo := storage.NewServerCredentialRepository(db)

	privateKey := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest-key\n-----END OPENSSH PRIVATE KEY-----"
	err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "private_key",
		PrivateKey:    privateKey,
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

	if server.PrivateKey != privateKey {
		t.Fatalf("expected decrypted private key, got %q", server.PrivateKey)
	}
	if !server.PrivateKeyConfigured {
		t.Fatalf("expected privateKeyConfigured to be true")
	}
	if server.Password != "" {
		t.Fatalf("expected password to stay empty for key auth, got %q", server.Password)
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

func TestServerConnectionService_TrustHostKeyFingerprintUpdatesServer(t *testing.T) {
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
	if err := svc.TrustHostKeyFingerprint(context.Background(), 1, "  SHA256:test-fingerprint  "); err != nil {
		t.Fatalf("trust host key fingerprint: %v", err)
	}

	server, err := svc.ResolveServer(context.Background(), 1)
	if err != nil {
		t.Fatalf("resolve server: %v", err)
	}
	if server.TrustedHostKeyFingerprint != "SHA256:test-fingerprint" {
		t.Fatalf("unexpected trusted fingerprint: %q", server.TrustedHostKeyFingerprint)
	}
}

func TestServerConnectionService_TrustHostKeyFingerprintRejectsMissingServer(t *testing.T) {
	db := openConnectionTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	credentialRepo := storage.NewServerCredentialRepository(db)
	svc := service.NewServerConnectionService(serverRepo, credentialRepo, "test-master-key")

	err := svc.TrustHostKeyFingerprint(context.Background(), 999, "SHA256:test-fingerprint")
	if !errors.Is(err, service.ErrConnectionServerNotFound) {
		t.Fatalf("expected ErrConnectionServerNotFound, got %v", err)
	}
}
