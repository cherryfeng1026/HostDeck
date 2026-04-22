package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
)

func TestWebhookAlertNotifierSendsJSONPayload(t *testing.T) {
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

func TestWebhookAlertNotifierReturnsHTTPError(t *testing.T) {
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

func TestDynamicWebhookAlertNotifierSkipsDisabledSettings(t *testing.T) {
	notifier := service.NewDynamicWebhookAlertNotifier(dynamicNotificationSettingsReader{})
	if err := notifier.NotifyAlert(context.Background(), service.AlertNotification{}); err != nil {
		t.Fatalf("notify disabled settings: %v", err)
	}
}

func TestDynamicWebhookAlertNotifierUsesCurrentSettings(t *testing.T) {
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

type dynamicNotificationSettingsReader struct {
	settings domain.AlertNotificationSettings
}

func (r dynamicNotificationSettingsReader) GetNotificationSettings(context.Context) (domain.AlertNotificationSettings, error) {
	return r.settings, nil
}
