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
	"hostdeck/server/internal/storage"
)

type stubSessionAuthenticator struct {
	user domain.User
	err  error
}

func (s stubSessionAuthenticator) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if s.err != nil {
		return domain.User{}, s.err
	}
	return s.user, nil
}

func TestRouterAuth_ReadRoutesAllowViewer(t *testing.T) {
	serverHandler := api.NewServerHandler(viewerServerStore{}, viewerLiveServerLister{})
	alertHandler := api.NewAlertHandler(service.NewAlertService(viewerAlertRuleStore{}, viewerAlertServerStore{}, viewerAlertStatusStore{}))
	router := httpx.NewRouterWithHandlers(
		serverHandler,
		nil,
		nil,
		nil,
		nil,
		alertHandler,
		httpx.WithAuthHandler(api.NewAuthHandler(nil, "hostdeck_session", false, "")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(stubSessionAuthenticator{user: domain.User{ID: 1, Username: "viewer", Role: domain.RoleViewer, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, "hostdeck_session")),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)

	for _, path := range []string{"/api/servers", "/api/alert-rules"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "hostdeck_session", Value: "token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d", path, rec.Code)
		}
	}
}

func TestRouterAuth_WriteRoutesBlockViewer(t *testing.T) {
	serverHandler := api.NewServerHandler(viewerServerStore{}, viewerLiveServerLister{})
	alertHandler := api.NewAlertHandler(service.NewAlertService(viewerAlertRuleStore{}, viewerAlertServerStore{}, viewerAlertStatusStore{}))
	router := httpx.NewRouterWithHandlers(
		serverHandler,
		nil,
		nil,
		nil,
		nil,
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

type viewerAlertServerStore struct{}

func (viewerAlertServerStore) List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error) {
	return []domain.Server{{ID: 1, Name: "prod-web-01", Hostname: "prod-web-01", IP: "10.0.0.21", SSHPort: 22, Username: "root", CollectorMode: "ssh_only", Enabled: true}}, nil
}

type viewerAlertStatusStore struct{}

func (viewerAlertStatusStore) ListLatest(ctx context.Context) ([]storage.LatestStatus, error) {
	return []storage.LatestStatus{}, nil
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
