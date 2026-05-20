package httpx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
)

type stubSessionAuthenticator struct {
	user   domain.User
	scopes []string
	err    error
}

func (s stubSessionAuthenticator) AuthenticateSession(ctx context.Context, token string) (domain.User, error) {
	if s.err != nil {
		return domain.User{}, s.err
	}
	return s.user, nil
}

func (s stubSessionAuthenticator) AuthenticateAPIToken(ctx context.Context, token string) (domain.User, []string, error) {
	if s.err != nil {
		return domain.User{}, nil, s.err
	}
	if s.scopes != nil {
		return s.user, s.scopes, nil
	}
	return s.user, []string{domain.ScopeAll}, nil
}

func TestRouterAuth_ReadRoutesAllowViewer(t *testing.T) {
	serverHandler := api.NewServerHandler(viewerServerStore{}, viewerLiveServerLister{})
	alertHandler := api.NewAlertHandler(service.NewAlertService(viewerAlertRuleStore{}, viewerAlertStateStore{}, viewerAlertServerStore{}, nil))
	commandHandler := api.NewCommandHandler(service.NewCommandService(viewerCommandServerResolver{}, viewerCommandRunner{}, viewerCommandLogStore{}, viewerCommandTemplateStore{}))
	router := httpx.NewRouterWithHandlers(
		serverHandler,
		nil,
		nil,
		nil,
		commandHandler,
		alertHandler,
		httpx.WithAuthHandler(api.NewAuthHandler(nil, "hostdeck_session", false, "")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(stubSessionAuthenticator{user: domain.User{ID: 1, Username: "viewer", Role: domain.RoleViewer, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, "hostdeck_session")),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)

	for _, path := range []string{"/api/servers", "/api/alert-rules", "/api/commands/templates"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "hostdeck_session", Value: "token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d", path, rec.Code)
		}
	}
}

