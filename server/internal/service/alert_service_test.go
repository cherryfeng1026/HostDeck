package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

func TestEvaluateAlerts_OfflineRule(t *testing.T) {
	rules := []domain.AlertRule{
		{ID: 1, Metric: "online", Operator: "eq", Threshold: 0, DurationSeconds: 60, Enabled: true},
	}

	events := service.EvaluateAlerts(rules, collector.Snapshot{Online: false, Source: "ssh"}, time.Now())
	if len(events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(events))
	}
	if events[0].Metric != "online" || events[0].Severity != "critical" {
		t.Fatalf("unexpected alert event: %+v", events[0])
	}
}

func TestCreateRule_PreservesDisabledState(t *testing.T) {
	store := &alertRuleStoreStub{}
	svc := service.NewAlertService(store, nil, nil, nil)

	err := svc.CreateRule(context.Background(), domain.AlertRule{
		Metric:          "cpu_usage",
		Operator:        "gte",
		Threshold:       90,
		DurationSeconds: 60,
		Enabled:         false,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	if !store.called {
		t.Fatalf("expected store create to be called")
	}
	if store.lastRule.Enabled {
		t.Fatalf("expected disabled rule to stay disabled")
	}
}

func TestAlertService_EvaluateServerSnapshotTransitionsPendingToActive(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	if len(stateStore.upserts) != 1 {
		t.Fatalf("expected 1 upsert after first evaluation, got %d", len(stateStore.upserts))
	}
	if got := stateStore.upserts[0].Status; got != domain.AlertStatusPending {
		t.Fatalf("expected pending status on first evaluation, got %q", got)
	}
	if !stateStore.upserts[0].TriggeredAt.Equal(now) {
		t.Fatalf("expected first triggered time to be initial sample, got %v", stateStore.upserts[0].TriggeredAt)
	}

	stateStore.states = []domain.AlertState{{
		ID:               11,
		RuleID:           1,
		ServerID:         server.ID,
		Metric:           "memory_usage",
		Operator:         "gte",
		Threshold:        80,
		CurrentValue:     90,
		Severity:         "warning",
		Message:          "内存使用率 90% 超过阈值 80%",
		Status:           domain.AlertStatusPending,
		DurationSeconds:  60,
		FirstTriggeredAt: now,
		LastTriggeredAt:  now,
	}}

	second := now.Add(61 * time.Second)
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, second); err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if len(stateStore.upserts) != 2 {
		t.Fatalf("expected 2 upserts after second evaluation, got %d", len(stateStore.upserts))
	}
	if got := stateStore.upserts[1].Status; got != domain.AlertStatusActive {
		t.Fatalf("expected active status after duration window, got %q", got)
	}
	if !stateStore.upserts[1].TriggeredAt.Equal(now) {
		t.Fatalf("expected first triggered time to stay unchanged, got %v", stateStore.upserts[1].TriggeredAt)
	}
	if !stateStore.upserts[1].LastTriggeredAt.Equal(second) {
		t.Fatalf("expected last triggered time to move forward, got %v", stateStore.upserts[1].LastTriggeredAt)
	}
}

func TestAlertService_EvaluateServerSnapshotResolvesRecoveredAlert(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusActive,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-2 * time.Minute),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil)

	recovered := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 50, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, recovered, now); err != nil {
		t.Fatalf("evaluate recovered snapshot: %v", err)
	}
	if len(stateStore.resolved) != 1 {
		t.Fatalf("expected one resolve call, got %d", len(stateStore.resolved))
	}
	if stateStore.resolved[0].ruleID != 1 || stateStore.resolved[0].serverID != server.ID {
		t.Fatalf("unexpected resolve target: %+v", stateStore.resolved[0])
	}
}

