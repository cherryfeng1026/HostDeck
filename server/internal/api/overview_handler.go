package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/service"
)

type OverviewHandler struct {
	service *service.OverviewService
}

func NewOverviewHandler(service *service.OverviewService) *OverviewHandler {
	return &OverviewHandler{service: service}
}

func RegisterOverviewRoutes(r chi.Router, h *OverviewHandler) {
	if h == nil {
		return
	}

	r.Get("/api/overview", h.GetOverview)
	r.Get("/api/overview/dashboard", h.GetDashboardOverview)
}

func (h *OverviewHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.GetOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *OverviewHandler) GetDashboardOverview(w http.ResponseWriter, r *http.Request) {
	since := time.Now().UTC().Add(-24 * time.Hour)
	if value := r.URL.Query().Get("range"); value != "" {
		var err error
		since, err = parseMetricsRange(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}

	response, err := h.service.GetDashboardOverview(r.Context(), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
