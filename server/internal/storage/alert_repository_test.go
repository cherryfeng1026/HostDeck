package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

func TestAlertRepository_AcknowledgeRejectsNonActiveState(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)
	seedAlertStateForRepositoryTest(t, repo, domain.AlertStatusAcknowledged)

	_, err := repo.Acknowledge(context.Background(), 1, "alice")
	if !errors.Is(err, storage.ErrAlertActionNotAllowed) {
		t.Fatalf("expected ErrAlertActionNotAllowed, got %v", err)
	}
}

func TestAlertRepository_MuteAllowsAcknowledgedState(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)
	seedAlertStateForRepositoryTest(t, repo, domain.AlertStatusAcknowledged)

	mutedUntil := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	state, err := repo.Mute(context.Background(), 1, "alice", mutedUntil)
	if err != nil {
		t.Fatalf("mute alert: %v", err)
	}
	if state.Status != domain.AlertStatusMuted {
		t.Fatalf("expected muted status, got %+v", state)
	}
	if state.MutedUntil == nil || !state.MutedUntil.Equal(mutedUntil) {
		t.Fatalf("expected mutedUntil to be persisted, got %+v", state)
	}
}

func TestAlertRepository_MuteRejectsPendingState(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)
	seedAlertStateForRepositoryTest(t, repo, domain.AlertStatusPending)

	_, err := repo.Mute(context.Background(), 1, "alice", time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if !errors.Is(err, storage.ErrAlertActionNotAllowed) {
		t.Fatalf("expected ErrAlertActionNotAllowed, got %v", err)
	}
}

func TestAlertRepository_ResolveSkipsHistoryForPendingState(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)
	seedAlertStateForRepositoryTest(t, repo, domain.AlertStatusPending)

	if err := repo.Resolve(context.Background(), 1, "metric recovered"); err != nil {
		t.Fatalf("resolve pending alert: %v", err)
	}

	history, err := repo.ListHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected no history for pending resolve, got %d", len(history))
	}
}

func TestAlertRepository_NotificationSettingsDefaultsWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)

	settings, err := repo.GetNotificationSettings(context.Background())
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

func TestAlertRepository_SaveNotificationSettingsPersistsSingleRow(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db, "test-master-key")

	saved, err := repo.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            " https://hooks.example.test/alerts ",
		WebhookTimeoutSeconds: 9,
	})
	if err != nil {
		t.Fatalf("save notification settings: %v", err)
	}
	if saved.WebhookURL != "https://hooks.example.test/alerts" {
		t.Fatalf("expected trimmed webhook url, got %+v", saved)
	}

	loaded, err := repo.GetNotificationSettings(context.Background())
	if err != nil {
		t.Fatalf("reload notification settings: %v", err)
	}
	if !loaded.Enabled || loaded.WebhookTimeoutSeconds != 9 || loaded.WebhookURL != "https://hooks.example.test/alerts" || !loaded.WebhookConfigured {
		t.Fatalf("unexpected notification settings: %+v", loaded)
	}
	var rawWebhookURL string
	if err := db.QueryRowContext(context.Background(), `SELECT webhook_url FROM alert_notification_settings LIMIT 1`).Scan(&rawWebhookURL); err != nil {
		t.Fatalf("load raw notification settings row: %v", err)
	}
	if rawWebhookURL == "https://hooks.example.test/alerts" || rawWebhookURL == " https://hooks.example.test/alerts " || strings.Contains(rawWebhookURL, "hooks.example.test") {
		t.Fatalf("expected webhook url to be stored encrypted, got %q", rawWebhookURL)
	}
}

