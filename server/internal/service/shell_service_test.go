package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

type shellAlertReaderStub struct {
	current              []domain.AlertEvent
	history              []domain.AlertHistoryEvent
	listHistoryCallLimit *[]int
}

func (s shellAlertReaderStub) ListCurrentAlerts(ctx context.Context) ([]domain.AlertEvent, error) {
	return s.current, nil
}

func (s shellAlertReaderStub) ListAlertHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	if s.listHistoryCallLimit != nil {
		*s.listHistoryCallLimit = append(*s.listHistoryCallLimit, limit)
	}
	if limit > 0 && len(s.history) > limit {
		return s.history[:limit], nil
	}
	return s.history, nil
}

type shellCommandReaderStub struct {
	items []storage.CommandLogListItem
}

func (s shellCommandReaderStub) ListRecent(ctx context.Context, limit int, keyword string) ([]storage.CommandLogListItem, error) {
	return s.items, nil
}

type shellAuthReaderStub struct {
	items []domain.AuthEvent
}

func (s shellAuthReaderStub) ListRecent(ctx context.Context, limit int, keyword string, eventTypes ...string) ([]domain.AuthEvent, error) {
	allowed := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		allowed[eventType] = struct{}{}
	}
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	items := make([]domain.AuthEvent, 0, len(s.items))
	for _, item := range s.items {
		if len(allowed) > 0 {
			if _, ok := allowed[item.EventType]; !ok {
				continue
			}
		}
		if keyword != "" &&
			!strings.Contains(strings.ToLower(item.Username), keyword) &&
			!strings.Contains(strings.ToLower(item.EventType), keyword) &&
			!strings.Contains(strings.ToLower(item.Detail), keyword) {
			continue
		}
		items = append(items, item)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

type shellAuditReaderStub struct {
	items []domain.AuditEvent
}

func (s shellAuditReaderStub) ListRecent(ctx context.Context, limit int, keyword string, kinds ...string) ([]domain.AuditEvent, error) {
	return s.items, nil
}

type shellNotificationStateStub struct {
	readAt    *time.Time
	updatedAt *time.Time
}

func (s shellNotificationStateStub) GetNotificationReadAt(ctx context.Context, userID int64) (*time.Time, error) {
	return s.readAt, nil
}

func (s shellNotificationStateStub) UpdateNotificationReadAt(ctx context.Context, userID int64, readAt time.Time) error {
	if s.updatedAt != nil {
		*s.updatedAt = readAt
	}
	return nil
}

func TestShellService_ListActivityMergesUnifiedEvents(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	svc := service.NewShellService(
		shellAlertReaderStub{history: []domain.AlertHistoryEvent{{
			EventType:     domain.AlertEventTriggered,
			Severity:      "warning",
			Message:       "内存使用率 90% 超过阈值 80%",
			ServerID:      1,
			ServerName:    "prod-web-01",
			CreatedAt:     now.Add(-2 * time.Minute),
			ActorUsername: "ops",
		}}},
		shellCommandReaderStub{items: []storage.CommandLogListItem{{
			ServerID:   1,
			ServerName: "prod-web-01",
			Command:    "df -h",
			ExitCode:   0,
			ExecutedAt: now.Add(-1 * time.Minute),
		}}},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			Username:  "admin",
			EventType: domain.AuthEventLoginFailed,
			Detail:    "bad password",
			CreatedAt: now.Add(-3 * time.Minute),
		}}},
		shellAuditReaderStub{items: []domain.AuditEvent{{
			Kind:       domain.AuditKindServer,
			Severity:   "info",
			Title:      "更新服务器",
			Summary:    "prod-web-01 · 10.0.0.21",
			ServerID:   1,
			ServerName: "prod-web-01",
			Username:   "admin",
			CreatedAt:  now,
		}}},
		shellNotificationStateStub{},
	)

	items, err := svc.ListActivity(context.Background(), 10, true, true)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 unified items, got %d", len(items))
	}
	if items[0].Kind != domain.AuditKindServer || items[0].Title != "更新服务器" {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].Kind != "command" || items[1].Title != "执行命令" || items[1].RoutePath != "/commands" {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
	if items[2].Kind != "alert" || items[2].Title != "告警触发" || items[2].RoutePath != "/alerts" {
		t.Fatalf("unexpected third item: %+v", items[2])
	}
	if items[3].Kind != "auth" || items[3].Title != "登录失败" || items[3].RoutePath != "/users" {
		t.Fatalf("unexpected fourth item: %+v", items[3])
	}
}

