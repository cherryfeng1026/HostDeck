package service_test

import (
	"context"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
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

type alertRuleStoreStub struct {
	lastRule domain.AlertRule
	called   bool
}

func (s *alertRuleStoreStub) List(context.Context) ([]domain.AlertRule, error) {
	return nil, nil
}

func (s *alertRuleStoreStub) Create(_ context.Context, rule domain.AlertRule) error {
	s.called = true
	s.lastRule = rule
	return nil
}

func (s *alertRuleStoreStub) Update(context.Context, domain.AlertRule) error {
	return nil
}
