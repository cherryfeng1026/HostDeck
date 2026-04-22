package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

var (
	ErrAlertNotFound                  = errors.New("告警不存在")
	ErrAlertActionNotAllowed          = errors.New("当前告警状态不支持该操作")
	ErrInvalidAlertNotificationSettings = errors.New("告警通知设置无效")
)

type AlertRuleStore interface {
	List(ctx context.Context) ([]domain.AlertRule, error)
	Create(ctx context.Context, rule domain.AlertRule) error
	Update(ctx context.Context, rule domain.AlertRule) error
}

type AlertStateStore interface {
	UpsertEvaluation(ctx context.Context, record storage.AlertEvaluationRecord) (domain.AlertState, bool, error)
	ResolveByRuleAndServer(ctx context.Context, ruleID int64, serverID int64, detail string) (bool, error)
	ListCurrentStates(ctx context.Context) ([]domain.AlertState, error)
	ListHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error)
	Acknowledge(ctx context.Context, alertID int64, username string) (domain.AlertState, error)
	Mute(ctx context.Context, alertID int64, username string, mutedUntil time.Time) (domain.AlertState, error)
}

type alertHistoryTypeReader interface {
	ListHistoryByTypes(ctx context.Context, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error)
}

type alertHistorySearchReader interface {
	SearchHistory(ctx context.Context, query string, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error)
}

type AlertServerStore interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
}

type AlertNotificationSettingsStore interface {
	GetNotificationSettings(ctx context.Context) (domain.AlertNotificationSettings, error)
	SaveNotificationSettings(ctx context.Context, settings domain.AlertNotificationSettings) (domain.AlertNotificationSettings, error)
}

type AlertService struct {
	rules    AlertRuleStore
	states   AlertStateStore
	servers  AlertServerStore
	settings AlertNotificationSettingsStore
	notifier AlertNotifier
}

