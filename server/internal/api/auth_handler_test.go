package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

func openAuthHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth-handler-test?mode=memory&cache=shared")
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

func newAuthRouterForTest(t *testing.T) http.Handler {
	t.Helper()
	db := openAuthHandlerTestDB(t)
	userRepo := storage.NewUserRepository(db)
	sessionRepo := storage.NewSessionRepository(db)
	eventRepo := storage.NewAuthEventRepository(db)
	authService := service.NewAuthService(userRepo, sessionRepo, eventRepo, 24*time.Hour)
	if err := authService.EnsureBootstrapAdmin(context.Background(), "", ""); err != nil {
		t.Fatalf("ensure bootstrap admin: %v", err)
	}

	return httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		httpx.WithAuthHandler(api.NewAuthHandler(authService, "hostdeck_session", false, "")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, "hostdeck_session")),
	)
}

func loginAsDefaultAdmin(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	return cookies[0]
}

func TestAuthHandlerCurrentUserReturnsPermissions(t *testing.T) {
	router := newAuthRouterForTest(t)
	cookie := loginAsDefaultAdmin(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Permissions struct {
			CanManageInfrastructure bool `json:"canManageInfrastructure"`
			CanManageUsers         bool `json:"canManageUsers"`
			CanChangeOwnPassword   bool `json:"canChangeOwnPassword"`
		} `json:"permissions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.User.Username != "admin" {
		t.Fatalf("unexpected username %q", payload.User.Username)
	}
	if !payload.Permissions.CanManageInfrastructure || !payload.Permissions.CanManageUsers || !payload.Permissions.CanChangeOwnPassword {
		t.Fatalf("unexpected permissions: %+v", payload.Permissions)
	}
}

func TestAuthHandlerListUsersRequiresAdmin(t *testing.T) {
	router := newAuthRouterForTest(t)
	cookie := loginAsDefaultAdmin(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatal("expected at least one user")
	}
}
