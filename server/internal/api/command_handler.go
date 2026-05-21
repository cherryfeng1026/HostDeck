package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/authctx"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

type CommandHandler struct {
	service *service.CommandService
	audit   AuditEventWriter
}

func (h *CommandHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	user, _ := authctx.CurrentUser(r.Context())
	slog.Info("command templates list started", "username", user.Username)
	items, err := h.service.ListTemplates(r.Context(), user.Username)
	if err != nil {
		slog.Error("command templates list failed", "username", user.Username, "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("查询命令模板失败"))
		return
	}
	slog.Info("command templates list succeeded", "username", user.Username, "count", len(items))
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type commandTemplateVariablePayload struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Placeholder  string `json:"placeholder"`
	DefaultValue string `json:"defaultValue"`
	Required     bool   `json:"required"`
}

type commandTemplateCreatePayload struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Command     string                           `json:"command"`
	Scope       string                           `json:"scope"`
	RiskLevel   string                           `json:"riskLevel"`
	Variables   []commandTemplateVariablePayload `json:"variables"`
}

type commandTemplateFavoritePayload struct {
	Favorite bool `json:"favorite"`
}

func toCommandTemplateVariables(items []commandTemplateVariablePayload) []domain.CommandTemplateVariable {
	if len(items) == 0 {
		return nil
	}
	result := make([]domain.CommandTemplateVariable, 0, len(items))
	for _, item := range items {
		result = append(result, domain.CommandTemplateVariable{
			Name:         item.Name,
			Label:        item.Label,
			Placeholder:  item.Placeholder,
			DefaultValue: item.DefaultValue,
			Required:     item.Required,
		})
	}
	return result
}

type commandExecutePayload struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	Source         string `json:"source"`
	TemplateID     string `json:"templateId"`
	RiskLevel      string `json:"riskLevel"`
	RiskConfirmed  bool   `json:"riskConfirmed"`
}

type batchCommandExecutePayload struct {
	ServerIDs      []int64 `json:"serverIds"`
	Command        string  `json:"command"`
	TimeoutSeconds int     `json:"timeoutSeconds"`
	Source         string  `json:"source"`
	TemplateID     string  `json:"templateId"`
	RiskLevel      string  `json:"riskLevel"`
	RiskConfirmed  bool    `json:"riskConfirmed"`
}

const (
	defaultCommandTimeoutSeconds = 15
	minCommandTimeoutSeconds     = 1
	maxCommandTimeoutSeconds     = 60
)

func NewCommandHandler(service *service.CommandService, audit ...AuditEventWriter) *CommandHandler {
	handler := &CommandHandler{service: service}
	if len(audit) > 0 {
		handler.audit = audit[0]
	}
	return handler
}

func RegisterCommandTemplateReadRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Get("/api/commands/templates", h.ListTemplates)
}

func RegisterCommandTemplateWriteRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Post("/api/commands/templates", h.CreateTemplate)
	r.Post("/api/commands/templates/{id}/favorite", h.SetTemplateFavorite)
}

func (h *CommandHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("请先登录"))
		return
	}

	payload, err := decodeJSON[commandTemplateCreatePayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(payload.Scope) == domain.CommandTemplateScopeShared && !domain.CanManageInfrastructure(user.Role) {
		writeError(w, http.StatusForbidden, errors.New("当前账号没有创建共享模板的权限"))
		return
	}

	item, err := h.service.CreateTemplate(r.Context(), domain.CommandTemplateCreateInput{
		Name:        payload.Name,
		Description: payload.Description,
		Command:     payload.Command,
		Scope:       payload.Scope,
		RiskLevel:   payload.RiskLevel,
		Variables:   toCommandTemplateVariables(payload.Variables),
	}, user.Username)
	if err != nil {
		if errors.Is(err, storage.ErrCommandTemplateAccessDenied) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		var validationErr storage.CommandTemplateValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusBadRequest, validationErr)
			return
		}
		var conflictErr storage.CommandTemplateConflictError
		if errors.As(err, &conflictErr) {
			writeError(w, http.StatusConflict, errors.New("命令模板已存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, errors.New("创建命令模板失败"))
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *CommandHandler) SetTemplateFavorite(w http.ResponseWriter, r *http.Request) {
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("请先登录"))
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("template id 不能为空"))
		return
	}

	payload, err := decodeJSON[commandTemplateFavoritePayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.SetTemplateFavorite(r.Context(), id, user.Username, payload.Favorite); err != nil {
		if errors.Is(err, storage.ErrCommandTemplateNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, storage.ErrCommandTemplateAccessDenied) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		var validationErr storage.CommandTemplateValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusBadRequest, validationErr)
			return
		}
		writeError(w, http.StatusInternalServerError, errors.New("更新模板收藏失败"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"favorite": payload.Favorite})
}

func RegisterCommandHistoryRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Get("/api/commands/history", h.ListHistory)
}

func RegisterCommandReadRoutes(r chi.Router, h *CommandHandler) {
	RegisterCommandTemplateReadRoutes(r, h)
	RegisterCommandHistoryRoutes(r, h)
}

func RegisterCommandWriteRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	RegisterCommandTemplateWriteRoutes(r, h)
	RegisterCommandExecutionRoutes(r, h)
}

func RegisterCommandExecutionRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Post("/api/servers/{id}/commands/execute", h.Execute)
	r.Post("/api/commands/execute", h.ExecuteBatch)
}

func RegisterCommandRoutes(r chi.Router, h *CommandHandler) {
	RegisterCommandReadRoutes(r, h)
	RegisterCommandWriteRoutes(r, h)
}

func (h *CommandHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := domain.CommandHistoryFilter{}
	user, _ := authctx.CurrentUser(r.Context())

	if limit := strings.TrimSpace(query.Get("limit")); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil {
			slog.Warn("command history list rejected", "username", user.Username, "limit", limit, "error", err)
			writeError(w, http.StatusBadRequest, errors.New("limit 参数无效"))
			return
		}
		if value <= 0 {
			slog.Warn("command history list rejected", "username", user.Username, "limit", value, "reason", "non_positive_limit")
			writeError(w, http.StatusBadRequest, errors.New("limit 必须大于 0"))
			return
		}
		filter.Limit = value
	}
	if serverID := strings.TrimSpace(query.Get("serverId")); serverID != "" {
		value, err := strconv.ParseInt(serverID, 10, 64)
		if err != nil {
			slog.Warn("command history list rejected", "username", user.Username, "server_id", serverID, "error", err)
			writeError(w, http.StatusBadRequest, errors.New("serverId 参数无效"))
			return
		}
		if value <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("serverId 必须大于 0"))
			return
		}
		filter.ServerID = value
	}
	filter.ExecutorUsername = strings.TrimSpace(query.Get("executorUsername"))
	filter.Keyword = strings.TrimSpace(query.Get("keyword"))
	if start := strings.TrimSpace(query.Get("startTime")); start != "" {
		value, err := time.Parse(time.RFC3339, start)
		if err != nil {
			slog.Warn("command history list rejected", "username", user.Username, "start_time", start, "error", err)
			writeError(w, http.StatusBadRequest, errors.New("startTime 参数无效"))
			return
		}
		filter.StartTime = &value
	}
	if end := strings.TrimSpace(query.Get("endTime")); end != "" {
		value, err := time.Parse(time.RFC3339, end)
		if err != nil {
			slog.Warn("command history list rejected", "username", user.Username, "end_time", end, "error", err)
			writeError(w, http.StatusBadRequest, errors.New("endTime 参数无效"))
			return
		}
		filter.EndTime = &value
	}
	if filter.StartTime != nil && filter.EndTime != nil && filter.EndTime.Before(*filter.StartTime) {
		slog.Warn(
			"command history list rejected",
			"username", user.Username,
			"start_time", filter.StartTime.Format(time.RFC3339),
			"end_time", filter.EndTime.Format(time.RFC3339),
			"reason", "end_before_start",
		)
		writeError(w, http.StatusBadRequest, errors.New("endTime 必须晚于或等于 startTime"))
		return
	}

	slog.Info(
		"command history list started",
		"username", user.Username,
		"limit", filter.Limit,
		"server_id", filter.ServerID,
		"executor_username", filter.ExecutorUsername,
		"keyword", filter.Keyword,
		"start_time", formatOptionalTime(filter.StartTime),
		"end_time", formatOptionalTime(filter.EndTime),
	)
	items, err := h.service.ListHistory(r.Context(), filter)
	if err != nil {
		slog.Error(
			"command history list failed",
			"username", user.Username,
			"limit", filter.Limit,
			"server_id", filter.ServerID,
			"executor_username", filter.ExecutorUsername,
			"keyword", filter.Keyword,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, errors.New("查询命令历史失败"))
		return
	}
	slog.Info("command history list succeeded", "username", user.Username, "count", len(items), "server_id", filter.ServerID)
	writeJSON(w, http.StatusOK, items)
}