func TestShellService_ListNotificationsReturnsUnreadState(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	readAt := now.Add(-90 * time.Second)
	svc := service.NewShellService(
		shellAlertReaderStub{history: []domain.AlertHistoryEvent{{
			EventType:  domain.AlertEventTriggered,
			Severity:   "warning",
			Message:    "磁盘使用率过高",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now.Add(-2 * time.Minute),
		}, {
			EventType:  domain.AlertEventResolved,
			Severity:   "info",
			Message:    "磁盘使用率恢复",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now,
		}}},
		shellCommandReaderStub{},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			Username:  "admin",
			EventType: domain.AuthEventLoginFailed,
			CreatedAt: now.Add(-time.Minute),
		}}},
		shellAuditReaderStub{},
		shellNotificationStateStub{readAt: &readAt},
	)

	result, err := svc.ListNotifications(context.Background(), 1, 10, true)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if result.UnreadCount != 2 {
		t.Fatalf("expected 2 unread items, got %d", result.UnreadCount)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 notification items, got %d", len(result.Items))
	}
	if !result.Items[2].IsRead {
		t.Fatalf("expected oldest notification to be read, got %+v", result.Items[2])
	}
	if result.Items[0].IsRead || result.Items[1].IsRead {
		t.Fatalf("expected newest notifications unread, got %+v", result.Items)
	}
}

func TestShellService_SearchReturnsDeduplicatedUnifiedResults(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	svc := service.NewShellService(
		shellAlertReaderStub{
			current: []domain.AlertEvent{{
				ID:          42,
				ServerID:    1,
				ServerName:  "prod-web-01",
				Severity:    "warning",
				Message:     "磁盘使用率过高",
				Metric:      "disk_usage",
				TriggeredAt: now.Add(-5 * time.Minute),
			}},
			history: []domain.AlertHistoryEvent{
				{
					AlertID:     42,
					EventType:   domain.AlertEventTriggered,
					Severity:    "warning",
					Message:     "磁盘使用率过高",
					Metric:      "disk_usage",
					ServerID:    1,
					ServerName:  "prod-web-01",
					TriggeredAt: now.Add(-5 * time.Minute),
					CreatedAt:   now,
				},
				{
					AlertID:     42,
					EventType:   domain.AlertEventResolved,
					Severity:    "info",
					Message:     "磁盘使用率恢复",
					Metric:      "disk_usage",
					ServerID:    1,
					ServerName:  "prod-web-01",
					TriggeredAt: now.Add(-5 * time.Minute),
					CreatedAt:   now.Add(-1 * time.Minute),
				},
			},
		},
		shellCommandReaderStub{items: []storage.CommandLogListItem{{
			ID:         7,
			ServerID:   1,
			ServerName: "prod-web-01",
			Command:    "df -h",
			ExitCode:   0,
			ExecutedAt: now.Add(-2 * time.Minute),
		}}},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			ID:        9,
			Username:  "prod-operator",
			EventType: domain.AuthEventLoginSucceeded,
			CreatedAt: now.Add(-3 * time.Minute),
		}}},
		shellAuditReaderStub{items: []domain.AuditEvent{{
			ID:         11,
			Kind:       domain.AuditKindServer,
			Severity:   "info",
			Title:      "更新服务器",
			Summary:    "prod-web-01 · 修改标签",
			ServerID:   1,
			ServerName: "prod-web-01",
			Username:   "admin",
			CreatedAt:  now.Add(-4 * time.Minute),
		}}},
		shellNotificationStateStub{},
	)

	results, err := svc.Search(context.Background(), "prod", 10, true, true)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results.Items) != 5 {
		t.Fatalf("expected 5 unified search results with triggered alert deduplicated, got %d %+v", len(results.Items), results.Items)
	}
	if results.Items[0].Kind != "alert" || results.Items[0].RoutePath != "/alerts" || results.Items[0].Title != "告警恢复" {
		t.Fatalf("expected latest alert history result first, got %+v", results.Items[0])
	}
	triggeredCount := 0
	for _, item := range results.Items {
		if item.Kind == "alert" && item.Title == "告警触发" {
			triggeredCount++
		}
	}
	if triggeredCount != 1 {
		t.Fatalf("expected triggered alert to be deduplicated across current and history, got %+v", results.Items)
	}
	hasAudit := false
	hasAuth := false
	for _, item := range results.Items {
		if item.Kind == domain.AuditKindServer && item.RoutePath == "/servers/1" {
			hasAudit = true
		}
		if item.Kind == "auth" && item.Title == "登录成功" {
			hasAuth = true
		}
	}
	if !hasAudit || !hasAuth {
		t.Fatalf("expected audit and auth results included, got %+v", results.Items)
	}
}