func TestAlertRepository_SaveNotificationSettingsUpsertsSingletonRow(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db, "test-master-key")

	if _, err := repo.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            "https://hooks.example.test/alerts-a",
		WebhookTimeoutSeconds: 5,
	}); err != nil {
		t.Fatalf("save initial notification settings: %v", err)
	}
	if _, err := repo.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:               false,
		WebhookURL:            "https://hooks.example.test/alerts-b",
		WebhookTimeoutSeconds: 7,
	}); err != nil {
		t.Fatalf("update notification settings: %v", err)
	}

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM alert_notification_settings`).Scan(&count); err != nil {
		t.Fatalf("count notification settings rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected singleton row, got %d", count)
	}

	loaded, err := repo.GetNotificationSettings(context.Background())
	if err != nil {
		t.Fatalf("reload notification settings: %v", err)
	}
	if loaded.Enabled {
		t.Fatalf("expected notifications disabled after update, got %+v", loaded)
	}
	if loaded.WebhookURL != "https://hooks.example.test/alerts-b" || loaded.WebhookTimeoutSeconds != 7 {
		t.Fatalf("unexpected updated notification settings: %+v", loaded)
	}
}

func TestAlertRepository_SaveNotificationSettingsRejectsMissingMasterKey(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)

	_, err := repo.SaveNotificationSettings(context.Background(), domain.AlertNotificationSettings{
		Enabled:               true,
		WebhookURL:            "https://hooks.example.test/alerts",
		WebhookTimeoutSeconds: 5,
	})
	if err == nil {
		t.Fatal("expected save notification settings to fail without master key")
	}
	if !strings.Contains(err.Error(), "master_key 未配置") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAlertRepository_ListHistoryByTypesFiltersAtQueryLevel(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)
	seedAlertHistoryForRepositoryQueryTest(t, db)

	items, err := repo.ListHistoryByTypes(context.Background(), 2, domain.AlertEventTriggered, domain.AlertEventResolved)
	if err != nil {
		t.Fatalf("list history by types: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 filtered history items, got %d %+v", len(items), items)
	}
	if items[0].EventType != domain.AlertEventResolved || items[0].Message != "disk alert recovered" {
		t.Fatalf("expected latest resolved event first, got %+v", items[0])
	}
	if items[1].EventType != domain.AlertEventTriggered || items[1].Message != "disk alert triggered" {
		t.Fatalf("expected triggered event second, got %+v", items[1])
	}
}

func TestAlertRepository_SearchHistoryFiltersBeforeLimit(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewAlertRepository(db)
	seedAlertHistoryForRepositoryQueryTest(t, db)

	items, err := repo.SearchHistory(context.Background(), "target-match", 1, domain.AlertEventResolved)
	if err != nil {
		t.Fatalf("search history: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 search result, got %d %+v", len(items), items)
	}
	if items[0].Message != "target-match recovery" || items[0].EventType != domain.AlertEventResolved {
		t.Fatalf("unexpected search result: %+v", items[0])
	}
	if items[0].ServerName != "prod-web-01" {
		t.Fatalf("expected joined server name, got %+v", items[0])
	}
}

func seedAlertHistoryForRepositoryQueryTest(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	if _, err := db.ExecContext(ctx, `INSERT INTO servers (id, name, hostname, ip, ssh_port, username, auth_type, credential_ref, collector_mode, tags, purpose, remark, enabled, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		1, "prod-web-01", "prod-web-01", "10.0.0.21", 22, "root", "password", "", "ssh_only", "[]", "", "", 1, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	entries := []struct {
		alertID     int64
		eventType   string
		message     string
		detail      string
		createdAt   time.Time
		actor       string
	}{
		{alertID: 1, eventType: domain.AlertEventAcknowledged, message: "ack noise", detail: "ignore me", createdAt: now},
		{alertID: 2, eventType: domain.AlertEventMuted, message: "mute noise", detail: "ignore me too", createdAt: now.Add(-1 * time.Minute)},
		{alertID: 3, eventType: domain.AlertEventResolved, message: "disk alert recovered", detail: "operator confirmed", createdAt: now.Add(-2 * time.Minute), actor: "alice"},
		{alertID: 4, eventType: domain.AlertEventTriggered, message: "disk alert triggered", detail: "threshold exceeded", createdAt: now.Add(-3 * time.Minute), actor: "bob"},
		{alertID: 5, eventType: domain.AlertEventResolved, message: "target-match recovery", detail: "target-match detail", createdAt: now.Add(-4 * time.Minute), actor: "carol"},
	}
	for _, entry := range entries {
		if _, err := db.ExecContext(ctx, `INSERT INTO alert_history (alert_id, rule_id, server_id, event_type, metric, operator, threshold, current_value, severity, message, status, triggered_at, created_at, actor_username, detail) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			entry.alertID, 1, 1, entry.eventType, "disk_usage", "gte", 80, 90, "warning", entry.message, domain.AlertStatusActive, entry.createdAt.Add(-30*time.Second).Format(time.RFC3339Nano), entry.createdAt.Format(time.RFC3339Nano), entry.actor, entry.detail); err != nil {
			t.Fatalf("seed alert history: %v", err)
		}
	}
}

func seedAlertStateForRepositoryTest(t *testing.T, repo *storage.AlertRepository, status string) {
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

	record := storage.AlertEvaluationRecord{
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
		Status:          status,
	}
	if status == domain.AlertStatusAcknowledged {
		ackAt := now.Add(-time.Minute)
		record.AcknowledgedAt = &ackAt
		record.AcknowledgedBy = "alice"
	}
	if status == domain.AlertStatusMuted {
		mutedUntil := now.Add(30 * time.Minute)
		record.MutedUntil = &mutedUntil
	}

	if _, _, err := repo.UpsertEvaluation(context.Background(), record); err != nil {
		t.Fatalf("seed alert state: %v", err)
	}
}
