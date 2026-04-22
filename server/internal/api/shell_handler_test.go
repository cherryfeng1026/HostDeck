package api_test

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

type shellAlertReaderStub struct {
	history []domain.AlertHistoryEvent
}

func (s shellAlertReaderStub) ListCurrentAlerts(ctx context.Context) ([]domain.AlertEvent, error) {
	return nil, nil
}

func (s shellAlertReaderStub) ListAlertHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	return s.history, nil
}

type shellCommandReaderStub struct{}

func (s shellCommandReaderStub) ListRecent(ctx context.Context, limit int, keyword string) ([]storage.CommandLogListItem, error) {
	return nil, nil
}

type shellAuthReaderStub struct {
	items []domain.AuthEvent
}

func (s shellAuthReaderStub) ListRecent(ctx context.Context, limit int, keyword string, eventTypes ...string) ([]domain.AuthEvent, error) {
	return s.items, nil
}

type shellAuditReaderStub struct{}

func (s shellAuditReaderStub) ListRecent(ctx context.Context, limit int, keyword string, kinds ...string) ([]domain.AuditEvent, error) {
	return nil, nil
}

type shellNotificationStateStub struct {
	readAt    *time.Time
	updatedAt *time.Time
}

func (s *shellNotificationStateStub) GetNotificationReadAt(ctx context.Context, userID int64) (*time.Time, error) {
	return s.readAt, nil
}

func (s *shellNotificationStateStub) UpdateNotificationReadAt(ctx context.Context, userID int64, readAt time.Time) error {
	value := readAt.UTC()
	s.readAt = &value
	if s.updatedAt != nil {
		*s.updatedAt = value
	}
	return nil
}

func newShellRouterForTest(t *testing.T, shellService *service.ShellService) http.Handler {
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
		httpx.WithShellHandler(api.NewShellHandler(shellService)),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, "hostdeck_session")),
	)
}

func TestShellHandlerListNotificationsReturnsUnreadState(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	readAt := now.Add(-90 * time.Second)
	shellService := service.NewShellService(
		shellAlertReaderStub{history: []domain.AlertHistoryEvent{{
			EventType:  domain.AlertEventTriggered,
			Severity:   "warning",
			Message:    "磁盘使用率过高",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now.Add(-2 * time.Minute),
		}, {
			EventType:  domain.AlertEventResolved,
			Severity:   "info",
			Message:    "磁盘使用率恢复",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now,
		}}},
		shellCommandReaderStub{},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			Username:  "admin",
			EventType: domain.AuthEventLoginFailed,
			CreatedAt: now.Add(-time.Minute),
		}}},
		shellAuditReaderStub{},
		&shellNotificationStateStub{readAt: &readAt},
	)
	router := newShellRouterForTest(t, shellService)
	cookie := loginAsDefaultAdmin(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications?limit=10", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload service.NotificationList
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.UnreadCount != 2 {
		t.Fatalf("expected 2 unread notifications, got %d", payload.UnreadCount)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(payload.Items))
	}
	if payload.Items[0].IsRead || payload.Items[1].IsRead {
		t.Fatalf("expected newest notifications unread, got %+v", payload.Items)
	}
	if !payload.Items[2].IsRead {
		t.Fatalf("expected oldest notification read, got %+v", payload.Items[2])
	}
}

