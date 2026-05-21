package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

func openAuthServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func newAuthServiceForTest(db *sql.DB) *service.AuthService {
	return service.NewAuthService(
		storage.NewUserRepository(db),
		storage.NewSessionRepository(db),
		storage.NewAPITokenRepository(db),
		storage.NewAuthEventRepository(db),
		24*time.Hour,
	)
}

func TestEnsureBootstrapAdminAllowsUninitializedSystemWithoutConfig(t *testing.T) {
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
	if count != 0 {
		t.Fatalf("expected 0 users, got %d", count)
	}
}

func TestEnsureBootstrapAdminUsesConfiguredCredentials(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	if err := authService.EnsureBootstrapAdmin(context.Background(), "root-admin", "strongpass123"); err != nil {
		t.Fatalf("ensure bootstrap admin: %v", err)
	}

	userRepo := storage.NewUserRepository(db)
	record, err := userRepo.GetByUsername(context.Background(), "root-admin")
	if err != nil {
		t.Fatalf("get configured admin: %v", err)
	}
	if record.Role != domain.RoleAdmin {
		t.Fatalf("expected admin role, got %q", record.Role)
	}
}

func TestEnsureBootstrapAdminIgnoresConfiguredCreationWhenUsersExist(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	if _, err := authService.CreateInitialAdmin(context.Background(), "seed-admin", "seedpass123", "", "", "test"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	if err := authService.EnsureBootstrapAdmin(context.Background(), "root-admin", "strongpass123"); err != nil {
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

func TestLoginReturnsSystemUninitializedWhenNoUsers(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	_, _, _, err := authService.Login(context.Background(), "admin", "admin123", "127.0.0.1", "test")
	if !errors.Is(err, service.ErrSystemUninitialized) {
		t.Fatalf("expected ErrSystemUninitialized, got %v", err)
	}
}

func TestCreateInitialAdminClearsUninitializedCache(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)

	_, _, _, err := authService.Login(context.Background(), "admin", "admin123", "127.0.0.1", "test")
	if !errors.Is(err, service.ErrSystemUninitialized) {
		t.Fatalf("expected ErrSystemUninitialized, got %v", err)
	}

	if _, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "127.0.0.1", "test", "test"); err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	if _, _, _, err := authService.Login(context.Background(), "admin", "admin123", "127.0.0.1", "test"); err != nil {
		t.Fatalf("login after initial admin creation: %v", err)
	}
}

func TestCreateUserAndListUsers(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)
	admin, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test")
	if err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	user, err := authService.CreateUser(context.Background(), admin, "viewer-1", "viewer123", domain.RoleViewer, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Role != domain.RoleViewer || !user.Enabled {
		t.Fatalf("unexpected user: %+v", user)
	}

	users, err := authService.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestUpdateUserRejectsDisablingLastEnabledAdmin(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)
	admin, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test")
	if err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	_, err = authService.UpdateUser(context.Background(), admin, admin.ID, domain.RoleAdmin, false, "127.0.0.1", "test")
	if !errors.Is(err, service.ErrCannotDisableSelf) {
		t.Fatalf("expected ErrCannotDisableSelf, got %v", err)
	}

	secondAdmin, err := authService.CreateUser(context.Background(), admin, "admin-2", "admin234", domain.RoleAdmin, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	if _, err := authService.UpdateUser(context.Background(), admin, secondAdmin.ID, domain.RoleViewer, false, "127.0.0.1", "test"); err != nil {
		t.Fatalf("disable second admin with first still enabled: %v", err)
	}
	_, err = authService.UpdateUser(context.Background(), admin, admin.ID, domain.RoleViewer, false, "127.0.0.1", "test")
	if err == nil {
		t.Fatal("expected self update to fail")
	}
}

func TestLoginRejectsDisabledUser(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)
	admin, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test")
	if err != nil {
		t.Fatalf("create initial admin: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), admin, "viewer-1", "viewer123", domain.RoleViewer, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := authService.UpdateUser(context.Background(), admin, user.ID, domain.RoleViewer, false, "127.0.0.1", "test"); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	_, _, _, err = authService.Login(context.Background(), "viewer-1", "viewer123", "127.0.0.1", "test")
	if !errors.Is(err, service.ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}

func TestListAPITokensHidesExpiredTokens(t *testing.T) {
	db := openAuthServiceTestDB(t)
	authService := newAuthServiceForTest(db)
	admin, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test")
	if err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	if _, _, err := authService.CreateAPIToken(context.Background(), admin, "expired", 1, []string{domain.ScopeAll}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("create api token: %v", err)
	}

	repo := storage.NewAPITokenRepository(db)
	records, err := repo.ListActiveByUserID(context.Background(), admin.ID)
	if err != nil {
		t.Fatalf("list active tokens: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 active token, got %d", len(records))
	}

	if err := repo.DeleteExpiredOrRevokedBefore(context.Background(), time.Now().UTC().Add(2*time.Hour)); err != nil {
		t.Fatalf("delete expired tokens: %v", err)
	}

	items, err := authService.ListAPITokens(context.Background(), admin)
	if err != nil {
		t.Fatalf("list api tokens: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected expired tokens to be hidden, got %d", len(items))
	}
}
