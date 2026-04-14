package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/service"
)

type ShellHandler struct {
	service *service.ShellService
}

func NewShellHandler(service *service.ShellService) *ShellHandler {
	return &ShellHandler{service: service}
}

func RegisterShellRoutes(r chi.Router, h *ShellHandler) {
	if h == nil {
		return
	}

	r.Get("/api/notifications", h.ListNotifications)
	r.Get("/api/activity-feed", h.ListActivity)
	r.Get("/api/search", h.Search)
}

func (h *ShellHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListNotifications(r.Context(), parseLimit(r, 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ShellHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListActivity(r.Context(), parseLimit(r, 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ShellHandler) Search(w http.ResponseWriter, r *http.Request) {
	results, err := h.service.Search(r.Context(), r.URL.Query().Get("q"), parseLimit(r, 10))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func parseLimit(r *http.Request, fallback int) int {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return fallback
	}

	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 50 {
		return 50
	}
	return limit
}
