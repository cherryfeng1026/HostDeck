package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
)

func TestWebhookAlertNotifierSendsJSONPayload(t *testing.T) {
	t.Setenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS", "1")
	var received service.AlertNotification
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected content-type application/json, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, err := service.NewWebhookAlertNotifier(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}

	notification := service.AlertNotification{
		EventType: domain.AlertEventTriggered,
		Alert: domain.AlertEvent{
			ID:         11,
			RuleID:     3,
			ServerID:   7,
			ServerName: "prod-web-01",
			Metric:     "memory_usage",
			Status:     domain.AlertStatusActive,
			Message:    "内存使用率 90% 超过阈值 80%",
		},
		OccurredAt: time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC),
	}
	if err := notifier.NotifyAlert(context.Background(), notification); err != nil {
		t.Fatalf("notify alert: %v", err)
	}
	if received.EventType != notification.EventType {
		t.Fatalf("unexpected event type: %+v", received)
	}
	if received.Alert.ServerName != notification.Alert.ServerName || received.Alert.Message != notification.Alert.Message {
		t.Fatalf("unexpected alert payload: %+v", received.Alert)
	}
}

func TestWebhookAlertNotifierRejectsUnsupportedScheme(t *testing.T) {
	if _, err := service.NewWebhookAlertNotifier("ftp://hooks.example.test/alerts", time.Second); err == nil {
		t.Fatal("expected scheme validation error")
	}
}

func TestWebhookAlertNotifierRejectsLoopbackHost(t *testing.T) {
	if _, err := service.NewWebhookAlertNotifier("http://127.0.0.1:8080/alerts", time.Second); err == nil || !strings.Contains(err.Error(), "must not target") {
		t.Fatalf("expected private host validation error, got %v", err)
	}
}

func TestWebhookAlertNotifierReturnsHTTPError(t *testing.T) {
	t.Setenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "downstream rejected", http.StatusBadGateway)
	}))
	defer server.Close()

	notifier, err := service.NewWebhookAlertNotifier(server.URL, time.Second)
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}
	if err := notifier.NotifyAlert(context.Background(), service.AlertNotification{}); err == nil {
		t.Fatal("expected webhook error")
	}
}

func TestWebhookAlertNotifierRejectsUnsafeRedirect(t *testing.T) {
	t.Setenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://hooks.example.test/alerts", http.StatusFound)
	}))
	defer server.Close()

	notifier, err := service.NewWebhookAlertNotifier(server.URL, time.Second)
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}
	err = notifier.NotifyAlert(context.Background(), service.AlertNotification{})
	if err == nil || (!strings.Contains(err.Error(), "must use http or https") && !strings.Contains(err.Error(), "unsupported protocol")) {
		t.Fatalf("expected unsafe redirect validation error, got %v", err)
	}
}

func TestDynamicWebhookAlertNotifierRecordsDeliverySuccess(t *testing.T) {
	t.Setenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	deliveries := &notificationDeliveryStoreStub{}
	notifier := service.NewDynamicWebhookAlertNotifier(dynamicNotificationSettingsReader{settings: domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            server.URL,
		WebhookTimeoutSeconds: 2,
	}}, deliveries)
	payload := service.AlertNotification{EventType: domain.AlertEventTriggered, Alert: domain.AlertEvent{ID: 11, RuleID: 3, ServerID: 7, ServerName: "prod-web-01"}}

	if err := notifier.NotifyAlert(context.Background(), payload); err != nil {
		t.Fatalf("notify with deliveries: %v", err)
	}
	if len(deliveries.created) != 1 {
		t.Fatalf("expected one delivery record, got %d", len(deliveries.created))
	}
	if deliveries.created[0].Status != domain.AlertNotificationDeliveryPending || deliveries.createdPayloads[0] == "" {
		t.Fatalf("unexpected created delivery: %+v payload=%q", deliveries.created[0], deliveries.createdPayloads[0])
	}
	if len(deliveries.attempts) != 1 || deliveries.attempts[0].status != domain.AlertNotificationDeliverySent {
		t.Fatalf("expected sent attempt, got %+v", deliveries.attempts)
	}
}

