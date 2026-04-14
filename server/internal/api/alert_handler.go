package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
)

type AlertHandler struct {
	service *service.AlertService
}

func NewAlertHandler(service *service.AlertService) *AlertHandler {
	return &AlertHandler{service: service}
}

func RegisterAlertReadRoutes(r chi.Router, h *AlertHandler) {
	if h == nil {
		return
	}

	r.Get("/api/alerts", h.ListAlerts)
	r.Get("/api/alert-rules", h.ListRules)
}

func RegisterAlertWriteRoutes(r chi.Router, h *AlertHandler) {
	if h == nil {
		return
	}

	r.Post("/api/alert-rules", h.CreateRule)
	r.Put("/api/alert-rules/{id}", h.UpdateRule)
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

func (h *AlertHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AlertHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	rule, err := decodeAlertRulePayload(r, 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.CreateRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *AlertHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rule, err := decodeAlertRulePayload(r, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.UpdateRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeAlertRulePayload(r *http.Request, id int64) (domain.AlertRule, error) {
	defer r.Body.Close()
	var rule domain.AlertRule
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rule); err != nil {
		return domain.AlertRule{}, err
	}
	rule.ID = id
	return rule, nil
}