func TestAlertService_CreateRuleRejectsInvalidScope(t *testing.T) {
	store := &alertRuleStoreStub{}
	svc := service.NewAlertService(store, nil, nil, nil)

	err := svc.CreateRule(context.Background(), domain.AlertRule{
		Metric:          "cpu_usage",
		Operator:        "gte",
		Threshold:       90,
		DurationSeconds: 60,
		Enabled:         true,
		ScopeType:       domain.AlertRuleScopeServer,
		ScopeValue:      "not-a-server-id",
	})
	if !errors.Is(err, service.ErrInvalidAlertRuleScope) {
		t.Fatalf("expected invalid scope error, got %v", err)
	}
	if store.called {
		t.Fatal("expected invalid scoped rule not to be stored")
	}
}

func TestAlertService_CreateRuleRejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name string
		rule domain.AlertRule
	}{
		{
			name: "metric",
			rule: domain.AlertRule{Metric: "load1", Operator: "gte", Threshold: 90, Enabled: true},
		},
		{
			name: "operator",
			rule: domain.AlertRule{Metric: "cpu_usage", Operator: "between", Threshold: 90, Enabled: true},
		},
		{
			name: "percentage threshold",
			rule: domain.AlertRule{Metric: "memory_usage", Operator: "gte", Threshold: 101, Enabled: true},
		},
		{
			name: "online threshold",
			rule: domain.AlertRule{Metric: "online", Operator: "eq", Threshold: 2, Enabled: true},
		},
		{
			name: "duration",
			rule: domain.AlertRule{Metric: "disk_usage", Operator: "gte", Threshold: 90, DurationSeconds: -1, Enabled: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &alertRuleStoreStub{}
			svc := service.NewAlertService(store, nil, nil, nil)
			err := svc.CreateRule(context.Background(), tc.rule)
			if !errors.Is(err, service.ErrInvalidAlertRule) {
				t.Fatalf("expected ErrInvalidAlertRule, got %v", err)
			}
			if store.called {
				t.Fatal("expected invalid rule not to be stored")
			}
		})
	}
}

func TestAlertService_DeleteRuleMapsMissingRule(t *testing.T) {
	store := &alertRuleStoreStub{deleteErr: storage.ErrAlertRuleNotFound}
	svc := service.NewAlertService(store, nil, nil, nil)

	err := svc.DeleteRule(context.Background(), 99)
	if !errors.Is(err, service.ErrAlertRuleNotFound) {
		t.Fatalf("expected ErrAlertRuleNotFound, got %v", err)
	}
}

func TestAlertService_ListCurrentAlertsIncludesAcknowledgedAndMuted(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	svc := service.NewAlertService(
		&alertRuleStoreStub{},
		&alertStateStoreStub{states: []domain.AlertState{
			{ID: 1, RuleID: 1, ServerID: 7, Metric: "cpu_usage", Operator: "gte", Threshold: 80, CurrentValue: 90, Severity: "warning", Message: "cpu", Status: domain.AlertStatusActive, FirstTriggeredAt: now.Add(-3 * time.Minute), LastTriggeredAt: now},
			{ID: 2, RuleID: 2, ServerID: 7, Metric: "memory_usage", Operator: "gte", Threshold: 80, CurrentValue: 90, Severity: "warning", Message: "memory", Status: domain.AlertStatusAcknowledged, FirstTriggeredAt: now.Add(-2 * time.Minute), LastTriggeredAt: now.Add(-time.Minute)},
			{ID: 3, RuleID: 3, ServerID: 7, Metric: "disk_usage", Operator: "gte", Threshold: 80, CurrentValue: 90, Severity: "warning", Message: "disk", Status: domain.AlertStatusMuted, FirstTriggeredAt: now.Add(-time.Minute), LastTriggeredAt: now.Add(-2 * time.Minute)},
			{ID: 4, RuleID: 4, ServerID: 7, Metric: "disk_usage", Operator: "gte", Threshold: 80, CurrentValue: 90, Severity: "warning", Message: "pending", Status: domain.AlertStatusPending, FirstTriggeredAt: now, LastTriggeredAt: now},
		}},
		&alertServerStoreStub{servers: []domain.Server{{ID: 7, Name: "prod-web-01"}}},
		nil,
	)

	items, err := svc.ListCurrentAlerts(context.Background())
	if err != nil {
		t.Fatalf("list current alerts: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected active, acknowledged, and muted alerts, got %+v", items)
	}
	for _, item := range items {
		if item.Status == domain.AlertStatusPending {
			t.Fatalf("expected pending alert to be hidden, got %+v", items)
		}
		if item.ServerName != "prod-web-01" {
			t.Fatalf("expected server name mapping, got %+v", item)
		}
	}
}