func NewAlertService(rules AlertRuleStore, states AlertStateStore, servers AlertServerStore, settings AlertNotificationSettingsStore, notifiers ...AlertNotifier) *AlertService {
	service := &AlertService{
		rules:    rules,
		states:   states,
		servers:  servers,
		settings: settings,
	}
	if len(notifiers) > 0 {
		service.notifier = notifiers[0]
	}
	return service
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
			Status:          domain.AlertStatusActive,
			TriggeredAt:     now,
			LastTriggeredAt: now,
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

func (s *AlertService) GetNotificationSettings(ctx context.Context) (domain.AlertNotificationSettings, error) {
	if s.settings == nil {
		return domain.AlertNotificationSettings{WebhookTimeoutSeconds: 5}, nil
	}
	return s.settings.GetNotificationSettings(ctx)
}

func (s *AlertService) SaveNotificationSettings(ctx context.Context, settings domain.AlertNotificationSettings) (domain.AlertNotificationSettings, error) {
	settings.WebhookURL = strings.TrimSpace(settings.WebhookURL)
	if settings.WebhookTimeoutSeconds <= 0 {
		settings.WebhookTimeoutSeconds = 5
	}
	if s.settings == nil {
		return domain.AlertNotificationSettings{}, errors.New("通知设置存储未配置")
	}
	current, err := s.settings.GetNotificationSettings(ctx)
	if err != nil {
		return domain.AlertNotificationSettings{}, err
	}
	if settings.ClearWebhookURL {
		settings.WebhookURL = ""
		settings.WebhookConfigured = false
	} else if settings.WebhookURL == "" && strings.TrimSpace(current.WebhookURL) != "" {
		settings.WebhookURL = strings.TrimSpace(current.WebhookURL)
		settings.WebhookConfigured = true
	}
	if settings.Enabled && settings.WebhookURL == "" {
		return domain.AlertNotificationSettings{}, fmt.Errorf("%w: 启用通知时必须填写 Webhook 地址", ErrInvalidAlertNotificationSettings)
	}
	if settings.WebhookURL != "" {
		if _, err := NewWebhookAlertNotifier(settings.WebhookURL, time.Duration(settings.WebhookTimeoutSeconds)*time.Second); err != nil {
			return domain.AlertNotificationSettings{}, fmt.Errorf("%w: %s", ErrInvalidAlertNotificationSettings, err.Error())
		}
		settings.WebhookConfigured = true
	}
	return s.settings.SaveNotificationSettings(ctx, settings)
}

func (s *AlertService) EvaluateServerSnapshot(ctx context.Context, server domain.Server, snapshot collector.Snapshot, sampledAt time.Time) error {
	rules, err := s.rules.List(ctx)
	if err != nil {
		return err
	}
	current := EvaluateAlerts(rules, snapshot, sampledAt)
	activeByRule := make(map[int64]domain.AlertEvent, len(current))
	for _, event := range current {
		activeByRule[event.RuleID] = event
	}

	states, err := s.states.ListCurrentStates(ctx)
	if err != nil {
		return err
	}
	stateByRule := make(map[int64]domain.AlertState)
	for _, state := range states {
		if state.ServerID == server.ID {
			stateByRule[state.RuleID] = state
		}
	}

	for _, rule := range rules {
		event, matched := activeByRule[rule.ID]
		state, hasState := stateByRule[rule.ID]
		if server.InMaintenanceWindow(sampledAt) {
			continue
		}
		if matched {
			firstTriggeredAt := sampledAt
			status := domain.AlertStatusPending
			acknowledgedAt := (*time.Time)(nil)
			acknowledgedBy := ""
			mutedUntil := (*time.Time)(nil)
			if hasState {
				firstTriggeredAt = state.FirstTriggeredAt
				acknowledgedAt = state.AcknowledgedAt
				acknowledgedBy = state.AcknowledgedBy
				mutedUntil = state.MutedUntil
				status = state.Status
			}
			if rule.DurationSeconds <= 0 || sampledAt.Sub(firstTriggeredAt) >= time.Duration(rule.DurationSeconds)*time.Second {
				status = domain.AlertStatusActive
				if mutedUntil != nil && mutedUntil.After(sampledAt) {
					status = domain.AlertStatusMuted
				}
				if acknowledgedAt != nil && status == domain.AlertStatusActive {
					status = domain.AlertStatusAcknowledged
				}
			}
			nextState, _, err := s.states.UpsertEvaluation(ctx, storage.AlertEvaluationRecord{
				RuleID:          rule.ID,
				ServerID:        server.ID,
				Metric:          event.Metric,
				Operator:        event.Operator,
				Threshold:       event.Threshold,
				CurrentValue:    event.CurrentValue,
				Severity:        event.Severity,
				Message:         event.Message,
				DurationSeconds: event.DurationSeconds,
				TriggeredAt:     firstTriggeredAt,
				LastTriggeredAt: sampledAt,
				Status:          status,
				AcknowledgedAt:  acknowledgedAt,
				AcknowledgedBy:  acknowledgedBy,
				MutedUntil:      mutedUntil,
			})
			if err != nil {
				return err
			}
			s.notifyTriggered(ctx, server, state, hasState, nextState, sampledAt)
			continue
		}

		if !hasState {
			continue
		}
		if _, err := s.states.ResolveByRuleAndServer(ctx, rule.ID, server.ID, "metric recovered"); err != nil {
			return err
		}
		s.notifyResolved(ctx, server, state, sampledAt)
	}
	return nil
}

func (s *AlertService) notifyTriggered(ctx context.Context, server domain.Server, previous domain.AlertState, hasPrevious bool, current domain.AlertState, occurredAt time.Time) {
	if s.notifier == nil {
		return
	}
	if current.Status != domain.AlertStatusActive {
		return
	}
	if hasPrevious && previous.Status != domain.AlertStatusPending {
		return
	}
	if err := s.notifier.NotifyAlert(ctx, AlertNotification{
		EventType:  domain.AlertEventTriggered,
		Alert:      s.alertEventFromState(server.Name, current),
		OccurredAt: occurredAt.UTC(),
	}); err != nil {
		slog.Warn("notify triggered alert failed", "error", err, "ruleId", current.RuleID, "serverId", current.ServerID)
	}
}

func (s *AlertService) notifyResolved(ctx context.Context, server domain.Server, state domain.AlertState, occurredAt time.Time) {
	if s.notifier == nil {
		return
	}
	if state.Status != domain.AlertStatusActive && state.Status != domain.AlertStatusAcknowledged {
		return
	}
	resolved := s.alertEventFromState(server.Name, state)
	resolved.Status = domain.AlertEventResolved
	resolved.LastTriggeredAt = occurredAt.UTC()
	if err := s.notifier.NotifyAlert(ctx, AlertNotification{
		EventType:  domain.AlertEventResolved,
		Alert:      resolved,
		OccurredAt: occurredAt.UTC(),
	}); err != nil {
		slog.Warn("notify resolved alert failed", "error", err, "ruleId", state.RuleID, "serverId", state.ServerID)
	}
}

func (s *AlertService) alertEventFromState(serverName string, state domain.AlertState) domain.AlertEvent {
	return domain.AlertEvent{
		ID:              state.ID,
		RuleID:          state.RuleID,
		ServerID:        state.ServerID,
		ServerName:      serverName,
		Metric:          state.Metric,
		Operator:        state.Operator,
		Threshold:       state.Threshold,
		CurrentValue:    state.CurrentValue,
		Severity:        state.Severity,
		Message:         state.Message,
		Status:          state.Status,
		TriggeredAt:     state.FirstTriggeredAt,
		LastTriggeredAt: state.LastTriggeredAt,
		AcknowledgedAt:  state.AcknowledgedAt,
		AcknowledgedBy:  state.AcknowledgedBy,
		MutedUntil:      state.MutedUntil,
		DurationSeconds: state.DurationSeconds,
	}
}

func (s *AlertService) ListCurrentAlerts(ctx context.Context) ([]domain.AlertEvent, error) {
	states, err := s.states.ListCurrentStates(ctx)
	if err != nil {
		return nil, err
	}
	servers, err := s.servers.List(ctx, storage.ServerFilter{})
	if err != nil {
		return nil, err
	}
	serverByID := make(map[int64]domain.Server, len(servers))
	for _, server := range servers {
		serverByID[server.ID] = server
	}
	items := make([]domain.AlertEvent, 0, len(states))
	for _, state := range states {
		if state.Status == domain.AlertStatusPending {
			continue
		}
		server := serverByID[state.ServerID]
		items = append(items, domain.AlertEvent{
			ID:              state.ID,
			RuleID:          state.RuleID,
			ServerID:        state.ServerID,
			ServerName:      server.Name,
			Metric:          state.Metric,
			Operator:        state.Operator,
			Threshold:       state.Threshold,
			CurrentValue:    state.CurrentValue,
			Severity:        state.Severity,
			Message:         state.Message,
			Status:          state.Status,
			TriggeredAt:     state.FirstTriggeredAt,
			LastTriggeredAt: state.LastTriggeredAt,
			AcknowledgedAt:  state.AcknowledgedAt,
			AcknowledgedBy:  state.AcknowledgedBy,
			MutedUntil:      state.MutedUntil,
			DurationSeconds: state.DurationSeconds,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LastTriggeredAt.After(items[j].LastTriggeredAt)
	})
	return items, nil
}

func (s *AlertService) ListAlertHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	return s.states.ListHistory(ctx, limit)
}

func (s *AlertService) ListAlertHistoryByTypes(ctx context.Context, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error) {
	if reader, ok := s.states.(alertHistoryTypeReader); ok {
		return reader.ListHistoryByTypes(ctx, limit, eventTypes...)
	}
	history, err := s.states.ListHistory(ctx, limit)
	if err != nil {
		return nil, err
	}
	return filterAlertHistoryByTypes(history, eventTypes...), nil
}

func (s *AlertService) SearchAlertHistory(ctx context.Context, query string, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error) {
	if reader, ok := s.states.(alertHistorySearchReader); ok {
		return reader.SearchHistory(ctx, query, limit, eventTypes...)
	}
	history, err := s.states.ListHistory(ctx, limit)
	if err != nil {
		return nil, err
	}
	history = filterAlertHistoryByTypes(history, eventTypes...)
	filtered := make([]domain.AlertHistoryEvent, 0, len(history))
	for _, item := range history {
		if matchesAlertHistoryQuery(query, item) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *AlertService) AcknowledgeAlert(ctx context.Context, alertID int64, username string) (domain.AlertEvent, error) {
	state, err := s.states.Acknowledge(ctx, alertID, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AlertEvent{}, ErrAlertNotFound
		}
		if errors.Is(err, storage.ErrAlertActionNotAllowed) {
			return domain.AlertEvent{}, ErrAlertActionNotAllowed
		}
		return domain.AlertEvent{}, err
	}
	return s.stateToEvent(ctx, state)
}

func (s *AlertService) MuteAlert(ctx context.Context, alertID int64, username string, mutedUntil time.Time) (domain.AlertEvent, error) {
	state, err := s.states.Mute(ctx, alertID, username, mutedUntil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AlertEvent{}, ErrAlertNotFound
		}
		if errors.Is(err, storage.ErrAlertActionNotAllowed) {
			return domain.AlertEvent{}, ErrAlertActionNotAllowed
		}
		return domain.AlertEvent{}, err
	}
	return s.stateToEvent(ctx, state)
}