func (h *CommandHandler) Execute(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		slog.Warn("command execute rejected", "server_id", chi.URLParam(r, "id"), "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload, err := decodeJSON[commandExecutePayload](w, r)
	if err != nil {
		slog.Warn("command execute rejected", "server_id", id, "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	timeout, err := commandTimeout(payload.TimeoutSeconds)
	if err != nil {
		slog.Warn("command execute rejected", "server_id", id, "timeout_seconds", payload.TimeoutSeconds, "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	timeoutSeconds := int(timeout / time.Second)

	user, _ := authctx.CurrentUser(r.Context())
	slog.Info(
		"command execute started",
		"username", user.Username,
		"server_id", id,
		"timeout_seconds", timeoutSeconds,
		"source", strings.TrimSpace(payload.Source),
		"command_length", len(strings.TrimSpace(payload.Command)),
	)
	result, err := h.service.ExecuteCommand(r.Context(), domain.CommandExecutionInput{
		ServerID:           id,
		Command:            payload.Command,
		Timeout:            timeout,
		Source:             payload.Source,
		TemplateID:         payload.TemplateID,
		RiskLevel:          payload.RiskLevel,
		RiskConfirmed:      payload.RiskConfirmed,
		ExecutorUsername:   user.Username,
		ExecutorAuthMethod: string(currentAuthMethod(r)),
		RequestID:          requestID(r),
	})
	if err != nil {
		slog.Warn(
			"command execute failed",
			"username", user.Username,
			"server_id", id,
			"timeout_seconds", timeoutSeconds,
			"command_length", len(strings.TrimSpace(payload.Command)),
			"error", err,
		)
		writeCommandError(w, err)
		return
	}
	if h.audit != nil {
		_ = h.audit.Create(r.Context(), domain.AuditEvent{
			Kind:      domain.AuditKindCommand,
			Severity:  commandSeverity(result.ExitCode),
			Title:     commandTitle(result.ExitCode),
			Summary:   commandAuditSummary(result.ExitCode),
			ServerID:  id,
			Username:  user.Username,
			CreatedAt: result.ExecutedAt,
		})
	}
	slog.Info(
		"command execute succeeded",
		"username", user.Username,
		"server_id", id,
		"exit_code", result.ExitCode,
		"duration_ms", result.DurationMS,
		"stdout_length", len(result.Stdout),
		"stderr_length", len(result.Stderr),
		"executed_at", result.ExecutedAt,
	)
	writeJSON(w, http.StatusOK, result)
}

func (h *CommandHandler) ExecuteBatch(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeJSON[batchCommandExecutePayload](w, r)
	if err != nil {
		slog.Warn("batch command execute rejected", "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	timeout, err := commandTimeout(payload.TimeoutSeconds)
	if err != nil {
		slog.Warn("batch command execute rejected", "timeout_seconds", payload.TimeoutSeconds, "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	timeoutSeconds := int(timeout / time.Second)

	user, _ := authctx.CurrentUser(r.Context())
	slog.Info(
		"batch command execute started",
		"username", user.Username,
		"server_count", len(payload.ServerIDs),
		"timeout_seconds", timeoutSeconds,
		"command_length", len(strings.TrimSpace(payload.Command)),
	)
	results, err := h.service.ExecuteBatchWithInput(r.Context(), domain.CommandExecutionInput{
		ServerIDs:          payload.ServerIDs,
		Command:            payload.Command,
		Timeout:            timeout,
		Source:             payload.Source,
		TemplateID:         payload.TemplateID,
		RiskLevel:          payload.RiskLevel,
		RiskConfirmed:      payload.RiskConfirmed,
		ExecutorUsername:   user.Username,
		ExecutorAuthMethod: string(currentAuthMethod(r)),
		RequestID:          requestID(r),
	})
	if err != nil {
		slog.Warn(
			"batch command execute failed",
			"username", user.Username,
			"server_count", len(payload.ServerIDs),
			"timeout_seconds", timeoutSeconds,
			"command_length", len(strings.TrimSpace(payload.Command)),
			"error", err,
		)
		writeCommandError(w, err)
		return
	}
	if h.audit != nil {
		for _, item := range results {
			if item.ServerID == 0 {
				continue
			}
			_ = h.audit.Create(r.Context(), domain.AuditEvent{
				Kind:       domain.AuditKindCommand,
				Severity:   batchCommandSeverity(item),
				Title:      batchCommandTitle(item),
				Summary:    commandAuditSummary(item.Result.ExitCode),
				ServerID:   item.ServerID,
				ServerName: item.ServerName,
				Username:   user.Username,
				CreatedAt:  item.Result.ExecutedAt,
			})
		}
	}
	successCount, failureCount := summarizeBatchResults(results)
	slog.Info(
		"batch command execute succeeded",
		"username", user.Username,
		"server_count", len(payload.ServerIDs),
		"result_count", len(results),
		"success_count", successCount,
		"failure_count", failureCount,
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

func currentAuthMethod(r *http.Request) authctx.AuthMethod {
	method, ok := authctx.CurrentAuthMethod(r.Context())
	if !ok {
		return authctx.AuthMethodSession
	}
	return method
}

func requestID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

func commandTimeout(seconds int) (time.Duration, error) {
	if seconds == 0 {
		seconds = defaultCommandTimeoutSeconds
	}
	if seconds < minCommandTimeoutSeconds || seconds > maxCommandTimeoutSeconds {
		return 0, errors.New("timeoutSeconds 必须在 1 到 60 秒之间")
	}
	return time.Duration(seconds) * time.Second, nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func summarizeBatchResults(results []service.BatchCommandResult) (int, int) {
	successCount := 0
	failureCount := 0
	for _, item := range results {
		if item.Success {
			successCount++
			continue
		}
		failureCount++
	}
	return successCount, failureCount
}

func commandTitle(exitCode int) string {
	if exitCode == 0 {
		return "执行命令"
	}
	return "命令执行失败"
}

func commandSeverity(exitCode int) string {
	if exitCode == 0 {
		return "info"
	}
	return "warning"
}

func batchCommandTitle(item service.BatchCommandResult) string {
	if item.Success {
		return "批量命令执行"
	}
	return "批量命令执行失败"
}

func batchCommandSeverity(item service.BatchCommandResult) string {
	if item.Success {
		return "info"
	}
	return "warning"
}

func commandAuditSummary(exitCode int) string {
	if exitCode == 0 {
		return "命令执行成功"
	}
	return "命令执行失败"
}

func writeCommandError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrConnectionServerNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, service.ErrServerDisabled) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, service.ErrServerCredentialNotConfigured) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, service.ErrCommandRiskConfirmationRequired) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, service.ErrCommandTemplateMismatch) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, storage.ErrCommandTemplateNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, storage.ErrCommandTemplateAccessDenied) {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var executionErr service.CommandExecutionError
	if errors.As(err, &executionErr) {
		writeError(w, http.StatusBadGateway, executionErr)
		return
	}
	writeError(w, http.StatusInternalServerError, errors.New("解析服务器失败"))
}