func TestAlertService_EvaluateServerSnapshotFiltersRulesByScope(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	server := domain.Server{ID: 7, Name: "prod-web-01", Tags: []string{"prod"}, Purpose: "web"}
	stateStore := &alertStateStoreStub{}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{
			{ID: 1, Metric: "memory_usage", Operator: "gte", Threshold: 80, Enabled: true, ScopeType: domain.AlertRuleScopeTag, ScopeValue: "prod"},
			{ID: 2, Metric: "memory_usage", Operator: "gte", Threshold: 80, Enabled: true, ScopeType: domain.AlertRuleScopePurpose, ScopeValue: "database"},
			{ID: 3, Metric: "memory_usage", Operator: "gte", Threshold: 80, Enabled: true, ScopeType: domain.AlertRuleScopeServer, ScopeValue: "8"},
		},
	}, stateStore, &alertServerStoreStub{}, nil)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate scoped rules: %v", err)
	}
	if len(stateStore.upserts) != 1 {
		t.Fatalf("expected only matching scoped rule to upsert, got %d", len(stateStore.upserts))
	}
	if stateStore.upserts[0].RuleID != 1 {
		t.Fatalf("expected tag scoped rule to match, got %+v", stateStore.upserts[0])
	}
}

func TestAlertService_EvaluateServerSnapshotResolvesStateWhenScopeStopsMatching(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	server := domain.Server{ID: 7, Name: "prod-web-01", Tags: []string{"prod"}, Purpose: "web"}
	stateStore := &alertStateStoreStub{states: []domain.AlertState{{
		ID:               11,
		RuleID:           1,
		ServerID:         server.ID,
		Metric:           "memory_usage",
		Operator:         "gte",
		Threshold:        80,
		CurrentValue:     90,
		Severity:         "warning",
		Message:          "内存使用率 90% 超过阈值 80%",
		Status:           domain.AlertStatusActive,
		DurationSeconds:  60,
		FirstTriggeredAt: now.Add(-2 * time.Minute),
		LastTriggeredAt:  now.Add(-time.Minute),
	}}}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{ID: 1, Metric: "memory_usage", Operator: "gte", Threshold: 80, Enabled: true, ScopeType: domain.AlertRuleScopeTag, ScopeValue: "database"}},
	}, stateStore, &alertServerStoreStub{}, nil)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate non-matching scope: %v", err)
	}
	if len(stateStore.resolved) != 1 {
		t.Fatalf("expected stale scoped state to resolve, got %d", len(stateStore.resolved))
	}
	if stateStore.resolved[0].detail != "rule disabled or scope changed" {
		t.Fatalf("unexpected resolve detail: %+v", stateStore.resolved[0])
	}
}

func TestAlertService_SendTestNotificationUsesActor(t *testing.T) {
	recorder := &alertNotificationRecorder{}
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, nil, recorder)

	if err := svc.SendTestNotification(context.Background(), "admin"); err != nil {
		t.Fatalf("send test notification: %v", err)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("expected one test notification, got %d", len(recorder.items))
	}
	if recorder.items[0].EventType != domain.AlertEventTest || !strings.Contains(recorder.items[0].Alert.Message, "admin") {
		t.Fatalf("unexpected test notification: %+v", recorder.items[0])
	}
}

func TestAlertService_AcknowledgeReturnsNotFound(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{ackErr: sql.ErrNoRows}, &alertServerStoreStub{}, nil)
	_, err := svc.AcknowledgeAlert(context.Background(), 99, "alice")
	if !errors.Is(err, service.ErrAlertNotFound) {
		t.Fatalf("expected ErrAlertNotFound, got %v", err)
	}
}

