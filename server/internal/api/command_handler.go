package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/authctx"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
)

type CommandHandler struct {
	service *service.CommandService
	audit   AuditEventWriter
}

func (h *CommandHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("查询命令模板失败"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type commandExecutePayload struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	Source         string `json:"source"`
}

type batchCommandExecutePayload struct {
	ServerIDs      []int64 `json:"serverIds"`
	Command        string  `json:"command"`
	TimeoutSeconds int     `json:"timeoutSeconds"`
}

func NewCommandHandler(service *service.CommandService, audit ...AuditEventWriter) *CommandHandler {
	handler := &CommandHandler{service: service}
	if len(audit) > 0 {
		handler.audit = audit[0]
	}
	return handler
}

func RegisterCommandTemplateRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Get("/api/commands/templates", h.ListTemplates)
}

func RegisterCommandHistoryRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Get("/api/commands/history", h.ListHistory)
}

func RegisterCommandReadRoutes(r chi.Router, h *CommandHandler) {
	RegisterCommandTemplateRoutes(r, h)
	RegisterCommandHistoryRoutes(r, h)
}

func RegisterCommandWriteRoutes(r chi.Router, h *CommandHandler) {
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

	if limit := strings.TrimSpace(query.Get("limit")); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("limit 参数无效"))
			return
		}
		if value <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("limit 必须大于 0"))
			return
		}
		filter.Limit = value
	}
	if serverID := strings.TrimSpace(query.Get("serverId")); serverID != "" {
		value, err := strconv.ParseInt(serverID, 10, 64)
		if err != nil {
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
			writeError(w, http.StatusBadRequest, errors.New("startTime 参数无效"))
			return
		}
		filter.StartTime = &value
	}
	if end := strings.TrimSpace(query.Get("endTime")); end != "" {
		value, err := time.Parse(time.RFC3339, end)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("endTime 参数无效"))
			return
		}
		filter.EndTime = &value
	}
	if filter.StartTime != nil && filter.EndTime != nil && filter.EndTime.Before(*filter.StartTime) {
		writeError(w, http.StatusBadRequest, errors.New("endTime 必须晚于或等于 startTime"))
		return
	}

	items, err := h.service.ListHistory(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("查询命令历史失败"))
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *CommandHandler) Execute(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	defer r.Body.Close()
	var payload commandExecutePayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	user, _ := authctx.CurrentUser(r.Context())
	timeout := time.Duration(payload.TimeoutSeconds) * time.Second
	result, err := h.service.ExecuteWithExecutor(r.Context(), id, payload.Command, timeout, user.Username)
	if err != nil {
		writeCommandError(w, err)
		return
	}
	if h.audit != nil {
		_ = h.audit.Create(r.Context(), domain.AuditEvent{
			Kind:      domain.AuditKindCommand,
			Severity:  commandSeverity(result.ExitCode),
			Title:     commandTitle(result.ExitCode),
			Summary:   payload.Command,
			ServerID:  id,
			Username:  user.Username,
			CreatedAt: result.ExecutedAt,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CommandHandler) ExecuteBatch(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var payload batchCommandExecutePayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	user, _ := authctx.CurrentUser(r.Context())
	timeout := time.Duration(payload.TimeoutSeconds) * time.Second
	results, err := h.service.ExecuteBatchWithExecutor(r.Context(), payload.ServerIDs, payload.Command, timeout, user.Username)
	if err != nil {
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
				Summary:    payload.Command,
				ServerID:   item.ServerID,
				ServerName: item.ServerName,
				Username:   user.Username,
				CreatedAt:  item.Result.ExecutedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
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

func writeCommandError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrConnectionServerNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, service.ErrServerDisabled) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, service.ErrServerPasswordNotConfigured) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeError(w, http.StatusInternalServerError, errors.New("解析服务器失败"))
}
