package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	"hostdeck/server/internal/testsupport"
)

type alertHistoryResponse struct {
	ID         int64  `json:"id"`
	AlertID    int64  `json:"alertId"`
	ServerName string `json:"serverName"`
	EventType  string `json:"eventType"`
	Status     string `json:"status"`
}

type alertEventResponse struct {
	ID             int64   `json:"id"`
	Status         string  `json:"status"`
	AcknowledgedBy string  `json:"acknowledgedBy"`
	MutedUntil     *string `json:"mutedUntil"`
}

type alertNotificationSettingsResponse struct {
	Enabled               bool   `json:"enabled"`
	WebhookURL            string `json:"webhookURL"`
	WebhookConfigured     bool   `json:"webhookConfigured"`
	WebhookTimeoutSeconds int    `json:"webhookTimeoutSeconds"`
}

func openAlertHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func newAlertRouterForTest(t *testing.T) (http.Handler, *sql.DB, *storage.AlertRepository, *storage.AuditEventRepository) {
	t.Helper()

	db := openAlertHandlerTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	if err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}

	alertRepo := storage.NewAlertRepository(db, "test-master-key")
	auditRepo := storage.NewAuditEventRepository(db)
	authService := service.NewAuthService(
		storage.NewUserRepository(db),
		storage.NewSessionRepository(db),
		storage.NewAPITokenRepository(db),
		storage.NewAuthEventRepository(db),
		24*time.Hour,
	)
	if _, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test_setup"); err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	alertService := service.NewAlertService(alertRepo, alertRepo, serverRepo, alertRepo)
	router := httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		nil,
		api.NewAlertHandler(alertService, auditRepo),
		httpx.WithAuthHandler(api.NewAuthHandler(authService, "hostdeck_session", false, "bootstrap-token")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, "hostdeck_session")),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)
	return router, db, alertRepo, auditRepo
}

func loginAsAdmin(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	body := strings.NewReader(`{"username":"admin","password":"admin123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
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

func seedActiveAlert(t *testing.T, repo *storage.AlertRepository) int64 {
	t.Helper()
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if err := repo.Create(context.Background(), domain.AlertRule{
		Metric:          "memory_usage",
		Operator:        "gte",
		Threshold:       80,
		DurationSeconds: 60,
		Enabled:         true,
	}); err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	state, _, err := repo.UpsertEvaluation(context.Background(), storage.AlertEvaluationRecord{
		RuleID:          1,
		ServerID:        1,
		Metric:          "memory_usage",
		Operator:        "gte",
		Threshold:       80,
		CurrentValue:    90,
		Severity:        "warning",
		Message:         "内存使用率 90% 超过阈值 80%",
		DurationSeconds: 60,
		TriggeredAt:     now.Add(-2 * time.Minute),
		LastTriggeredAt: now,
		Status:          domain.AlertStatusActive,
	})
	if err != nil {
		t.Fatalf("seed alert state: %v", err)
	}
	return state.ID
}

func TestAlertHandler_ListHistoryReturnsTriggeredEvents(t *testing.T) {
	router, _, alertRepo, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)
	seedActiveAlert(t, alertRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/alert-history?limit=10", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var items []alertHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
	if items[0].EventType != domain.AlertEventTriggered {
		t.Fatalf("expected triggered event, got %+v", items[0])
	}
	if items[0].ServerName != "prod-web-01" {
		t.Fatalf("expected server name to be joined, got %+v", items[0])
	}
}

func TestAlertHandler_AcknowledgeAlertUpdatesState(t *testing.T) {
	router, _, alertRepo, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)
	alertID := seedActiveAlert(t, alertRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/ack", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload alertEventResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode ack response: %v", err)
	}
	if payload.ID != alertID || payload.Status != domain.AlertStatusAcknowledged {
		t.Fatalf("unexpected ack payload: %+v", payload)
	}
	if payload.AcknowledgedBy != "admin" {
		t.Fatalf("expected acknowledgedBy=admin, got %+v", payload)
	}

	state, err := alertRepo.GetStateByRuleAndServer(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("load alert state: %v", err)
	}
	if state.Status != domain.AlertStatusAcknowledged {
		t.Fatalf("expected acknowledged state, got %+v", state)
	}
	if state.AcknowledgedBy != "admin" || state.AcknowledgedAt == nil {
		t.Fatalf("expected acknowledgment metadata, got %+v", state)
	}

}

func TestAlertHandler_MuteAlertUpdatesState(t *testing.T) {
	router, _, alertRepo, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)
	alertID := seedActiveAlert(t, alertRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/mute", strings.NewReader(`{"durationMinutes":45}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload alertEventResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode mute response: %v", err)
	}
	if payload.ID != alertID || payload.Status != domain.AlertStatusMuted {
		t.Fatalf("unexpected mute payload: %+v", payload)
	}
	if payload.MutedUntil == nil || *payload.MutedUntil == "" {
		t.Fatalf("expected mutedUntil to be returned, got %+v", payload)
	}

	state, err := alertRepo.GetStateByRuleAndServer(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("load alert state: %v", err)
	}
	if state.Status != domain.AlertStatusMuted {
		t.Fatalf("expected muted state, got %+v", state)
	}
	if state.MutedUntil == nil {
		t.Fatalf("expected mutedUntil to persist, got %+v", state)
	}

}