func TestAlertService_AcknowledgeReturnsConflictForInvalidState(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{ackErr: storage.ErrAlertActionNotAllowed}, &alertServerStoreStub{}, nil)
	_, err := svc.AcknowledgeAlert(context.Background(), 99, "alice")
	if !errors.Is(err, service.ErrAlertActionNotAllowed) {
		t.Fatalf("expected ErrAlertActionNotAllowed, got %v", err)
	}
}

func TestAlertService_GetNotificationSettingsReturnsDefaultWithoutStore(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, nil)
	settings, err := svc.GetNotificationSettings(context.Background())
	if err != nil {
		t.Fatalf("get notification settings: %v", err)
	}
	if settings.Enabled {
		t.Fatalf("expected notifications disabled by default, got %+v", settings)
	}
	if settings.WebhookTimeoutSeconds != 5 {
		t.Fatalf("expected default timeout 5, got %+v", settings)
	}
}

func TestAlertService_SaveNotificationSettingsValidatesEnabledWebhook(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, &alertNotificationSettingsStoreStub{})
	_, err := svc.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{Enabled: true})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAlertService_SaveNotificationSettingsRejectsPrivateWebhookHost(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, &alertNotificationSettingsStoreStub{})
	_, err := svc.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:    true,
		WebhookURL: "http://127.0.0.1:8080/alerts",
	})
	if err == nil || !errors.Is(err, service.ErrInvalidAlertNotificationSettings) {
		t.Fatalf("expected invalid notification settings error, got %v", err)
	}
}

func TestAlertService_SaveNotificationSettingsNormalizesTimeout(t *testing.T) {
	settingsStore := &alertNotificationSettingsStoreStub{}
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, settingsStore)
	settings, err := svc.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:    true,
		WebhookURL: "https://hooks.example.test/alert",
	})
	if err != nil {
		t.Fatalf("save notification settings: %v", err)
	}
	if settings.WebhookTimeoutSeconds != 5 {
		t.Fatalf("expected default timeout 5, got %+v", settings)
	}
	if settingsStore.saved.WebhookURL != "https://hooks.example.test/alert" {
		t.Fatalf("expected trimmed webhook url to be saved, got %+v", settingsStore.saved)
	}
}

func TestAlertService_SaveNotificationSettingsRetainsExistingWebhookWhenBlank(t *testing.T) {
	settingsStore := &alertNotificationSettingsStoreStub{settings: domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            "https://hooks.example.test/existing",
		WebhookTimeoutSeconds: 5,
	}}
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, settingsStore)
	settings, err := svc.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookTimeoutSeconds: 9,
	})
	if err != nil {
		t.Fatalf("save notification settings: %v", err)
	}
	if settingsStore.saved.WebhookURL != "https://hooks.example.test/existing" {
		t.Fatalf("expected existing webhook url to be preserved, got %+v", settingsStore.saved)
	}
	if !settings.WebhookConfigured {
		t.Fatalf("expected webhookConfigured=true, got %+v", settings)
	}
}

func TestAlertService_SaveNotificationSettingsClearsWebhookWhenRequested(t *testing.T) {
	settingsStore := &alertNotificationSettingsStoreStub{settings: domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            "https://hooks.example.test/existing",
		WebhookTimeoutSeconds: 5,
	}}
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, settingsStore)
	settings, err := svc.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:               false,
		ClearWebhookURL:       true,
		WebhookTimeoutSeconds: 7,
	})
	if err != nil {
		t.Fatalf("save notification settings: %v", err)
	}
	if settingsStore.saved.WebhookURL != "" {
		t.Fatalf("expected webhook url to be cleared, got %+v", settingsStore.saved)
	}
	if settingsStore.saved.WebhookConfigured {
		t.Fatalf("expected webhookConfigured=false after clearing, got %+v", settingsStore.saved)
	}
	if settings.WebhookConfigured {
		t.Fatalf("expected response webhookConfigured=false, got %+v", settings)
	}
}

