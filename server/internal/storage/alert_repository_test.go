package storage_test

import (
	"context"
	"errors"
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
