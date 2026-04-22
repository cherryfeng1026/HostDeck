package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

func openAuthHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func newAuthRouterForTest(t *testing.T) http.Handler {
	t.Helper()
	db := openAuthHandlerTestDB(t)
	userRepo := storage.NewUserRepository(db)
	sessionRepo := storage.NewSessionRepository(db)
	eventRepo := storage.NewAuthEventRepository(db)
	apiTokenRepo := storage.NewAPITokenRepository(db)
	authService := service.NewAuthService(userRepo, sessionRepo, apiTokenRepo, eventRepo, 24*time.Hour)
	if _, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test_setup"); err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	return httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		httpx.WithAuthHandler(api.NewAuthHandler(authService, "hostdeck_session", false, "bootstrap-token")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, "hostdeck_session")),
	)
}

func loginAs(t *testing.T, router http.Handler, username string, password string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
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

func loginAsDefaultAdmin(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	return loginAs(t, router, "admin", "admin123")
}

func createUserAs(t *testing.T, router http.Handler, cookie *http.Cookie, username string, password string, role string) storage.UserRecord {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/users",
		strings.NewReader(fmt.Sprintf(`{"username":"%s","password":"%s","role":"%s"}`, username, password, role)),
	)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var created storage.UserRecord
	if err := json.NewDecoder(rec.Body).Decode(&created.User); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	return created
}

func TestAuthHandlerStatusReportsInitialization(t *testing.T) {
	router := newAuthRouterForTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Initialized      bool `json:"initialized"`
		BootstrapEnabled bool `json:"bootstrapEnabled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Initialized {
		t.Fatal("expected initialized to be true")
	}
	if !payload.BootstrapEnabled {
		t.Fatal("expected bootstrapEnabled to be true")
	}
}

func TestAuthHandlerLoginReturnsPreconditionFailedWhenUninitialized(t *testing.T) {
	db := openAuthHandlerTestDB(t)
	authService := service.NewAuthService(
		storage.NewUserRepository(db),
		storage.NewSessionRepository(db),
		storage.NewAPITokenRepository(db),
		storage.NewAuthEventRepository(db),
		24*time.Hour,
	)
	router := httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		httpx.WithAuthHandler(api.NewAuthHandler(authService, "hostdeck_session", false, "bootstrap-token")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, "hostdeck_session")),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412, got %d body=%s", rec.Code, rec.Body.String())
	}
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
			CanManageUsers          bool `json:"canManageUsers"`
			CanChangeOwnPassword    bool `json:"canChangeOwnPassword"`
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

func TestAuthHandlerUserManagementRoutesRejectOperator(t *testing.T) {
	router := newAuthRouterForTest(t)
	adminCookie := loginAsDefaultAdmin(t, router)
	managedUser := createUserAs(t, router, adminCookie, "viewer-2", "viewer123", "viewer")
	createUserAs(t, router, adminCookie, "operator-1", "operator123", "operator")
	operatorCookie := loginAs(t, router, "operator-1", "operator123")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list users", method: http.MethodGet, path: "/api/users"},
		{name: "create user", method: http.MethodPost, path: "/api/users", body: `{"username":"viewer-3","password":"viewer123","role":"viewer"}`},
		{name: "update user", method: http.MethodPut, path: fmt.Sprintf("/api/users/%d", managedUser.ID), body: `{"role":"admin","enabled":true}`},
		{name: "reset password", method: http.MethodPost, path: fmt.Sprintf("/api/users/%d/reset-password", managedUser.ID), body: `{"newPassword":"admin9999"}`},
		{name: "revoke sessions", method: http.MethodPost, path: fmt.Sprintf("/api/users/%d/revoke-sessions", managedUser.ID)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.AddCookie(operatorCookie)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
			}

			var payload map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["error"] != "当前账号没有查看用户管理信息的权限" {
				t.Fatalf("unexpected error message: %q", payload["error"])
			}
		})
	}
}

func TestAuthHandlerCreateAndUpdateUser(t *testing.T) {
	router := newAuthRouterForTest(t)
	cookie := loginAsDefaultAdmin(t, router)
	created := createUserAs(t, router, cookie, "viewer-1", "viewer123", "viewer")
	if created.Username != "viewer-1" || created.Role != "viewer" || !created.Enabled {
		t.Fatalf("unexpected created user: %+v", created)
	}

	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/users/%d", created.ID), strings.NewReader(`{"role":"operator","enabled":false}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	var updated storage.UserRecord
	if err := json.NewDecoder(updateRec.Body).Decode(&updated.User); err != nil {
		t.Fatalf("decode updated user: %v", err)
	}
	if updated.Role != "operator" || updated.Enabled {
		t.Fatalf("unexpected updated user: %+v", updated)
	}
}

func TestAuthHandlerResetPasswordAndRevokeSessions(t *testing.T) {
	router := newAuthRouterForTest(t)
	adminCookie := loginAsDefaultAdmin(t, router)
	operator := createUserAs(t, router, adminCookie, "operator-1", "operator123", "operator")
	operatorCookie := loginAs(t, router, "operator-1", "operator123")

	resetReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/users/%d/reset-password", operator.ID), strings.NewReader(`{"newPassword":"reset1234"}`))
	resetReq.Header.Set("Content-Type", "application/json")
	resetReq.AddCookie(adminCookie)
	resetRec := httptest.NewRecorder()
	router.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", resetRec.Code, resetRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"operator-1","password":"operator123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for old password, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}

	newOperatorCookie := loginAs(t, router, "operator-1", "reset1234")

	revokeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/users/%d/revoke-sessions", operator.ID), nil)
	revokeReq.AddCookie(adminCookie)
	revokeRec := httptest.NewRecorder()
	router.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", revokeRec.Code, revokeRec.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(operatorCookie)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected original session to be revoked, got %d body=%s", meRec.Code, meRec.Body.String())
	}

	meReq2 := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq2.AddCookie(newOperatorCookie)
	meRec2 := httptest.NewRecorder()
	router.ServeHTTP(meRec2, meReq2)
	if meRec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected new session to be revoked, got %d body=%s", meRec2.Code, meRec2.Body.String())
	}
}