func TestShellHandlerMarkNotificationsReadPersistsCursor(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	var updatedAt time.Time
	state := &shellNotificationStateStub{updatedAt: &updatedAt}
	shellService := service.NewShellService(
		shellAlertReaderStub{history: []domain.AlertHistoryEvent{{
			EventType:  domain.AlertEventTriggered,
			Severity:   "warning",
			Message:    "磁盘使用率过高",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now.Add(-2 * time.Minute),
		}}},
		shellCommandReaderStub{},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			Username:  "admin",
			EventType: domain.AuthEventLoginFailed,
			CreatedAt: now.Add(-time.Minute),
		}}},
		shellAuditReaderStub{},
		state,
	)
	router := newShellRouterForTest(t, shellService)
	cookie := loginAsDefaultAdmin(t, router)

	markReq := httptest.NewRequest(http.MethodPost, "/api/notifications/read", strings.NewReader(`{"readBefore":"2026-04-21T09:59:00Z"}`))
	markReq.Header.Set("Content-Type", "application/json")
	markReq.AddCookie(cookie)
	markRec := httptest.NewRecorder()
	router.ServeHTTP(markRec, markReq)

	if markRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", markRec.Code, markRec.Body.String())
	}
	if updatedAt.IsZero() {
		t.Fatal("expected notification read cursor to be updated")
	}
	expectedReadAt := time.Date(2026, 4, 21, 9, 59, 0, 0, time.UTC)
	if !updatedAt.Equal(expectedReadAt) {
		t.Fatalf("expected read cursor %s, got %s", expectedReadAt, updatedAt)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/notifications?limit=10", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload service.NotificationList
	if err := json.NewDecoder(listRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.UnreadCount != 0 {
		t.Fatalf("expected all notifications read, got unreadCount=%d", payload.UnreadCount)
	}
	for _, item := range payload.Items {
		if !item.IsRead {
			t.Fatalf("expected all notifications read after mark, got %+v", payload.Items)
		}
	}
}

func TestShellHandlerViewerCannotSeeAuthEvents(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	shellService := service.NewShellService(
		shellAlertReaderStub{history: []domain.AlertHistoryEvent{{
			EventType:  domain.AlertEventTriggered,
			Severity:   "warning",
			Message:    "磁盘使用率过高",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now.Add(-2 * time.Minute),
		}}},
		shellCommandReaderStub{},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			Username:  "viewer-user",
			EventType: domain.AuthEventLoginFailed,
			Detail:    "bad password",
			CreatedAt: now.Add(-time.Minute),
		}}},
		shellAuditReaderStub{},
		&shellNotificationStateStub{},
	)
	router := newShellRouterForTest(t, shellService)
	adminCookie := loginAsDefaultAdmin(t, router)
	createUserAs(t, router, adminCookie, "viewer-user", "viewer123", "viewer")
	viewerCookie := loginAs(t, router, "viewer-user", "viewer123")

	notificationReq := httptest.NewRequest(http.MethodGet, "/api/notifications?limit=10", nil)
	notificationReq.AddCookie(viewerCookie)
	notificationRec := httptest.NewRecorder()
	router.ServeHTTP(notificationRec, notificationReq)
	if notificationRec.Code != http.StatusOK {
		t.Fatalf("expected notifications 200, got %d body=%s", notificationRec.Code, notificationRec.Body.String())
	}
	var notifications service.NotificationList
	if err := json.NewDecoder(notificationRec.Body).Decode(&notifications); err != nil {
		t.Fatalf("decode notifications: %v", err)
	}
	for _, item := range notifications.Items {
		if item.Kind == "auth" {
			t.Fatalf("expected auth notifications hidden for viewer, got %+v", notifications.Items)
		}
	}

	activityReq := httptest.NewRequest(http.MethodGet, "/api/activity-feed?limit=10", nil)
	activityReq.AddCookie(viewerCookie)
	activityRec := httptest.NewRecorder()
	router.ServeHTTP(activityRec, activityReq)
	if activityRec.Code != http.StatusOK {
		t.Fatalf("expected activity 200, got %d body=%s", activityRec.Code, activityRec.Body.String())
	}
	var activity service.ShellEventList
	if err := json.NewDecoder(activityRec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	for _, item := range activity.Items {
		if item.Kind == "auth" {
			t.Fatalf("expected auth activity hidden for viewer, got %+v", activity.Items)
		}
	}

	searchReq := httptest.NewRequest(http.MethodGet, "/api/search?q=viewer-user&limit=10", nil)
	searchReq.AddCookie(viewerCookie)
	searchRec := httptest.NewRecorder()
	router.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("expected search 200, got %d body=%s", searchRec.Code, searchRec.Body.String())
	}
	var search service.SearchResults
	if err := json.NewDecoder(searchRec.Body).Decode(&search); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if len(search.Items) != 0 {
		t.Fatalf("expected viewer auth search to be empty, got %+v", search.Items)
	}
}