func TestAlertHandler_AcknowledgeAlertReturnsConflictForAcknowledgedState(t *testing.T) {
	router, _, alertRepo, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)
	alertID := seedActiveAlert(t, alertRepo)

	ackReq := httptest.NewRequest(http.MethodPost, "/api/alerts/1/ack", nil)
	ackReq.AddCookie(cookie)
	ackRec := httptest.NewRecorder()
	router.ServeHTTP(ackRec, ackReq)
	if ackRec.Code != http.StatusOK {
		t.Fatalf("expected first ack 200, got %d body=%s", ackRec.Code, ackRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/ack", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if payload["error"] == "" || alertID == 0 {
		t.Fatalf("expected conflict error payload, got %+v", payload)
	}
}

func TestAlertHandler_GetNotificationSettingsReturnsDefaults(t *testing.T) {
	router, _, _, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/alert-notification-settings", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload alertNotificationSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if payload.Enabled {
		t.Fatalf("expected notifications disabled by default, got %+v", payload)
	}
	if payload.WebhookConfigured {
		t.Fatalf("expected webhook not configured by default, got %+v", payload)
	}
	if payload.WebhookURL != "" {
		t.Fatalf("expected webhook url to be redacted, got %+v", payload)
	}
	if payload.WebhookTimeoutSeconds != 5 {
		t.Fatalf("expected default timeout 5, got %+v", payload)
	}
}