func TestAlertService_ListNotificationDeliveriesRejectsInvalidStatus(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{}, &alertServerStoreStub{}, &alertNotificationSettingsStoreStub{})
	_, err := svc.ListNotificationDeliveries(context.Background(), "bogus", 10)
	if !errors.Is(err, service.ErrInvalidNotificationDeliveryStatus) {
		t.Fatalf("expected ErrInvalidNotificationDeliveryStatus, got %v", err)
	}
}

func TestAlertService_EvaluateServerSnapshotKeepsAcknowledgedUntilRecovered(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	ackAt := now.Add(-30 * time.Second)
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusAcknowledged,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-2 * time.Minute),
			LastTriggeredAt:  now.Add(-time.Minute),
			AcknowledgedAt:   &ackAt,
			AcknowledgedBy:   "alice",
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate snapshot: %v", err)
	}
	if len(stateStore.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(stateStore.upserts))
	}
	if got := stateStore.upserts[0].Status; got != domain.AlertStatusAcknowledged {
		t.Fatalf("expected acknowledged status to be preserved, got %q", got)
	}
}

func TestAlertService_EvaluateServerSnapshotReactivatesExpiredMute(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	mutedUntil := now.Add(-time.Minute)
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusMuted,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-2 * time.Minute),
			LastTriggeredAt:  now.Add(-time.Minute),
			MutedUntil:       &mutedUntil,
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate snapshot: %v", err)
	}
	if len(stateStore.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(stateStore.upserts))
	}
	if got := stateStore.upserts[0].Status; got != domain.AlertStatusActive {
		t.Fatalf("expected mute expiry to reactivate alert, got %q", got)
	}
}

func TestAlertService_EvaluateServerSnapshotNotifiesTriggeredAlert(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	recorder := &alertNotificationRecorder{}
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusPending,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-61 * time.Second),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil, recorder)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate snapshot: %v", err)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(recorder.items))
	}
	if recorder.items[0].EventType != domain.AlertEventTriggered {
		t.Fatalf("unexpected notification: %+v", recorder.items[0])
	}
	if recorder.items[0].Alert.ServerName != server.Name || recorder.items[0].Alert.Status != domain.AlertStatusActive {
		t.Fatalf("unexpected alert payload: %+v", recorder.items[0].Alert)
	}
}

func TestAlertService_EvaluateServerSnapshotCreatesOutboxBeforeTriggeredDelivery(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	notifier := &transactionalAlertNotifierStub{deliverErr: errors.New("webhook unavailable")}
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusPending,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-61 * time.Second),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil, notifier)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate snapshot: %v", err)
	}
	if len(stateStore.upserts) != 1 || stateStore.createdDeliveries != 1 {
		t.Fatalf("expected one upsert and one transactional delivery, upserts=%d deliveries=%d", len(stateStore.upserts), stateStore.createdDeliveries)
	}
	if len(notifier.prepared) != 1 || len(notifier.delivered) != 1 {
		t.Fatalf("expected prepared and delivered notification, prepared=%d delivered=%d", len(notifier.prepared), len(notifier.delivered))
	}
	if notifier.delivered[0].ID == 0 || notifier.delivered[0].AlertID == 0 {
		t.Fatalf("expected persisted delivery with alert id, got %+v", notifier.delivered[0])
	}
}

func TestAlertService_EvaluateServerSnapshotSkipsOutboxWhenPreparedDeliveryDisabled(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	notifier := &transactionalAlertNotifierStub{skipPrepare: true}
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusPending,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-61 * time.Second),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{ID: 1, Metric: "memory_usage", Operator: "gte", Threshold: 80, DurationSeconds: 60, Enabled: true}},
	}, stateStore, &alertServerStoreStub{}, nil, notifier)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 90, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate snapshot: %v", err)
	}
	if stateStore.createdDeliveries != 0 || len(notifier.delivered) != 0 {
		t.Fatalf("expected disabled prepared delivery to skip outbox and delivery, deliveries=%d delivered=%d", stateStore.createdDeliveries, len(notifier.delivered))
	}
}

