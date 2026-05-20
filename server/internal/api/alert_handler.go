package api

import (
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

type AlertHandler struct {
	service *service.AlertService
	audit   AuditEventWriter
}

const maxAlertMuteDurationMinutes = 43200

type muteAlertPayload struct {
	DurationMinutes int `json:"durationMinutes"`
}

type notificationSettingsPayload struct {
	Enabled               bool   `json:"enabled"`
	WebhookURL            string `json:"webhookURL"`
	ClearWebhookURL       bool   `json:"clearWebhookURL"`
	WebhookTimeoutSeconds int    `json:"webhookTimeoutSeconds"`
}

func NewAlertHandler(service *service.AlertService, audit ...AuditEventWriter) *AlertHandler {
	handler := &AlertHandler{service: service}
	if len(audit) > 0 {
		handler.audit = audit[0]
	}
	return handler
}

func RegisterAlertReadRoutes(r chi.Router, h *AlertHandler) {
	if h == nil {
		return
	}

	r.Get("/api/alerts", h.ListAlerts)
	r.Get("/api/alert-history", h.ListHistory)
	r.Get("/api/alert-rules", h.ListRules)
}

func RegisterAlertWriteRoutes(r chi.Router, h *AlertHandler) {
	if h == nil {
		return
	}

	r.Get("/api/alert-notification-settings", h.GetNotificationSettings)
	r.Get("/api/alert-notification-deliveries", h.ListNotificationDeliveries)
	r.Post("/api/alert-rules", h.CreateRule)
	r.Put("/api/alert-rules/{id}", h.UpdateRule)
	r.Delete("/api/alert-rules/{id}", h.DeleteRule)
	r.Put("/api/alert-notification-settings", h.UpdateNotificationSettings)
	r.Post("/api/alert-notification-settings/test", h.TestNotificationSettings)
	r.Post("/api/alert-notification-deliveries/retry", h.RetryNotificationDeliveries)
	r.Post("/api/alerts/{id}/ack", h.AcknowledgeAlert)
	r.Post("/api/alerts/{id}/mute", h.MuteAlert)
}

func RegisterAlertRoutes(r chi.Router, h *AlertHandler) {
	RegisterAlertReadRoutes(r, h)
	RegisterAlertWriteRoutes(r, h)
}

func (h *AlertHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListCurrentAlerts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AlertHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("limit 参数无效"))
			return
		}
		limit = parsed
	}
	items, err := h.service.ListAlertHistory(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AlertHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AlertHandler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.GetNotificationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("加载通知设置失败"))
		return
	}
	writeJSON(w, http.StatusOK, sanitizeNotificationSettings(settings))
}

func (h *AlertHandler) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeJSON[notificationSettingsPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := h.service.SaveNotificationSettings(r.Context(), domain.AlertNotificationSettings{
		Enabled:               payload.Enabled,
		WebhookURL:            payload.WebhookURL,
		ClearWebhookURL:       payload.ClearWebhookURL,
		WebhookTimeoutSeconds: payload.WebhookTimeoutSeconds,
	})
	if err != nil {
		writeNotificationSettingsError(w, err)
		return
	}
	if h.audit != nil {
		user, _ := authctx.CurrentUser(r.Context())
		_ = h.audit.Create(r.Context(), domain.AuditEvent{
			Kind:      domain.AuditKindAlert,
			Severity:  "info",
			Title:     "更新通知设置",
			Summary:   notificationSettingsAuditSummary(settings),
			Username:  user.Username,
			CreatedAt: time.Now().UTC(),
		})
	}
	writeJSON(w, http.StatusOK, sanitizeNotificationSettings(settings))
}

func (h *AlertHandler) ListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, err := parseOptionalPositiveLimit(r, 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := h.service.ListNotificationDeliveries(r.Context(), r.URL.Query().Get("status"), limit)
	if err != nil {
		if errors.Is(err, service.ErrAlertNotificationDeliveryUnavailable) {
			writeError(w, http.StatusInternalServerError, errors.New("通知投递记录不可用"))
			return
		}
		if errors.Is(err, service.ErrInvalidNotificationDeliveryStatus) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AlertHandler) TestNotificationSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := authctx.CurrentUser(r.Context())
	if err := h.service.SendTestNotification(r.Context(), user.Username); err != nil {
		writeNotificationDeliveryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandler) RetryNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, err := parseOptionalPositiveLimit(r, 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	retried, err := h.service.RetryNotificationDeliveries(r.Context(), limit)
	if err != nil {
		writeNotificationDeliveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"retried": retried})
}

