package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

func openAuthServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth-service-test?mode=memory&cache=shared")
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

func newAuthServiceForTest(db *sql.DB) *service.AuthService {
	return service.NewAuthService(
		storage.NewUserRepository(db),
		storage.NewSessionRepository(db),
		storage.NewAuthEventRepository(db),
		24*time.Hour,
	)
}

func TestEnsureBootstrapAdminCreatesDefaultAdminOnEmptyDB(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	if err := authService.EnsureBootstrapAdmin(context.Background(), "", ""); err != nil {
		t.Fatalf("ensure bootstrap admin: %v", err)
	}

	userRepo := storage.NewUserRepository(db)
	count, err := userRepo.Count(context.Background())
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	record, err := userRepo.GetByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("get admin: %v", err)
	}
	if record.Role != domain.RoleAdmin {
		t.Fatalf("expected admin role, got %q", record.Role)
	}

	if _, _, _, err := authService.Login(context.Background(), "admin", "admin123", "127.0.0.1", "test"); err != nil {
		t.Fatalf("login default admin: %v", err)
	}
}

func TestEnsureBootstrapAdminUsesConfiguredCredentials(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	if err := authService.EnsureBootstrapAdmin(context.Background(), "root-admin", "strongpass123"); err != nil {
		t.Fatalf("ensure bootstrap admin: %v", err)
	}

	userRepo := storage.NewUserRepository(db)
	if _, err := userRepo.GetByUsername(context.Background(), "root-admin"); err != nil {
		t.Fatalf("get configured admin: %v", err)
	}
}

func TestEnsureBootstrapAdminIgnoresDefaultCreationWhenUsersExist(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	if _, err := authService.CreateInitialAdmin(context.Background(), "seed-admin", "seedpass123", "", "", "test"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	if err := authService.EnsureBootstrapAdmin(context.Background(), "", ""); err != nil {
		t.Fatalf("ensure bootstrap admin: %v", err)
	}

	userRepo := storage.NewUserRepository(db)
	count, err := userRepo.Count(context.Background())
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected still 1 user, got %d", count)
	}
}

func TestEnsureBootstrapAdminRejectsPartialConfig(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	if err := authService.EnsureBootstrapAdmin(context.Background(), "admin", ""); err == nil {
		t.Fatal("expected partial bootstrap config to fail")
	}
}