func TestAlertService_EvaluateServerSnapshotNotifiesResolvedAlert(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	recorder := &alertNotificationRecorder{}
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusActive,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-2 * time.Minute),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil, recorder)

	recovered := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 50, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, recovered, now); err != nil {
		t.Fatalf("evaluate recovered snapshot: %v", err)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(recorder.items))
	}
	if recorder.items[0].EventType != domain.AlertEventResolved {
		t.Fatalf("unexpected notification: %+v", recorder.items[0])
	}
	if recorder.items[0].Alert.Status != domain.AlertEventResolved {
		t.Fatalf("expected resolved alert status, got %+v", recorder.items[0].Alert)
	}
}

func TestAlertService_EvaluateServerSnapshotSkipsResolvedNotificationForPendingAlert(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	recorder := &alertNotificationRecorder{}
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusPending,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-30 * time.Second),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil, recorder)

	recovered := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 50, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, recovered, now); err != nil {
		t.Fatalf("evaluate recovered snapshot: %v", err)
	}
	if len(recorder.items) != 0 {
		t.Fatalf("expected no notification, got %d", len(recorder.items))
	}
}

func TestAlertService_EvaluateServerSnapshotSkipsResolvedNotificationForMutedAlert(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	mutedUntil := now.Add(10 * time.Minute)
	recorder := &alertNotificationRecorder{}
	server := domain.Server{ID: 7, Name: "prod-web-01"}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusMuted,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-2 * time.Minute),
			LastTriggeredAt:  now.Add(-time.Minute),
			MutedUntil:       &mutedUntil,
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil, recorder)

	recovered := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 50, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, recovered, now); err != nil {
		t.Fatalf("evaluate recovered snapshot: %v", err)
	}
	if len(recorder.items) != 0 {
		t.Fatalf("expected no notification, got %d", len(recorder.items))
	}
}

func TestAlertService_EvaluateServerSnapshotSuppressesAlertsDuringMaintenanceWindow(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	recorder := &alertNotificationRecorder{}
	server := domain.Server{
		ID:                 7,
		Name:               "prod-web-01",
		MaintenanceStartAt: &start,
		MaintenanceEndAt:   &end,
	}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{{
			ID:               11,
			RuleID:           1,
			ServerID:         server.ID,
			Metric:           "memory_usage",
			Operator:         "gte",
			Threshold:        80,
			CurrentValue:     90,
			Severity:         "warning",
			Message:          "内存使用率 90% 超过阈值 80%",
			Status:           domain.AlertStatusActive,
			DurationSeconds:  60,
			FirstTriggeredAt: now.Add(-2 * time.Minute),
			LastTriggeredAt:  now.Add(-time.Minute),
		}},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{
		rules: []domain.AlertRule{{
			ID:              1,
			Metric:          "memory_usage",
			Operator:        "gte",
			Threshold:       80,
			DurationSeconds: 60,
			Enabled:         true,
		}},
	}, stateStore, &alertServerStoreStub{}, nil, recorder)

	snapshot := collector.Snapshot{Online: true, SSHOK: true, MemoryUsage: 95, Source: "ssh"}
	if err := svc.EvaluateServerSnapshot(context.Background(), server, snapshot, now); err != nil {
		t.Fatalf("evaluate maintenance snapshot: %v", err)
	}
	if len(stateStore.upserts) != 0 {
		t.Fatalf("expected no upsert during maintenance, got %d", len(stateStore.upserts))
	}
	if len(stateStore.resolved) != 0 {
		t.Fatalf("expected no resolve during maintenance, got %d", len(stateStore.resolved))
	}
	if len(recorder.items) != 0 {
		t.Fatalf("expected no notification during maintenance, got %d", len(recorder.items))
	}
}