func TestShellService_HidesSensitiveAndAuthEventsWithoutPermissions(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	svc := service.NewShellService(
		shellAlertReaderStub{history: []domain.AlertHistoryEvent{{
			EventType:  domain.AlertEventTriggered,
			Severity:   "warning",
			Message:    "磁盘使用率过高",
			ServerID:   1,
			ServerName: "prod-web-01",
			CreatedAt:  now,
		}}},
		shellCommandReaderStub{items: []storage.CommandLogListItem{{
			ServerID:   1,
			ServerName: "prod-web-01",
			Command:    "df -h",
			ExitCode:   0,
			ExecutedAt: now.Add(-time.Minute),
		}}},
		shellAuthReaderStub{items: []domain.AuthEvent{{
			ID:        12,
			Username:  "viewer-user",
			EventType: domain.AuthEventLoginFailed,
			Detail:    "bad password",
			CreatedAt: now.Add(-45 * time.Second),
		}}},
		shellAuditReaderStub{items: []domain.AuditEvent{{
			Kind:       domain.AuditKindServer,
			Severity:   "info",
			Title:      "更新服务器",
			Summary:    "prod-web-01 · 10.0.0.21",
			ServerID:   1,
			ServerName: "prod-web-01",
			Username:   "admin",
			CreatedAt:  now.Add(-30 * time.Second),
		}}},
		shellNotificationStateStub{},
	)

	activity, err := svc.ListActivity(context.Background(), 10, false, false)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(activity) != 1 {
		t.Fatalf("expected only non-sensitive alert activity items, got %+v", activity)
	}
	hasVisibleAlert := false
	for _, item := range activity {
		if item.Kind == "auth" {
			t.Fatalf("expected auth activity to be hidden without user management access, got %+v", activity)
		}
		if item.Kind == "alert" && item.Title == "告警触发" && item.RoutePath == "/alerts" {
			hasVisibleAlert = true
		}
	}
	if !hasVisibleAlert {
		t.Fatalf("expected alert items to remain visible without sensitive access, got %+v", activity)
	}

	results, err := svc.Search(context.Background(), "df", 10, false, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, item := range results.Items {
		if item.Kind == "command" {
			t.Fatalf("expected command results to be hidden without sensitive access, got %+v", results.Items)
		}
	}

	results, err = svc.Search(context.Background(), "prod-web-01", 10, false, false)
	if err != nil {
		t.Fatalf("search audit redaction: %v", err)
	}
	hasVisibleAlert = false
	for _, item := range results.Items {
		if item.Kind == "alert" && item.Title == "告警触发" && item.Summary != "" && item.RoutePath == "/alerts" {
			hasVisibleAlert = true
		}
		if item.Kind == domain.AuditKindServer || item.Kind == domain.AuditKindCommand || item.Kind == domain.AuditKindAlertRule {
			t.Fatalf("expected sensitive audit results to be hidden without sensitive access, got %+v", results.Items)
		}
	}
	if !hasVisibleAlert {
		t.Fatalf("expected non-sensitive alert search result to remain visible, got %+v", results.Items)
	}

	results, err = svc.Search(context.Background(), "viewer-user", 10, false, false)
	if err != nil {
		t.Fatalf("search auth visibility: %v", err)
	}
	for _, item := range results.Items {
		if item.Kind == "auth" {
			t.Fatalf("expected auth results to be hidden without user management access, got %+v", results.Items)
		}
		if item.Kind == "command" || item.Kind == domain.AuditKindServer || item.Kind == domain.AuditKindCommand || item.Kind == domain.AuditKindAlertRule {
			t.Fatalf("expected only non-sensitive alert results, got %+v", results.Items)
		}
	}
	if len(results.Items) != 0 {
		t.Fatalf("expected auth-only query to return no results without user management access, got %+v", results.Items)
	}
}

func TestShellService_ListNotificationsUsesExpandedHistoryWindowAndCountsAllUnread(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	limits := make([]int, 0, 1)
	readAt := now.Add(-10 * time.Minute)
	history := []domain.AlertHistoryEvent{
		{EventType: domain.AlertEventAcknowledged, Severity: "info", Message: "ack-1", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now},
		{EventType: domain.AlertEventMuted, Severity: "info", Message: "mute-1", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-1 * time.Minute)},
		{EventType: domain.AlertEventAcknowledged, Severity: "info", Message: "ack-2", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-2 * time.Minute)},
		{EventType: domain.AlertEventMuted, Severity: "info", Message: "mute-2", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-3 * time.Minute)},
		{EventType: domain.AlertEventTriggered, Severity: "warning", Message: "trigger-1", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-4 * time.Minute)},
		{EventType: domain.AlertEventResolved, Severity: "info", Message: "resolved-1", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-5 * time.Minute)},
		{EventType: domain.AlertEventTriggered, Severity: "warning", Message: "trigger-2", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-6 * time.Minute)},
	}
	svc := service.NewShellService(
		shellAlertReaderStub{history: history, listHistoryCallLimit: &limits},
		shellCommandReaderStub{},
		shellAuthReaderStub{},
		shellAuditReaderStub{},
		shellNotificationStateStub{readAt: &readAt},
	)

	result, err := svc.ListNotifications(context.Background(), 1, 2, false)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(limits) != 1 || limits[0] <= 2 {
		t.Fatalf("expected expanded history fetch limit, got %+v", limits)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected paged notification items, got %d", len(result.Items))
	}
	if result.UnreadCount != 3 {
		t.Fatalf("expected unread count across full filtered set, got %d", result.UnreadCount)
	}
	if result.Items[0].Summary == "" || result.Items[1].Summary == "" {
		t.Fatalf("expected filtered notification items, got %+v", result.Items)
	}
}

