package api

import (
	"net/http"

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
}

func (h *OverviewHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.GetOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