func TestAlertService_ResolveServerAlertsClearsOnlyTargetServer(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	recorder := &alertNotificationRecorder{}
	stateStore := &alertStateStoreStub{
		states: []domain.AlertState{
			{
				ID:               11,
				RuleID:           1,
				ServerID:         7,
				Metric:           "memory_usage",
				Operator:         "gte",
				Threshold:        80,
				CurrentValue:     90,
				Severity:         "warning",
				Message:          "memory high",
				Status:           domain.AlertStatusActive,
				FirstTriggeredAt: now.Add(-time.Minute),
				LastTriggeredAt:  now,
			},
			{
				ID:               12,
				RuleID:           2,
				ServerID:         8,
				Metric:           "memory_usage",
				Operator:         "gte",
				Threshold:        80,
				CurrentValue:     91,
				Severity:         "warning",
				Message:          "memory high",
				Status:           domain.AlertStatusActive,
				FirstTriggeredAt: now.Add(-time.Minute),
				LastTriggeredAt:  now,
			},
		},
	}
	svc := service.NewAlertService(&alertRuleStoreStub{}, stateStore, &alertServerStoreStub{}, nil, recorder)

	server := domain.Server{ID: 7, Name: "prod-web-01"}
	if err := svc.ResolveServerAlerts(context.Background(), server, "server disabled", now); err != nil {
		t.Fatalf("resolve server alerts: %v", err)
	}
	if len(stateStore.resolved) != 1 {
		t.Fatalf("expected one alert to resolve, got %d", len(stateStore.resolved))
	}
	if stateStore.resolved[0].serverID != server.ID || stateStore.resolved[0].detail != "server disabled" {
		t.Fatalf("unexpected resolve call: %+v", stateStore.resolved[0])
	}
	if len(recorder.items) != 1 || recorder.items[0].EventType != domain.AlertEventResolved {
		t.Fatalf("expected one resolved notification, got %+v", recorder.items)
	}
}

type alertRuleStoreStub struct {
	rules     []domain.AlertRule
	lastRule  domain.AlertRule
	called    bool
	deleteErr error
}

func (s *alertRuleStoreStub) List(context.Context) ([]domain.AlertRule, error) {
	return s.rules, nil
}

func (s *alertRuleStoreStub) Create(_ context.Context, rule domain.AlertRule) error {
	s.called = true
	s.lastRule = rule
	return nil
}

func (s *alertRuleStoreStub) Update(context.Context, domain.AlertRule) error {
	return nil
}

func (s *alertRuleStoreStub) Delete(context.Context, int64) error {
	return s.deleteErr
}

type alertServerStoreStub struct {
	servers []domain.Server
}

func (s *alertServerStoreStub) List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error) {
	if filter.ID == 0 {
		return s.servers, nil
	}
	for _, server := range s.servers {
		if server.ID == filter.ID {
			return []domain.Server{server}, nil
		}
	}
	return nil, nil
}

type alertResolveCall struct {
	ruleID   int64
	serverID int64
	detail   string
}

type alertNotificationRecorder struct {
	items []service.AlertNotification
}

func (r *alertNotificationRecorder) NotifyAlert(ctx context.Context, notification service.AlertNotification) error {
	r.items = append(r.items, notification)
	return nil
}

type transactionalAlertNotifierStub struct {
	prepared    []service.AlertNotification
	delivered   []domain.AlertNotificationDelivery
	deliverErr  error
	skipPrepare bool
}

func (n *transactionalAlertNotifierStub) NotifyAlert(ctx context.Context, notification service.AlertNotification) error {
	n.prepared = append(n.prepared, notification)
	return nil
}

func (n *transactionalAlertNotifierStub) PrepareDelivery(ctx context.Context, notification service.AlertNotification) (domain.AlertNotificationDelivery, string, bool, error) {
	n.prepared = append(n.prepared, notification)
	if n.skipPrepare {
		return domain.AlertNotificationDelivery{}, "", false, nil
	}
	return domain.AlertNotificationDelivery{
		EventType:  notification.EventType,
		AlertID:    notification.Alert.ID,
		RuleID:     notification.Alert.RuleID,
		ServerID:   notification.Alert.ServerID,
		ServerName: notification.Alert.ServerName,
		Status:     domain.AlertNotificationDeliveryPending,
		OccurredAt: notification.OccurredAt,
	}, fmt.Sprintf(`{"eventType":%q}`, notification.EventType), true, nil
}