func TestRouterAuth_BearerTokenReadRoutesRequireMatchingScopes(t *testing.T) {
	serverHandler := api.NewServerHandler(viewerServerStore{}, viewerLiveServerLister{})
	alertHandler := api.NewAlertHandler(service.NewAlertService(viewerAlertRuleStore{}, viewerAlertStateStore{}, viewerAlertServerStore{}, nil))
	commandHandler := api.NewCommandHandler(service.NewCommandService(viewerCommandServerResolver{}, viewerCommandRunner{}, viewerCommandLogStore{}, viewerCommandTemplateStore{}))
	router := httpx.NewRouterWithHandlers(
		serverHandler,
		nil,
		nil,
		nil,
		commandHandler,
		alertHandler,
		httpx.WithAuthHandler(api.NewAuthHandler(nil, "hostdeck_session", false, "")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(stubSessionAuthenticator{
			user:   domain.User{ID: 1, Username: "api", Role: domain.RoleAdmin, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			scopes: []string{domain.ScopeCommandsRead},
		}, "hostdeck_session")),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)

	for _, item := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/api/servers", wantStatus: http.StatusForbidden},
		{path: "/api/alert-rules", wantStatus: http.StatusForbidden},
		{path: "/api/commands/templates", wantStatus: http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, item.path, nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != item.wantStatus {
			t.Fatalf("expected %s to return %d, got %d", item.path, item.wantStatus, rec.Code)
		}
	}
}

func TestRouterAuth_BearerTokenCommandWriteRoutesRequireSpecificScopes(t *testing.T) {
	for _, item := range []struct {
		name       string
		scopes     []string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "execute scope can execute commands",
			scopes:     []string{domain.ScopeCommandsExecute},
			method:     http.MethodPost,
			path:       "/api/servers/1/commands/execute",
			body:       `{"command":"df -h","timeoutSeconds":1}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "execute scope cannot mutate templates",
			scopes:     []string{domain.ScopeCommandsExecute},
			method:     http.MethodPost,
			path:       "/api/commands/templates",
			body:       `{"name":"disk","command":"df -h","scope":"personal","riskLevel":"normal"}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "template write scope can mutate templates",
			scopes:     []string{domain.ScopeCommandTemplatesWrite},
			method:     http.MethodPost,
			path:       "/api/commands/templates",
			body:       `{"name":"disk","command":"df -h","scope":"personal","riskLevel":"normal"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "template write scope cannot execute commands",
			scopes:     []string{domain.ScopeCommandTemplatesWrite},
			method:     http.MethodPost,
			path:       "/api/servers/1/commands/execute",
			body:       `{"command":"df -h","timeoutSeconds":1}`,
			wantStatus: http.StatusForbidden,
		},
	} {
		t.Run(item.name, func(t *testing.T) {
			commandHandler := api.NewCommandHandler(service.NewCommandService(viewerCommandServerResolver{}, viewerCommandRunner{}, viewerCommandLogStore{}, viewerCommandTemplateStore{}))
			router := httpx.NewRouterWithHandlers(
				nil,
				nil,
				nil,
				nil,
				commandHandler,
				nil,
				httpx.WithAuthHandler(api.NewAuthHandler(nil, "hostdeck_session", false, "")),
				httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(stubSessionAuthenticator{
					user:   domain.User{ID: 1, Username: "api", Role: domain.RoleAdmin, CreatedAt: time.Now(), UpdatedAt: time.Now()},
					scopes: item.scopes,
				}, "hostdeck_session")),
				httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
			)

			req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != item.wantStatus {
				t.Fatalf("expected %s %s to return %d, got %d", item.method, item.path, item.wantStatus, rec.Code)
			}
		})
	}
}

func TestRouterAuth_WriteRoutesBlockViewer(t *testing.T) {
	serverHandler := api.NewServerHandler(viewerServerStore{}, viewerLiveServerLister{})
	alertHandler := api.NewAlertHandler(service.NewAlertService(viewerAlertRuleStore{}, viewerAlertStateStore{}, viewerAlertServerStore{}, nil))
	commandHandler := api.NewCommandHandler(service.NewCommandService(viewerCommandServerResolver{}, viewerCommandRunner{}, viewerCommandLogStore{}, viewerCommandTemplateStore{}))
	router := httpx.NewRouterWithHandlers(
		serverHandler,
		nil,
		nil,
		nil,
		commandHandler,
		alertHandler,
		httpx.WithAuthHandler(api.NewAuthHandler(nil, "hostdeck_session", false, "")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(stubSessionAuthenticator{user: domain.User{ID: 1, Username: "viewer", Role: domain.RoleViewer, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, "hostdeck_session")),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/commands/history"},
		{method: http.MethodGet, path: "/api/alert-notification-settings"},
		{method: http.MethodPost, path: "/api/commands/templates", body: `{"name":"巡检 nginx","command":"systemctl status nginx","scope":"personal","riskLevel":"normal"}`},
		{method: http.MethodPost, path: "/api/commands/templates/shared-disk-usage/favorite", body: `{"favorite":true}`},
		{method: http.MethodPost, path: "/api/servers", body: `{"name":"prod-web-01","hostname":"prod-web-01","ip":"10.0.0.21","username":"root","authType":"password","password":"super-secret"}`},
		{method: http.MethodPost, path: "/api/alert-rules", body: `{"metric":"memory_usage","operator":"gte","threshold":70,"durationSeconds":60,"enabled":true}`},
	}

	for _, item := range requests {
		req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "hostdeck_session", Value: "token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected %s %s to return 403, got %d", item.method, item.path, rec.Code)
		}
		assertAuthErrorMessage(t, rec, "当前账号没有执行该操作的权限")
	}
}

func TestRouterAuth_SessionOnlyRoutesBlockBearerToken(t *testing.T) {
	router := httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		httpx.WithAuthHandler(api.NewAuthHandler(nil, "hostdeck_session", false, "")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(stubSessionAuthenticator{user: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, "hostdeck_session")),
	)

	for _, item := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/auth/me"},
		{method: http.MethodPost, path: "/api/auth/change-password", body: `{"currentPassword":"oldpass123","newPassword":"newpass123"}`},
		{method: http.MethodGet, path: "/api/auth/api-tokens"},
		{method: http.MethodPost, path: "/api/auth/api-tokens", body: `{"name":"ci","expiresInHours":24}`},
		{method: http.MethodDelete, path: "/api/auth/api-tokens/1"},
		{method: http.MethodGet, path: "/api/users"},
		{method: http.MethodPost, path: "/api/users", body: `{"username":"viewer-1","password":"viewer123","role":"viewer"}`},
		{method: http.MethodPut, path: "/api/users/1", body: `{"role":"viewer","enabled":true}`},
		{method: http.MethodPost, path: "/api/users/1/reset-password", body: `{"newPassword":"viewer123"}`},
		{method: http.MethodPost, path: "/api/users/1/revoke-sessions"},
	} {
		req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
		if item.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected %s %s to return 403, got %d", item.method, item.path, rec.Code)
		}
		assertAuthErrorMessage(t, rec, "API Token 不支持该操作")
	}
}

type viewerServerStore struct{}

func (viewerServerStore) Create(ctx context.Context, item domain.Server) error { return nil }
func (viewerServerStore) List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error) {
	return []domain.Server{{ID: 1, Name: "prod-web-01", Hostname: "prod-web-01", IP: "10.0.0.21", SSHPort: 22, Username: "root", AuthType: "password", CollectorMode: "ssh_only", Tags: []string{}, Enabled: true}}, nil
}
func (viewerServerStore) Update(ctx context.Context, item domain.Server) error { return nil }
func (viewerServerStore) Delete(ctx context.Context, id int64) error           { return nil }

type viewerLiveServerLister struct{}

func (viewerLiveServerLister) ListLive(ctx context.Context, filter storage.ServerFilter) ([]service.LiveServerItem, error) {
	return []service.LiveServerItem{{Server: domain.Server{ID: 1, Name: "prod-web-01", Hostname: "prod-web-01", IP: "10.0.0.21", SSHPort: 22, Username: "root", AuthType: "password", CollectorMode: "ssh_only", Tags: []string{}, Enabled: true}}}, nil
}

type viewerAlertRuleStore struct{}

func (viewerAlertRuleStore) List(ctx context.Context) ([]domain.AlertRule, error) {
	return []domain.AlertRule{{ID: 1, Metric: "memory_usage", Operator: "gte", Threshold: 70, DurationSeconds: 60, Enabled: true}}, nil
}
func (viewerAlertRuleStore) Create(ctx context.Context, rule domain.AlertRule) error { return nil }
func (viewerAlertRuleStore) Update(ctx context.Context, rule domain.AlertRule) error { return nil }
func (viewerAlertRuleStore) Delete(ctx context.Context, id int64) error              { return nil }

type viewerAlertServerStore struct{}

func (viewerAlertServerStore) List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error) {
	return []domain.Server{{ID: 1, Name: "prod-web-01", Hostname: "prod-web-01", IP: "10.0.0.21", SSHPort: 22, Username: "root", CollectorMode: "ssh_only", Enabled: true}}, nil
}

type viewerAlertStateStore struct{}

func (viewerAlertStateStore) UpsertEvaluation(ctx context.Context, record storage.AlertEvaluationRecord) (domain.AlertState, bool, error) {
	return domain.AlertState{}, false, nil
}

func (viewerAlertStateStore) ResolveByRuleAndServer(ctx context.Context, ruleID int64, serverID int64, detail string) (bool, error) {
	return false, nil
}

func (viewerAlertStateStore) ListCurrentStates(ctx context.Context) ([]domain.AlertState, error) {
	return []domain.AlertState{}, nil
}

func (viewerAlertStateStore) ListHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	return []domain.AlertHistoryEvent{}, nil
}

func (viewerAlertStateStore) Acknowledge(ctx context.Context, alertID int64, username string) (domain.AlertState, error) {
	return domain.AlertState{}, nil
}

func (viewerAlertStateStore) Mute(ctx context.Context, alertID int64, username string, mutedUntil time.Time) (domain.AlertState, error) {
	return domain.AlertState{}, nil
}

type viewerCommandServerResolver struct{}

func (viewerCommandServerResolver) ResolveServer(ctx context.Context, serverID int64) (domain.Server, error) {
	return domain.Server{ID: serverID, Name: "prod-web-01", Hostname: "prod-web-01", IP: "10.0.0.21", SSHPort: 22, Username: "root", AuthType: "password", Password: "super-secret", CollectorMode: "ssh_only", Enabled: true}, nil
}

type viewerCommandRunner struct{}

func (viewerCommandRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	return "ok", "", 0, nil
}

type viewerCommandLogStore struct{}

func (viewerCommandLogStore) Create(ctx context.Context, log domain.CommandLog) error {
	return nil
}

type viewerCommandTemplateStore struct{}

func (viewerCommandTemplateStore) EnsureDefaults(ctx context.Context, items []domain.CommandTemplate) error {
	return nil
}

func (viewerCommandTemplateStore) List(ctx context.Context, filter domain.CommandTemplateFilter) ([]domain.CommandTemplate, error) {
	return []domain.CommandTemplate{{
		ID:         "shared-disk-usage",
		Name:       "磁盘使用率",
		Command:    "df -h",
		Scope:      domain.CommandTemplateScopeShared,
		RiskLevel:  domain.CommandTemplateRiskNormal,
		IsFavorite: false,
	}}, nil
}

func (viewerCommandTemplateStore) GetByID(ctx context.Context, templateID string, username string) (domain.CommandTemplate, error) {
	return domain.CommandTemplate{ID: templateID, Scope: domain.CommandTemplateScopeShared, RiskLevel: domain.CommandTemplateRiskNormal}, nil
}

func (viewerCommandTemplateStore) Create(ctx context.Context, input domain.CommandTemplateCreateInput, username string) (domain.CommandTemplate, error) {
	return domain.CommandTemplate{}, nil
}

func (viewerCommandTemplateStore) SetFavorite(ctx context.Context, templateID string, username string, favorite bool) error {
	return nil
}

func (viewerCommandLogStore) ListHistory(ctx context.Context, filter domain.CommandHistoryFilter) ([]domain.CommandLog, error) {
	return []domain.CommandLog{{
		ID:               1,
		ServerID:         1,
		ServerName:       "prod-web-01",
		ExecutorUsername: "viewer",
		Command:          "df -h",
		Stdout:           "ok",
		Stderr:           "",
		ExitCode:         0,
		DurationMS:       10,
		ExecutedAt:       time.Now(),
	}}, nil
}

func assertAuthErrorMessage(t *testing.T, rec *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode auth error response: %v", err)
	}
	if payload["error"] != expected {
		t.Fatalf("expected error %q, got %q", expected, payload["error"])
	}
}