func TestShellService_SearchUsesExpandedHistoryWindow(t *testing.T) {
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	limits := make([]int, 0, 1)
	history := []domain.AlertHistoryEvent{
		{EventType: domain.AlertEventAcknowledged, Severity: "info", Message: "ack-1", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now},
		{EventType: domain.AlertEventMuted, Severity: "info", Message: "mute-1", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-1 * time.Minute)},
		{EventType: domain.AlertEventAcknowledged, Severity: "info", Message: "ack-2", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-2 * time.Minute)},
		{EventType: domain.AlertEventResolved, Severity: "info", Message: "target-match", ServerID: 1, ServerName: "prod-web-01", CreatedAt: now.Add(-3 * time.Minute)},
	}
	svc := service.NewShellService(
		shellAlertReaderStub{history: history, listHistoryCallLimit: &limits},
		shellCommandReaderStub{},
		shellAuthReaderStub{},
		shellAuditReaderStub{},
		shellNotificationStateStub{},
	)

	results, err := svc.Search(context.Background(), "target-match", 2, false, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(limits) != 1 || limits[0] <= 2 {
		t.Fatalf("expected expanded history fetch limit, got %+v", limits)
	}
	if len(results.Items) != 1 || results.Items[0].Summary == "" {
		t.Fatalf("expected matching historical alert beyond page limit, got %+v", results.Items)
	}
}
