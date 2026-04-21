package service_test

import (
	"context"
	"database/sql"
	"errors"
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
	svc := service.NewAlertService(store, nil, nil)

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
	}, stateStore, &alertServerStoreStub{})

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
	}, stateStore, &alertServerStoreStub{})

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

func TestAlertService_AcknowledgeReturnsNotFound(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{ackErr: sql.ErrNoRows}, &alertServerStoreStub{})
	_, err := svc.AcknowledgeAlert(context.Background(), 99, "alice")
	if !errors.Is(err, service.ErrAlertNotFound) {
		t.Fatalf("expected ErrAlertNotFound, got %v", err)
	}
}

func TestAlertService_AcknowledgeReturnsConflictForInvalidState(t *testing.T) {
	svc := service.NewAlertService(&alertRuleStoreStub{}, &alertStateStoreStub{ackErr: storage.ErrAlertActionNotAllowed}, &alertServerStoreStub{})
	_, err := svc.AcknowledgeAlert(context.Background(), 99, "alice")
	if !errors.Is(err, service.ErrAlertActionNotAllowed) {
		t.Fatalf("expected ErrAlertActionNotAllowed, got %v", err)
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
	}, stateStore, &alertServerStoreStub{})

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
	}, stateStore, &alertServerStoreStub{})

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
	}, stateStore, &alertServerStoreStub{}, recorder)

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
	}, stateStore, &alertServerStoreStub{}, recorder)

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
	}, stateStore, &alertServerStoreStub{}, recorder)

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
	}, stateStore, &alertServerStoreStub{}, recorder)

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
	}, stateStore, &alertServerStoreStub{}, recorder)

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

type alertRuleStoreStub struct {
	rules    []domain.AlertRule
	lastRule domain.AlertRule
	called   bool
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

type alertStateStoreStub struct {
	states    []domain.AlertState
	history   []domain.AlertHistoryEvent
	upserts   []storage.AlertEvaluationRecord
	resolved  []alertResolveCall
	ackState  domain.AlertState
	ackErr    error
	muteState domain.AlertState
	muteErr   error
}

func (s *alertStateStoreStub) UpsertEvaluation(ctx context.Context, record storage.AlertEvaluationRecord) (domain.AlertState, bool, error) {
	s.upserts = append(s.upserts, record)
	state := domain.AlertState{
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
