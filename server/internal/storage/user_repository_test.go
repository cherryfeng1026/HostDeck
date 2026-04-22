package storage_test

import (
	"context"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

func TestUserRepository_NotificationReadAtRoundTrip(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewUserRepository(db)

	user, err := repo.Create(context.Background(), "admin", "hashed-password", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	initial, err := repo.GetNotificationReadAt(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get initial notification read at: %v", err)
	}
	if initial != nil {
		t.Fatalf("expected nil initial notification read at, got %v", initial)
	}

	readAt := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	if err := repo.UpdateNotificationReadAt(context.Background(), user.ID, readAt); err != nil {
		t.Fatalf("update notification read at: %v", err)
	}

	stored, err := repo.GetNotificationReadAt(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get stored notification read at: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored notification read at")
	}
	if !stored.Equal(readAt) {
		t.Fatalf("expected readAt %s, got %s", readAt.Format(time.RFC3339Nano), stored.Format(time.RFC3339Nano))
	}

	record, err := repo.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if !record.UpdatedAt.Equal(readAt) {
		t.Fatalf("expected updatedAt to match notification cursor, got %s", record.UpdatedAt.Format(time.RFC3339Nano))
	}
}
