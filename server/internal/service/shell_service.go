package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

type ShellAlertReader interface {
	ListCurrentAlerts(ctx context.Context) ([]domain.AlertEvent, error)
}

type ShellCommandReader interface {
	ListRecent(ctx context.Context, limit int, keyword string) ([]storage.CommandLogListItem, error)
}

type ShellAuthEventReader interface {
	ListRecent(ctx context.Context, limit int, keyword string, eventTypes ...string) ([]domain.AuthEvent, error)
}

type NotificationItem struct {
	Kind      string    `json:"kind"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

type ActivityItem struct {
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	CreatedAt  time.Time `json:"createdAt"`
	ServerID   int64     `json:"serverId,omitempty"`
	ServerName string    `json:"serverName,omitempty"`
	Username   string    `json:"username,omitempty"`
}

type SearchResults struct {
	Alerts     []NotificationItem `json:"alerts"`
	Commands   []ActivityItem     `json:"commands"`
	AuthEvents []ActivityItem     `json:"authEvents"`
}

type ShellService struct {
	alerts     ShellAlertReader
	commands   ShellCommandReader
	authEvents ShellAuthEventReader
}

func NewShellService(alerts ShellAlertReader, commands ShellCommandReader, authEvents ShellAuthEventReader) *ShellService {
	return &ShellService{
		alerts:     alerts,
		commands:   commands,
		authEvents: authEvents,
	}
}

func (s *ShellService) ListNotifications(ctx context.Context, limit int) ([]NotificationItem, error) {
	limit = normalizeShellLimit(limit, 20)
	items := make([]NotificationItem, 0, limit)

	if s.authEvents != nil {
		events, err := s.authEvents.ListRecent(ctx, limit*2, "", domain.AuthEventLoginFailed)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			items = append(items, NotificationItem{
				Kind:      "auth",
				Severity:  "warning",
				Title:     "登录失败",
				Message:   buildAuthEventMessage(event),
				CreatedAt: event.CreatedAt,
			})
		}
	}

	sort.Slice(items, func(i int, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *ShellService) ListActivity(ctx context.Context, limit int) ([]ActivityItem, error) {
	limit = normalizeShellLimit(limit, 20)
	items := make([]ActivityItem, 0, limit*2)

	if s.commands != nil {
		commands, err := s.commands.ListRecent(ctx, limit, "")
		if err != nil {
			return nil, err
		}
		for _, item := range commands {
			items = append(items, ActivityItem{
				Kind:       "command",
				Title:      commandActivityTitle(item),
				Summary:    buildCommandSummary(item),
				CreatedAt:  item.ExecutedAt,
				ServerID:   item.ServerID,
				ServerName: item.ServerName,
			})
		}
	}

	if s.authEvents != nil {
		events, err := s.authEvents.ListRecent(
			ctx,
			limit,
			"",
			domain.AuthEventBootstrapAdminCreated,
			domain.AuthEventLoginFailed,
			domain.AuthEventPasswordChanged,
		)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			items = append(items, ActivityItem{
				Kind:      "auth",
				Title:     authEventTitle(event.EventType),
				Summary:   buildAuthEventMessage(event),
				CreatedAt: event.CreatedAt,
				Username:  event.Username,
			})
		}
	}

	sort.Slice(items, func(i int, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *ShellService) Search(ctx context.Context, query string, limit int) (SearchResults, error) {
	limit = normalizeShellLimit(limit, 10)
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResults{}, nil
	}

	results := SearchResults{
		Alerts:     make([]NotificationItem, 0, limit),
		Commands:   make([]ActivityItem, 0, limit),
		AuthEvents: make([]ActivityItem, 0, limit),
	}

	if s.alerts != nil {
		alerts, err := s.alerts.ListCurrentAlerts(ctx)
		if err != nil {
			return SearchResults{}, err
		}
		for _, alert := range alerts {
			if !matchesQuery(query, alert.ServerName, alert.Message, alert.Metric) {
				continue
			}
			results.Alerts = append(results.Alerts, NotificationItem{
				Kind:      "alert",
				Severity:  alert.Severity,
				Title:     alert.ServerName,
				Message:   alert.Message,
				CreatedAt: alert.TriggeredAt,
			})
			if len(results.Alerts) >= limit {
				break
			}
		}
	}

	if s.commands != nil {
		commands, err := s.commands.ListRecent(ctx, limit, query)
		if err != nil {
			return SearchResults{}, err
		}
		for _, item := range commands {
			results.Commands = append(results.Commands, ActivityItem{
				Kind:       "command",
				Title:      commandActivityTitle(item),
				Summary:    buildCommandSummary(item),
				CreatedAt:  item.ExecutedAt,
				ServerID:   item.ServerID,
				ServerName: item.ServerName,
			})
		}
	}

	if s.authEvents != nil {
		events, err := s.authEvents.ListRecent(ctx, limit, query)
		if err != nil {
			return SearchResults{}, err
		}
		for _, event := range events {
			results.AuthEvents = append(results.AuthEvents, ActivityItem{
				Kind:      "auth",
				Title:     authEventTitle(event.EventType),
				Summary:   buildAuthEventMessage(event),
				CreatedAt: event.CreatedAt,
				Username:  event.Username,
			})
		}
	}

	return results, nil
}

func normalizeShellLimit(limit int, fallback int) int {
	if limit <= 0 {
		limit = fallback
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func commandActivityTitle(item storage.CommandLogListItem) string {
	if item.ExitCode == 0 {
		return "执行命令"
	}
	return "命令执行失败"
}

func buildCommandSummary(item storage.CommandLogListItem) string {
	serverName := strings.TrimSpace(item.ServerName)
	if serverName == "" {
		serverName = fmt.Sprintf("#%d", item.ServerID)
	}
	return serverName + " · " + item.Command
}

func buildAuthEventMessage(event domain.AuthEvent) string {
	if strings.TrimSpace(event.Detail) != "" {
		return event.Username + " · " + event.Detail
	}
	return event.Username
}

func authEventTitle(eventType string) string {
	switch eventType {
	case domain.AuthEventBootstrapAdminCreated:
		return "创建初始管理员"
	case domain.AuthEventLoginFailed:
		return "登录失败"
	case domain.AuthEventLoginSucceeded:
		return "登录成功"
	case domain.AuthEventLogout:
		return "退出登录"
	case domain.AuthEventPasswordChanged:
		return "修改密码"
	default:
		return eventType
	}
}

func matchesQuery(query string, values ...string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}