func TestAlertHandler_UpdateNotificationSettingsPersistsAndAudits(t *testing.T) {
	router, db, alertRepo, auditRepo := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)

	req := httptest.NewRequest(http.MethodPut, "/api/alert-notification-settings", strings.NewReader(`{"enabled":true,"webhookURL":"https://hooks.example.test/alerts","webhookTimeoutSeconds":8}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload alertNotificationSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if !payload.Enabled || !payload.WebhookConfigured || payload.WebhookURL != "" || payload.WebhookTimeoutSeconds != 8 {
		t.Fatalf("unexpected notification settings payload: %+v", payload)
	}

	stored, err := alertRepo.GetNotificationSettings(context.Background())
	if err != nil {
		t.Fatalf("load stored notification settings: %v", err)
	}
	if !stored.Enabled || stored.WebhookURL != "https://hooks.example.test/alerts" || !stored.WebhookConfigured {
		t.Fatalf("unexpected stored notification settings: %+v", stored)
	}
	var rawWebhookURL string
	if err := db.QueryRowContext(context.Background(), `SELECT webhook_url FROM alert_notification_settings LIMIT 1`).Scan(&rawWebhookURL); err != nil {
		t.Fatalf("load raw notification settings row: %v", err)
	}
	if rawWebhookURL == "https://hooks.example.test/alerts" || strings.Contains(rawWebhookURL, "hooks.example.test") {
		t.Fatalf("expected webhook url to be stored encrypted, got %q", rawWebhookURL)
	}

	audits, err := auditRepo.ListRecent(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audits))
	}
	if audits[0].Kind != domain.AuditKindAlert || audits[0].Title != "更新通知设置" {
		t.Fatalf("unexpected audit event: %+v", audits[0])
	}
	if audits[0].Summary == "https://hooks.example.test/alerts" || strings.Contains(audits[0].Summary, "hooks.example.test") {
		t.Fatalf("expected audit summary to be redacted, got %+v", audits[0])
	}
}

func TestAlertHandler_MuteAlertRejectsPendingState(t *testing.T) {
	router, _, alertRepo, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)
	seedPendingAlert(t, alertRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/mute", strings.NewReader(`{"durationMinutes":15}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAlertHandler_UpdateNotificationSettingsRejectsInvalidPayload(t *testing.T) {
	router, _, _, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)

	req := httptest.NewRequest(http.MethodPut, "/api/alert-notification-settings", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAlertHandler_UpdateNotificationSettingsClearsWebhook(t *testing.T) {
	router, _, alertRepo, _ := newAlertRouterForTest(t)
	cookie := loginAsAdmin(t, router)

	seedReq := httptest.NewRequest(http.MethodPut, "/api/alert-notification-settings", strings.NewReader(`{"enabled":true,"webhookURL":"https://hooks.example.test/alerts","webhookTimeoutSeconds":8}`))
	seedReq.Header.Set("Content-Type", "application/json")
	seedReq.AddCookie(cookie)
	seedRec := httptest.NewRecorder()
	router.ServeHTTP(seedRec, seedReq)
	if seedRec.Code != http.StatusOK {
		t.Fatalf("expected seed request 200, got %d body=%s", seedRec.Code, seedRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPut, "/api/alert-notification-settings", strings.NewReader(`{"enabled":false,"clearWebhookURL":true,"webhookTimeoutSeconds":6}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload alertNotificationSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if payload.WebhookURL != "" || payload.WebhookConfigured {
		t.Fatalf("expected cleared and redacted webhook settings, got %+v", payload)
	}

	stored, err := alertRepo.GetNotificationSettings(context.Background())
	if err != nil {
		t.Fatalf("load stored notification settings: %v", err)
	}
	if stored.WebhookURL != "" || stored.WebhookConfigured {
		t.Fatalf("expected stored webhook settings to be cleared, got %+v", stored)
	}
}

func TestAlertHandler_UpdateNotificationSettingsReturnsInternalErrorForStoreFailure(t *testing.T) {
	router := newAlertRouterWithSettingsStore(t, failingAlertNotificationSettingsStore{err: errors.New("store unavailable")})
	cookie := loginAsAdmin(t, router)

	req := httptest.NewRequest(http.MethodPut, "/api/alert-notification-settings", strings.NewReader(`{"enabled":true,"webhookURL":"https://hooks.example.test/alerts","webhookTimeoutSeconds":8}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type failingAlertNotificationSettingsStore struct {
	err error
}

func (s failingAlertNotificationSettingsStore) GetNotificationSettings(context.Context) (domain.AlertNotificationSettings, error) {
	return domain.AlertNotificationSettings{}, nil
}

func (s failingAlertNotificationSettingsStore) SaveNotificationSettings(context.Context, domain.AlertNotificationSettings) (domain.AlertNotificationSettings, error) {
	return domain.AlertNotificationSettings{}, s.err
}

func newAlertRouterWithSettingsStore(t *testing.T, settingsStore service.AlertNotificationSettingsStore) http.Handler {
	t.Helper()

	db := openAlertHandlerTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	if err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}

	authService := service.NewAuthService(
		storage.NewUserRepository(db),
		storage.NewSessionRepository(db),
		storage.NewAPITokenRepository(db),
		storage.NewAuthEventRepository(db),
		24*time.Hour,
	)
	if _, err := authService.CreateInitialAdmin(context.Background(), "admin", "admin123", "", "", "test_setup"); err != nil {
		t.Fatalf("create initial admin: %v", err)
	}

	alertService := service.NewAlertService(storage.NewAlertRepository(db, "test-master-key"), storage.NewAlertRepository(db, "test-master-key"), serverRepo, settingsStore)
	return httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		nil,
		api.NewAlertHandler(alertService),
		httpx.WithAuthHandler(api.NewAuthHandler(authService, "hostdeck_session", false, "bootstrap-token")),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, "hostdeck_session")),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)
}

func seedPendingAlert(t *testing.T, repo *storage.AlertRepository) int64 {
	t.Helper()
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if err := repo.Create(context.Background(), domain.AlertRule{
		Metric:          "memory_usage",
		Operator:        "gte",
		Threshold:       80,
		DurationSeconds: 60,
		Enabled:         true,
	}); err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	state, _, err := repo.UpsertEvaluation(context.Background(), storage.AlertEvaluationRecord{
		RuleID:          1,
		ServerID:        1,
		Metric:          "memory_usage",
		Operator:        "gte",
		Threshold:       80,
		CurrentValue:    90,
		Severity:        "warning",
		Message:         "内存使用率 90% 超过阈值 80%",
		DurationSeconds: 60,
		TriggeredAt:     now,
		LastTriggeredAt: now,
		Status:          domain.AlertStatusPending,
	})
	if err != nil {
		t.Fatalf("seed pending alert state: %v", err)
	}
	return state.ID
}