func (h *AlertHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	rule, err := decodeJSON[domain.AlertRule](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.CreateRule(r.Context(), rule); err != nil {
		writeAlertRuleError(w, err)
		return
	}
	if h.audit != nil {
		user, _ := authctx.CurrentUser(r.Context())
		_ = h.audit.Create(r.Context(), domain.AuditEvent{
			Kind:      domain.AuditKindAlertRule,
			Severity:  "info",
			Title:     "新增告警规则",
			Summary:   rule.Metric + " " + rule.Operator,
			Username:  user.Username,
			CreatedAt: time.Now().UTC(),
		})
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *AlertHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rule, err := decodeJSON[domain.AlertRule](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rule.ID = id
	if err := h.service.UpdateRule(r.Context(), rule); err != nil {
		writeAlertRuleError(w, err)
		return
	}
	if h.audit != nil {
		user, _ := authctx.CurrentUser(r.Context())
		_ = h.audit.Create(r.Context(), domain.AuditEvent{
			Kind:      domain.AuditKindAlertRule,
			Severity:  "info",
			Title:     "更新告警规则",
			Summary:   rule.Metric + " " + rule.Operator,
			Username:  user.Username,
			CreatedAt: time.Now().UTC(),
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.DeleteRule(r.Context(), id); err != nil {
		writeAlertRuleError(w, err)
		return
	}
	if h.audit != nil {
		user, _ := authctx.CurrentUser(r.Context())
		_ = h.audit.Create(r.Context(), domain.AuditEvent{
			Kind:      domain.AuditKindAlertRule,
			Severity:  "info",
			Title:     "删除告警规则",
			Summary:   "规则已删除",
			Username:  user.Username,
			CreatedAt: time.Now().UTC(),
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("请先登录"))
		return
	}
	item, err := h.service.AcknowledgeAlert(r.Context(), id, user.Username)
	if err != nil {
		if errors.Is(err, service.ErrAlertNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, service.ErrAlertActionNotAllowed) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AlertHandler) MuteAlert(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("请先登录"))
		return
	}
	payload, err := decodeJSON[muteAlertPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if payload.DurationMinutes <= 0 {
		payload.DurationMinutes = 30
	}
	if payload.DurationMinutes > maxAlertMuteDurationMinutes {
		writeError(w, http.StatusBadRequest, errors.New("静默时长不能超过 30 天"))
		return
	}
	item, err := h.service.MuteAlert(r.Context(), id, user.Username, time.Now().Add(time.Duration(payload.DurationMinutes)*time.Minute))
	if err != nil {
		if errors.Is(err, service.ErrAlertNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, service.ErrAlertActionNotAllowed) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func parseOptionalPositiveLimit(r *http.Request, defaultLimit int) (int, error) {
	limit := defaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return 0, errors.New("limit 参数无效")
		}
		limit = parsed
	}
	return limit, nil
}

func writeAlertRuleError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrAlertRuleNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, service.ErrInvalidAlertRuleScope) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, service.ErrInvalidAlertRule) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func writeNotificationDeliveryError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrAlertNotificationDeliveryUnavailable) {
		writeError(w, http.StatusInternalServerError, errors.New("通知投递记录不可用"))
		return
	}
	writeError(w, http.StatusBadGateway, err)
}

func writeNotificationSettingsError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrInvalidAlertNotificationSettings) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeError(w, http.StatusInternalServerError, errors.New("保存通知设置失败"))
}

func sanitizeNotificationSettings(settings domain.AlertNotificationSettings) domain.AlertNotificationSettings {
	settings.WebhookConfigured = strings.TrimSpace(settings.WebhookURL) != ""
	settings.WebhookURL = ""
	return settings
}

func notificationSettingsAuditSummary(settings domain.AlertNotificationSettings) string {
	if settings.Enabled {
		return "Webhook 通知已启用"
	}
	return "Webhook 通知已关闭"
}