func (s *AlertService) stateToEvent(ctx context.Context, state domain.AlertState) (domain.AlertEvent, error) {
	servers, err := s.servers.List(ctx, storage.ServerFilter{ID: state.ServerID})
	if err != nil {
		return domain.AlertEvent{}, err
	}
	serverName := ""
	if len(servers) > 0 {
		serverName = servers[0].Name
	}
	return domain.AlertEvent{
		ID:              state.ID,
		RuleID:          state.RuleID,
		ServerID:        state.ServerID,
		ServerName:      serverName,
		Metric:          state.Metric,
		Operator:        state.Operator,
		Threshold:       state.Threshold,
		CurrentValue:    state.CurrentValue,
		Severity:        state.Severity,
		Message:         state.Message,
		Status:          state.Status,
		TriggeredAt:     state.FirstTriggeredAt,
		LastTriggeredAt: state.LastTriggeredAt,
		AcknowledgedAt:  state.AcknowledgedAt,
		AcknowledgedBy:  state.AcknowledgedBy,
		MutedUntil:      state.MutedUntil,
		DurationSeconds: state.DurationSeconds,
	}, nil
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

func filterAlertHistoryByTypes(items []domain.AlertHistoryEvent, eventTypes ...string) []domain.AlertHistoryEvent {
	if len(eventTypes) == 0 {
		return items
	}
	allowed := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		allowed[strings.TrimSpace(eventType)] = struct{}{}
	}
	filtered := make([]domain.AlertHistoryEvent, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EventType]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func matchesAlertHistoryQuery(query string, item domain.AlertHistoryEvent) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	values := []string{item.ServerName, item.Message, item.Metric, item.Detail, item.ActorUsername}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
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
