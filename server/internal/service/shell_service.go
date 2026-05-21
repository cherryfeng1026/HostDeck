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
	ListAlertHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error)
}

type shellAlertHistoryTypeReader interface {
	ListAlertHistoryByTypes(ctx context.Context, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error)
}

type shellAlertHistorySearchReader interface {
	SearchAlertHistory(ctx context.Context, query string, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error)
}

type ShellCommandReader interface {
	ListRecent(ctx context.Context, limit int, keyword string) ([]storage.CommandLogListItem, error)
}

type ShellAuthEventReader interface {
	ListRecent(ctx context.Context, limit int, keyword string, eventTypes ...string) ([]domain.AuthEvent, error)
}

type ShellAuditEventReader interface {
	ListRecent(ctx context.Context, limit int, keyword string, kinds ...string) ([]domain.AuditEvent, error)
}

type ShellNotificationStateStore interface {
	GetNotificationReadAt(ctx context.Context, userID int64) (*time.Time, error)
	UpdateNotificationReadAt(ctx context.Context, userID int64, readAt time.Time) error
}

type ShellEventItem struct {
	Kind       string    `json:"kind"`
	Severity   string    `json:"severity"`
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	CreatedAt  time.Time `json:"createdAt"`
	ServerID   int64     `json:"serverId,omitempty"`
	ServerName string    `json:"serverName,omitempty"`
	Username   string    `json:"username,omitempty"`
	RoutePath  string    `json:"routePath,omitempty"`
	IsRead     bool      `json:"isRead"`
}

type NotificationList struct {
	Items       []ShellEventItem `json:"items"`
	UnreadCount int              `json:"unreadCount"`
}

type ShellEventList struct {
	Items []ShellEventItem `json:"items"`
}

type SearchResults struct {
	Items []ShellEventItem `json:"items"`
}

const (
	shellHistoryFetchFactor = 4
	shellHistoryMinFetch    = 50
	shellHistoryMaxFetch    = 200
)

type ShellService struct {
	alerts            ShellAlertReader
	commands          ShellCommandReader
	authEvents        ShellAuthEventReader
	audit             ShellAuditEventReader
	notificationState ShellNotificationStateStore
}

func NewShellService(alerts ShellAlertReader, commands ShellCommandReader, authEvents ShellAuthEventReader, audit ShellAuditEventReader, notificationState ShellNotificationStateStore) *ShellService {
	return &ShellService{
		alerts:            alerts,
		commands:          commands,
		authEvents:        authEvents,
		audit:             audit,
		notificationState: notificationState,
	}
}

func (s *ShellService) ListNotifications(ctx context.Context, userID int64, limit int, includeAuth bool) (NotificationList, error) {
	limit = normalizeShellLimit(limit, 20)
	fetchLimit := shellHistoryFetchLimit(limit)
	items := make([]ShellEventItem, 0, fetchLimit)

	if s.alerts != nil {
		history, err := s.listNotificationAlertHistory(ctx, fetchLimit)
		if err != nil {
			return NotificationList{}, err
		}
		for _, item := range history {
			items = append(items, shellEventFromAlertHistory(item))
		}
	}

	if includeAuth && s.authEvents != nil {
		events, err := s.authEvents.ListRecent(ctx, fetchLimit, "", domain.AuthEventLoginFailed)
		if err != nil {
			return NotificationList{}, err
		}
		for _, event := range events {
			items = append(items, shellEventFromAuthEvent(event))
		}
	}

	sortShellEvents(items)
	readAt, err := s.notificationReadAt(ctx, userID)
	if err != nil {
		return NotificationList{}, err
	}
	unreadCount := markNotificationReadState(items, readAt)
	if len(items) > limit {
		items = items[:limit]
	}
	return NotificationList{Items: items, UnreadCount: unreadCount}, nil
}

