package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/service"
)

type ServerDetailHandler struct {
	service *service.OverviewService
}

func NewServerDetailHandler(service *service.OverviewService) *ServerDetailHandler {
	return &ServerDetailHandler{service: service}
}

func RegisterServerDetailRoutes(r chi.Router, h *ServerDetailHandler) {
	if h == nil {
		return
	}

	r.Get("/api/servers/{id}/status", h.GetServerStatus)
	r.Get("/api/servers/{id}/metrics", h.GetServerMetrics)
}

func (h *ServerDetailHandler) GetServerStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	response, err := h.service.GetServerStatus(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *ServerDetailHandler) GetServerMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	if value := r.URL.Query().Get("range"); value != "" {
		since, err = parseMetricsRange(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}

	points, err := h.service.GetServerMetrics(r.Context(), id, since)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"points": points,
	})
}

func parseMetricsRange(value string) (time.Time, error) {
	switch strings.TrimSpace(value) {
	case "1h":
		return time.Now().Add(-time.Hour), nil
	case "6h":
		return time.Now().Add(-6 * time.Hour), nil
	case "24h":
		return time.Now().Add(-24 * time.Hour), nil
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour), nil
	default:
		return time.Time{}, errors.New("不支持的时间范围")
	}
}