func (n *transactionalAlertNotifierStub) DeliverPrepared(ctx context.Context, delivery domain.AlertNotificationDelivery, notification service.AlertNotification) error {
	n.delivered = append(n.delivered, delivery)
	return n.deliverErr
}

type alertNotificationSettingsStoreStub struct {
	settings   domain.AlertNotificationSettings
	saved      domain.AlertNotificationSettings
	err        error
	deliveries []domain.AlertNotificationDelivery
}

func (s *alertNotificationSettingsStoreStub) GetNotificationSettings(context.Context) (domain.AlertNotificationSettings, error) {
	if s.err != nil {
		return domain.AlertNotificationSettings{}, s.err
	}
	return s.settings, nil
}

func (s *alertNotificationSettingsStoreStub) SaveNotificationSettings(_ context.Context, settings domain.AlertNotificationSettings) (domain.AlertNotificationSettings, error) {
	if s.err != nil {
		return domain.AlertNotificationSettings{}, s.err
	}
	s.saved = settings
	s.settings = settings
	return settings, nil
}

func (s *alertNotificationSettingsStoreStub) ListNotificationDeliveries(context.Context, domain.AlertNotificationDeliveryFilter) ([]domain.AlertNotificationDelivery, error) {
	return s.deliveries, nil
}

type alertStateStoreStub struct {
	states            []domain.AlertState
	history           []domain.AlertHistoryEvent
	upserts           []storage.AlertEvaluationRecord
	resolved          []alertResolveCall
	createdDeliveries int
	ackState          domain.AlertState
	ackErr            error
	muteState         domain.AlertState
	muteErr           error
}

func (s *alertStateStoreStub) UpsertEvaluation(ctx context.Context, record storage.AlertEvaluationRecord) (domain.AlertState, bool, error) {
	s.upserts = append(s.upserts, record)
	state := domain.AlertState{
		ID:               int64(len(s.upserts)),
		RuleID:           record.RuleID,
		ServerID:         record.ServerID,
		Metric:           record.Metric,
		Operator:         record.Operator,
		Threshold:        record.Threshold,
		CurrentValue:     record.CurrentValue,
		Severity:         record.Severity,
		Message:          record.Message,
		Status:           record.Status,
		DurationSeconds:  record.DurationSeconds,
		FirstTriggeredAt: record.TriggeredAt,
		LastTriggeredAt:  record.LastTriggeredAt,
		AcknowledgedAt:   record.AcknowledgedAt,
		AcknowledgedBy:   record.AcknowledgedBy,
		MutedUntil:       record.MutedUntil,
	}
	if record.NotificationDeliveryBuilder != nil {
		delivery, _, ok, err := record.NotificationDeliveryBuilder(state)
		if err != nil {
			return domain.AlertState{}, false, err
		}
		if ok {
			delivery.ID = int64(s.createdDeliveries + 1)
			s.createdDeliveries++
			if record.NotificationDeliveryCreated != nil {
				record.NotificationDeliveryCreated(delivery)
			}
		}
	}
	return state, len(s.upserts) == 1, nil
}

func (s *alertStateStoreStub) ResolveByRuleAndServer(ctx context.Context, ruleID int64, serverID int64, detail string) (bool, error) {
	s.resolved = append(s.resolved, alertResolveCall{ruleID: ruleID, serverID: serverID, detail: detail})
	return true, nil
}

func (s *alertStateStoreStub) ListCurrentStates(ctx context.Context) ([]domain.AlertState, error) {
	return s.states, nil
}

func (s *alertStateStoreStub) ListHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	return s.history, nil
}

func (s *alertStateStoreStub) Acknowledge(ctx context.Context, alertID int64, username string) (domain.AlertState, error) {
	if s.ackErr != nil {
		return domain.AlertState{}, s.ackErr
	}
	return s.ackState, nil
}

func (s *alertStateStoreStub) Mute(ctx context.Context, alertID int64, username string, mutedUntil time.Time) (domain.AlertState, error) {
	if s.muteErr != nil {
		return domain.AlertState{}, s.muteErr
	}
	return s.muteState, nil
}