func (s *ShellService) ListActivity(ctx context.Context, limit int, includeSensitive bool, includeAuth bool) ([]ShellEventItem, error) {
	limit = normalizeShellLimit(limit, 20)
	items := make([]ShellEventItem, 0, limit*4)

	if includeSensitive && s.commands != nil {
		commands, err := s.commands.ListRecent(ctx, limit, "")
		if err != nil {
			return nil, err
		}
		for _, item := range commands {
			items = append(items, shellEventFromCommand(item))
		}
	}

	if includeAuth && s.authEvents != nil {
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
			items = append(items, shellEventFromAuthEvent(event))
		}
	}

	if s.alerts != nil {
		history, err := s.alerts.ListAlertHistory(ctx, limit)
		if err != nil {
			return nil, err
		}
		for _, item := range history {
			items = append(items, shellEventFromAlertHistory(item))
		}
	}

	if includeSensitive && s.audit != nil {
		events, err := s.audit.ListRecent(ctx, limit, "")
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			items = append(items, shellEventFromAudit(event))
		}
	}

	sortShellEvents(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *ShellService) MarkNotificationsRead(ctx context.Context, userID int64, readBefore time.Time) error {
	if userID <= 0 || s.notificationState == nil {
		return nil
	}
	if readBefore.IsZero() {
		return nil
	}
	return s.notificationState.UpdateNotificationReadAt(ctx, userID, readBefore.UTC())
}

func (s *ShellService) Search(ctx context.Context, query string, limit int, includeSensitive bool, includeAuth bool) (SearchResults, error) {
	limit = normalizeShellLimit(limit, 10)
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResults{}, nil
	}

	items := make([]ShellEventItem, 0, limit*4)
	seen := map[string]struct{}{}

	appendUnique := func(key string, item ShellEventItem) {
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, item)
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
			appendUnique(searchKeyForCurrentAlert(alert), shellEventFromCurrentAlert(alert))
		}

		history, err := s.searchAlertHistory(ctx, query, shellHistoryFetchLimit(limit))
		if err != nil {
			return SearchResults{}, err
		}
		for _, item := range history {
			if !matchesQuery(query, item.ServerName, item.Message, item.Metric, item.Detail, item.ActorUsername) {
				continue
			}
			appendUnique(searchKeyForAlertHistory(item), shellEventFromAlertHistory(item))
		}
	}

	if includeSensitive && s.commands != nil {
		commands, err := s.commands.ListRecent(ctx, limit, query)
		if err != nil {
			return SearchResults{}, err
		}
		for _, item := range commands {
			appendUnique(searchKeyForCommand(item), shellEventFromCommand(item))
		}
	}

	if includeAuth && s.authEvents != nil {
		events, err := s.authEvents.ListRecent(ctx, limit, query)
		if err != nil {
			return SearchResults{}, err
		}
		for _, event := range events {
			appendUnique(searchKeyForAuthEvent(event), shellEventFromAuthEvent(event))
		}
	}

	if includeSensitive && s.audit != nil {
		events, err := s.audit.ListRecent(ctx, limit, query)
		if err != nil {
			return SearchResults{}, err
		}
		for _, event := range events {
			appendUnique(searchKeyForAudit(event), shellEventFromAudit(event))
		}
	}

	sortShellEvents(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return SearchResults{Items: items}, nil
}

func (s *ShellService) notificationReadAt(ctx context.Context, userID int64) (*time.Time, error) {
	if userID <= 0 || s.notificationState == nil {
		return nil, nil
	}
	return s.notificationState.GetNotificationReadAt(ctx, userID)
}

