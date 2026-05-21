package storage_test

import (
	"context"
	"testing"

	"hostdeck/server/internal/storage"
)

func TestServerCredentialRepository_UpsertAndGetByServerID(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewServerCredentialRepository(db)

	err := repo.UpsertPassword(context.Background(), 42, "password", "ciphertext-value")
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	item, err := repo.GetByServerID(context.Background(), 42)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if item.ServerID != 42 {
		t.Fatalf("expected server id 42, got %d", item.ServerID)
	}
	if item.AuthType != "password" {
		t.Fatalf("expected auth type password, got %q", item.AuthType)
	}
	if item.PasswordCiphertext != "ciphertext-value" {
		t.Fatalf("expected ciphertext to round-trip, got %q", item.PasswordCiphertext)
	}
}
