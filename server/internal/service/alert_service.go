package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

type AlertRuleStore interface {
	List(ctx context.Context) ([]domain.AlertRule, error)
	Create(ctx context.Context, rule domain.AlertRule) error
	Update(ctx context.Context, rule domain.AlertRule) error
}

type AlertServerStore interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
}

type AlertStatusStore interface {
	ListLatest(ctx context.Context) ([]storage.LatestStatus, error)
}

type AlertService struct {
	rules    AlertRuleStore
	servers  AlertServerStore
	statuses AlertStatusStore
}

func NewAlertService(rules AlertRuleStore, servers AlertServerStore, statuses AlertStatusStore) *AlertService {
	return &AlertService{
		rules:    rules,
		servers:  servers,
		statuses: statuses,
	}
}

func EvaluateAlerts(rules []domain.AlertRule, snapshot collector.Snapshot, now time.Time) []domain.AlertEvent {
	events := make([]domain.AlertEvent, 0)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		currentValue := metricValue(rule.Metric, snapshot)
		if !compareValue(currentValue, rule.Operator, rule.Threshold) {
			continue
		}

		events = append(events, domain.AlertEvent{
			RuleID:          rule.ID,
			Metric:          rule.Metric,
			Operator:        rule.Operator,
			Threshold:       rule.Threshold,
			CurrentValue:    currentValue,
			Severity:        severityForMetric(rule.Metric),
			Message:         buildAlertMessage(rule.Metric, currentValue, rule.Threshold),
			TriggeredAt:     now,
			DurationSeconds: rule.DurationSeconds,
		})
	}
	return events
}

func (s *AlertService) ListRules(ctx context.Context) ([]domain.AlertRule, error) {
	return s.rules.List(ctx)
}

func (s *AlertService) CreateRule(ctx context.Context, rule domain.AlertRule) error {
	normalizeRule(&rule)
	return s.rules.Create(ctx, rule)
}

func (s *AlertService) UpdateRule(ctx context.Context, rule domain.AlertRule) error {
	normalizeRule(&rule)
	return s.rules.Update(ctx, rule)
}

func (s *AlertService) ListCurrentAlerts(ctx context.Context) ([]domain.AlertEvent, error) {
	rules, err := s.rules.List(ctx)
	if err != nil {
		return nil, err
	}
	servers, err := s.servers.List(ctx, storage.ServerFilter{})
	if err != nil {
		return nil, err
	}
	statuses, err := s.statuses.ListLatest(ctx)
	if err != nil {
		return nil, err
	}

	serverByID := make(map[int64]domain.Server, len(servers))
	for _, server := range servers {
		serverByID[server.ID] = server
	}

	events := make([]domain.AlertEvent, 0)
	for _, latest := range statuses {
		server := serverByID[latest.ServerID]
		current := EvaluateAlerts(rules, latest.Snapshot, latest.LastReportAt)
		for _, event := range current {
			event.ServerID = latest.ServerID
			event.ServerName = server.Name
			events = append(events, event)
		}
	}
	return events, nil
}

func normalizeRule(rule *domain.AlertRule) {
	rule.Metric = strings.TrimSpace(rule.Metric)
	rule.Operator = strings.TrimSpace(rule.Operator)
}

func metricValue(metric string, snapshot collector.Snapshot) float64 {
	switch metric {
	case "online":
		if snapshot.Online {
			return 1
		}
		return 0
	case "cpu_usage":
		return snapshot.CPUUsage
	case "memory_usage":
		return snapshot.MemoryUsage
	case "disk_usage":
		return snapshot.DiskUsage
	default:
		return 0
	}
}

func compareValue(current float64, operator string, threshold float64) bool {
	switch operator {
	case "eq":
		return current == threshold
	case "gt":
		return current > threshold
	case "gte":
		return current >= threshold
	case "lt":
		return current < threshold
	case "lte":
		return current <= threshold
	default:
		return false
	}
}

func severityForMetric(metric string) string {
	if metric == "online" {
		return "critical"
	}
	return "warning"
}

func buildAlertMessage(metric string, current float64, threshold float64) string {
	switch metric {
	case "online":
		return "服务器离线"
	case "cpu_usage":
		return fmt.Sprintf("CPU 使用率 %.0f%% 超过阈值 %.0f%%", current, threshold)
	case "memory_usage":
		return fmt.Sprintf("内存使用率 %.0f%% 超过阈值 %.0f%%", current, threshold)
	case "disk_usage":
		return fmt.Sprintf("磁盘使用率 %.0f%% 超过阈值 %.0f%%", current, threshold)
	default:
		return fmt.Sprintf("%s 告警：当前值 %.2f，阈值 %.2f", metric, current, threshold)
	}
}