func (s *ShellService) listNotificationAlertHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	if reader, ok := s.alerts.(shellAlertHistoryTypeReader); ok {
		return reader.ListAlertHistoryByTypes(ctx, limit, domain.AlertEventTriggered, domain.AlertEventResolved)
	}
	history, err := s.alerts.ListAlertHistory(ctx, limit)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.AlertHistoryEvent, 0, len(history))
	for _, item := range history {
		if item.EventType != domain.AlertEventTriggered && item.EventType != domain.AlertEventResolved {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *ShellService) searchAlertHistory(ctx context.Context, query string, limit int) ([]domain.AlertHistoryEvent, error) {
	if reader, ok := s.alerts.(shellAlertHistorySearchReader); ok {
		return reader.SearchAlertHistory(ctx, query, limit)
	}
	history, err := s.alerts.ListAlertHistory(ctx, limit)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.AlertHistoryEvent, 0, len(history))
	for _, item := range history {
		if !matchesQuery(query, item.ServerName, item.Message, item.Metric, item.Detail, item.ActorUsername) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func markNotificationReadState(items []ShellEventItem, readAt *time.Time) int {
	if len(items) == 0 {
		return 0
	}
	if readAt == nil || readAt.IsZero() {
		for i := range items {
			items[i].IsRead = false
		}
		return len(items)
	}

	cutoff := readAt.UTC()
	unreadCount := 0
	for i := range items {
		items[i].IsRead = !items[i].CreatedAt.After(cutoff)
		if !items[i].IsRead {
			unreadCount++
		}
	}
	return unreadCount
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

func shellHistoryFetchLimit(limit int) int {
	fetchLimit := normalizeShellLimit(limit, 20) * shellHistoryFetchFactor
	if fetchLimit < shellHistoryMinFetch {
		fetchLimit = shellHistoryMinFetch
	}
	if fetchLimit > shellHistoryMaxFetch {
		return shellHistoryMaxFetch
	}
	return fetchLimit
}

func sortShellEvents(items []ShellEventItem) {
	sort.Slice(items, func(i int, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}

func shellEventFromCommand(item storage.CommandLogListItem) ShellEventItem {
	title := "执行命令"
	severity := "info"
	if item.ExitCode != 0 {
		title = "命令执行失败"
		severity = "warning"
	}
	serverName := strings.TrimSpace(item.ServerName)
	if serverName == "" {
		serverName = fmt.Sprintf("#%d", item.ServerID)
	}
	return ShellEventItem{
		Kind:       "command",
		Severity:   severity,
		Title:      title,
		Summary:    serverName + " · " + item.Command,
		CreatedAt:  item.ExecutedAt,
		ServerID:   item.ServerID,
		ServerName: item.ServerName,
		RoutePath:  routePathForCommand(item.ServerID),
	}
}

func shellEventFromAuthEvent(event domain.AuthEvent) ShellEventItem {
	severity := "info"
	if event.EventType == domain.AuthEventLoginFailed {
		severity = "warning"
	}
	return ShellEventItem{
		Kind:      "auth",
		Severity:  severity,
		Title:     authEventTitle(event.EventType),
		Summary:   buildAuthEventMessage(event),
		CreatedAt: event.CreatedAt,
		Username:  event.Username,
		RoutePath: routePathForAuthEvent(event.EventType),
	}
}

func shellEventFromCurrentAlert(alert domain.AlertEvent) ShellEventItem {
	severity := alert.Severity
	if severity == "" {
		severity = "warning"
	}
	return ShellEventItem{
		Kind:       "alert",
		Severity:   severity,
		Title:      alertHistoryTitle(domain.AlertEventTriggered),
		Summary:    alert.ServerName + " · " + alert.Message,
		CreatedAt:  alert.TriggeredAt,
		ServerID:   alert.ServerID,
		ServerName: alert.ServerName,
		RoutePath:  routePathForAlert(alert.ServerID),
	}
}

func shellEventFromAlertHistory(item domain.AlertHistoryEvent) ShellEventItem {
	title := alertHistoryTitle(item.EventType)
	severity := item.Severity
	if severity == "" {
		severity = "warning"
	}
	summary := item.ServerName + " · " + item.Message
	if strings.TrimSpace(item.Detail) != "" {
		summary += " · " + item.Detail
	}
	return ShellEventItem{
		Kind:       "alert",
		Severity:   severity,
		Title:      title,
		Summary:    summary,
		CreatedAt:  item.CreatedAt,
		ServerID:   item.ServerID,
		ServerName: item.ServerName,
		Username:   item.ActorUsername,
		RoutePath:  routePathForAlert(item.ServerID),
	}
}

func shellEventFromAudit(event domain.AuditEvent) ShellEventItem {
	return ShellEventItem{
		Kind:       event.Kind,
		Severity:   event.Severity,
		Title:      event.Title,
		Summary:    event.Summary,
		CreatedAt:  event.CreatedAt,
		ServerID:   event.ServerID,
		ServerName: event.ServerName,
		Username:   event.Username,
		RoutePath:  routePathForAudit(event.Kind, event.ServerID),
	}
}

func searchKeyForCurrentAlert(alert domain.AlertEvent) string {
	if alert.ID > 0 {
		return fmt.Sprintf("alert|%d|%s", alert.ID, domain.AlertEventTriggered)
	}
	return fmt.Sprintf("alert|%d|%s|%s|%s", alert.ServerID, domain.AlertEventTriggered, alert.Metric, alert.TriggeredAt.UTC().Format(time.RFC3339Nano))
}

func searchKeyForAlertHistory(item domain.AlertHistoryEvent) string {
	if item.EventType == domain.AlertEventTriggered && item.AlertID > 0 {
		return fmt.Sprintf("alert|%d|%s", item.AlertID, item.EventType)
	}
	if item.AlertID > 0 {
		return fmt.Sprintf("alert|%d|%s|%s", item.AlertID, item.EventType, item.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	return fmt.Sprintf("alert|%d|%s|%s|%s", item.ServerID, item.EventType, item.Metric, item.CreatedAt.UTC().Format(time.RFC3339Nano))
}

func searchKeyForCommand(item storage.CommandLogListItem) string {
	if item.ID > 0 {
		return fmt.Sprintf("command|%d", item.ID)
	}
	return fmt.Sprintf("command|%d|%s|%s", item.ServerID, item.Command, item.ExecutedAt.UTC().Format(time.RFC3339Nano))
}

func searchKeyForAuthEvent(event domain.AuthEvent) string {
	if event.ID > 0 {
		return fmt.Sprintf("auth|%d", event.ID)
	}
	return fmt.Sprintf("auth|%s|%s|%s", event.Username, event.EventType, event.CreatedAt.UTC().Format(time.RFC3339Nano))
}

func searchKeyForAudit(event domain.AuditEvent) string {
	if event.ID > 0 {
		return fmt.Sprintf("audit|%d", event.ID)
	}
	return fmt.Sprintf("audit|%s|%s|%s", event.Kind, event.Title, event.CreatedAt.UTC().Format(time.RFC3339Nano))
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

func alertHistoryTitle(eventType string) string {
	switch eventType {
	case domain.AlertEventTriggered:
		return "告警触发"
	case domain.AlertEventAcknowledged:
		return "告警确认"
	case domain.AlertEventMuted:
		return "告警静默"
	case domain.AlertEventResolved:
		return "告警恢复"
	default:
		return eventType
	}
}

func routePathForAlert(serverID int64) string {
	_ = serverID
	return "/alerts"
}

func routePathForCommand(serverID int64) string {
	_ = serverID
	return "/commands"
}

func routePathForAuthEvent(eventType string) string {
	_ = eventType
	return "/users"
}

func routePathForAudit(kind string, serverID int64) string {
	switch kind {
	case domain.AuditKindServer:
		if serverID > 0 {
			return fmt.Sprintf("/servers/%d", serverID)
		}
		return "/servers"
	case domain.AuditKindCommand:
		return "/commands"
	case domain.AuditKindAlert, domain.AuditKindAlertRule:
		return "/alerts"
	default:
		if serverID > 0 {
			return fmt.Sprintf("/servers/%d", serverID)
		}
		return ""
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
