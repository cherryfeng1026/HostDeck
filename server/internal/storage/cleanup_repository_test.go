package storage_test

import (
	"context"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

func TestCleanupRepositoriesDeleteExpiredHistoryAndEvents(t *testing.T) {
	db := testsupport.OpenPostgresTestDB(t)
	ctx := context.Background()

	statusRepo := storage.NewStatusRepository(db)
	commandRepo := storage.NewCommandLogRepository(db)
	alertRepo := storage.NewAlertRepository(db)
	authRepo := storage.NewAuthEventRepository(db)
	auditRepo := storage.NewAuditEventRepository(db)

	oldTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	if _, err := db.ExecContext(ctx, `INSERT INTO servers (name, hostname, ip, ssh_port, username, auth_type, credential_ref, collector_mode, tags, purpose, remark, enabled, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		"prod-web-01", "prod-web-01", "10.0.0.21", 22, "root", "password", "", "ssh_only", "[]", "", "", 1, oldTime.Format(time.RFC3339Nano), oldTime.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	snapshot := collector.Snapshot{CPUUsage: 20, MemoryUsage: 30, DiskUsage: 40, Load1: 0.1, Load5: 0.2, Load15: 0.3}
	if err := statusRepo.AppendHistory(ctx, 1, snapshot, oldTime); err != nil {
		t.Fatalf("append old status history: %v", err)
	}
	if err := statusRepo.AppendHistory(ctx, 1, snapshot, newTime); err != nil {
		t.Fatalf("append new status history: %v", err)
	}

	if err := commandRepo.Create(ctx, domain.CommandLog{ServerID: 1, ExecutorUsername: "alice", Command: "df -h", Stdout: "ok", ExitCode: 0, DurationMS: 10, ExecutedAt: oldTime}); err != nil {
		t.Fatalf("create old command log: %v", err)
	}
	if err := commandRepo.Create(ctx, domain.CommandLog{ServerID: 1, ExecutorUsername: "alice", Command: "free -m", Stdout: "ok", ExitCode: 0, DurationMS: 10, ExecutedAt: newTime}); err != nil {
		t.Fatalf("create new command log: %v", err)
	}

	if err := authRepo.Create(ctx, domain.AuthEvent{Username: "alice", EventType: domain.AuthEventLoginFailed, CreatedAt: oldTime}); err != nil {
		t.Fatalf("create old auth event: %v", err)
	}
	if err := authRepo.Create(ctx, domain.AuthEvent{Username: "alice", EventType: domain.AuthEventLoginSucceeded, CreatedAt: newTime}); err != nil {
		t.Fatalf("create new auth event: %v", err)
	}

	if err := auditRepo.Create(ctx, domain.AuditEvent{Kind: domain.AuditKindServer, Severity: "info", Title: "old event", CreatedAt: oldTime}); err != nil {
		t.Fatalf("create old audit event: %v", err)
	}
	if err := auditRepo.Create(ctx, domain.AuditEvent{Kind: domain.AuditKindServer, Severity: "info", Title: "new event", CreatedAt: newTime}); err != nil {
		t.Fatalf("create new audit event: %v", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO alert_history (alert_id, rule_id, server_id, event_type, metric, operator, threshold, current_value, severity, message, status, triggered_at, created_at, actor_username, detail) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		1, 1, 1, domain.AlertEventResolved, "memory_usage", "gte", 80, 75, "warning", "old alert", domain.AlertStatusActive, oldTime.Format(time.RFC3339Nano), oldTime.Format(time.RFC3339Nano), "alice", ""); err != nil {
		t.Fatalf("seed old alert history: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO alert_history (alert_id, rule_id, server_id, event_type, metric, operator, threshold, current_value, severity, message, status, triggered_at, created_at, actor_username, detail) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		2, 1, 1, domain.AlertEventTriggered, "memory_usage", "gte", 80, 90, "warning", "new alert", domain.AlertStatusActive, newTime.Format(time.RFC3339Nano), newTime.Format(time.RFC3339Nano), "alice", ""); err != nil {
		t.Fatalf("seed new alert history: %v", err)
	}

	if err := statusRepo.DeleteHistoryBefore(ctx, cutoff); err != nil {
		t.Fatalf("cleanup status history: %v", err)
	}
	if err := commandRepo.DeleteBefore(ctx, cutoff); err != nil {
		t.Fatalf("cleanup command logs: %v", err)
	}
	if err := alertRepo.DeleteHistoryBefore(ctx, cutoff); err != nil {
		t.Fatalf("cleanup alert history: %v", err)
	}
	if err := authRepo.DeleteBefore(ctx, cutoff); err != nil {
		t.Fatalf("cleanup auth events: %v", err)
	}
	if err := auditRepo.DeleteBefore(ctx, cutoff); err != nil {
		t.Fatalf("cleanup audit events: %v", err)
	}

	points, err := statusRepo.ListHistory(ctx, 1, oldTime.Add(-time.Hour))
	if err != nil {
		t.Fatalf("list status history: %v", err)
	}
	if len(points) != 1 || !points[0].SampledAt.Equal(newTime) {
		t.Fatalf("unexpected remaining status history: %+v", points)
	}

	commandItems, err := commandRepo.ListHistory(ctx, domain.CommandHistoryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list command logs: %v", err)
	}
	if len(commandItems) != 1 || !commandItems[0].ExecutedAt.Equal(newTime) {
		t.Fatalf("unexpected remaining command logs: %+v", commandItems)
	}

	alertItems, err := alertRepo.ListHistory(ctx, 10)
	if err != nil {
		t.Fatalf("list alert history: %v", err)
	}
	if len(alertItems) != 1 || !alertItems[0].CreatedAt.Equal(newTime) {
		t.Fatalf("unexpected remaining alert history: %+v", alertItems)
	}

	authItems, err := authRepo.ListRecent(ctx, 10, "")
	if err != nil {
		t.Fatalf("list auth events: %v", err)
	}
	if len(authItems) != 1 || !authItems[0].CreatedAt.Equal(newTime) {
		t.Fatalf("unexpected remaining auth events: %+v", authItems)
	}

	auditItems, err := auditRepo.ListRecent(ctx, 10, "")
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(auditItems) != 1 || !auditItems[0].CreatedAt.Equal(newTime) {
		t.Fatalf("unexpected remaining audit events: %+v", auditItems)
	}
}