func TestDynamicWebhookAlertNotifierRecordsDeliveryFailureAndRetries(t *testing.T) {
	t.Setenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary failure", http.StatusBadGateway)
	}))
	defer server.Close()

	deliveries := &notificationDeliveryStoreStub{}
	notifier := service.NewDynamicWebhookAlertNotifier(dynamicNotificationSettingsReader{settings: domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            server.URL,
		WebhookTimeoutSeconds: 2,
	}}, deliveries)

	err := notifier.NotifyAlert(context.Background(), service.AlertNotification{EventType: domain.AlertEventTriggered, Alert: domain.AlertEvent{ID: 11, RuleID: 3, ServerID: 7, ServerName: "prod-web-01"}})
	if err == nil {
		t.Fatal("expected webhook failure")
	}
	if len(deliveries.attempts) != 1 {
		t.Fatalf("expected one failed attempt, got %d", len(deliveries.attempts))
	}
	attempt := deliveries.attempts[0]
	if attempt.status != domain.AlertNotificationDeliveryFailed || attempt.nextAttemptAt == nil || attempt.lastError == "" {
		t.Fatalf("unexpected failed attempt: %+v", attempt)
	}

	retried, err := notifier.RetryPending(context.Background(), 5)
	if retried != 0 || err == nil {
		t.Fatalf("expected retry to fail with no success count, retried=%d err=%v", retried, err)
	}
	if len(deliveries.listFilters) != 1 || !deliveries.listFilters[0].DueOnly {
		t.Fatalf("expected due-only retry query, got %+v", deliveries.listFilters)
	}
}

func TestDynamicWebhookAlertNotifierSkipsDisabledSettings(t *testing.T) {
	notifier := service.NewDynamicWebhookAlertNotifier(dynamicNotificationSettingsReader{})
	if err := notifier.NotifyAlert(context.Background(), service.AlertNotification{}); err != nil {
		t.Fatalf("notify disabled settings: %v", err)
	}
}

func TestDynamicWebhookAlertNotifierUsesCurrentSettings(t *testing.T) {
	t.Setenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS", "1")
	var received service.AlertNotification
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier := service.NewDynamicWebhookAlertNotifier(dynamicNotificationSettingsReader{
		settings: domain.AlertNotificationSettings{
			Enabled:               true,
			WebhookURL:            server.URL,
			WebhookTimeoutSeconds: 2,
		},
	})
	payload := service.AlertNotification{EventType: domain.AlertEventTriggered, Alert: domain.AlertEvent{ServerName: "prod-web-01"}}
	if err := notifier.NotifyAlert(context.Background(), payload); err != nil {
		t.Fatalf("notify with dynamic settings: %v", err)
	}
	if received.Alert.ServerName != "prod-web-01" {
		t.Fatalf("unexpected payload: %+v", received)
	}
}

type notificationDeliveryAttemptRecord struct {
	deliveryID    int64
	status        string
	lastError     string
	nextAttemptAt *time.Time
}

type notificationDeliveryStoreStub struct {
	created         []domain.AlertNotificationDelivery
	createdPayloads []string
	attempts        []notificationDeliveryAttemptRecord
	listFilters     []domain.AlertNotificationDeliveryFilter
}

func (s *notificationDeliveryStoreStub) CreateNotificationDelivery(ctx context.Context, delivery domain.AlertNotificationDelivery, payload string) (domain.AlertNotificationDelivery, error) {
	delivery.ID = int64(len(s.created) + 1)
	delivery.Payload = payload
	s.created = append(s.created, delivery)
	s.createdPayloads = append(s.createdPayloads, payload)
	return delivery, nil
}

func (s *notificationDeliveryStoreStub) RecordNotificationDeliveryAttempt(ctx context.Context, deliveryID int64, status string, lastError string, nextAttemptAt *time.Time, attemptedAt time.Time) error {
	s.attempts = append(s.attempts, notificationDeliveryAttemptRecord{deliveryID: deliveryID, status: status, lastError: lastError, nextAttemptAt: nextAttemptAt})
	return nil
}

func (s *notificationDeliveryStoreStub) ListNotificationDeliveries(ctx context.Context, filter domain.AlertNotificationDeliveryFilter) ([]domain.AlertNotificationDelivery, error) {
	s.listFilters = append(s.listFilters, filter)
	items := make([]domain.AlertNotificationDelivery, len(s.created))
	copy(items, s.created)
	return items, nil
}

type dynamicNotificationSettingsReader struct {
	settings domain.AlertNotificationSettings
}

func (r dynamicNotificationSettingsReader) GetNotificationSettings(context.Context) (domain.AlertNotificationSettings, error) {
	return r.settings, nil
}
