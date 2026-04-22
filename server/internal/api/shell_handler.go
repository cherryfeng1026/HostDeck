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

type ShellHandler struct {
	service *service.ShellService
}

type markNotificationsReadPayload struct {
	ReadBefore string `json:"readBefore"`
}

func NewShellHandler(service *service.ShellService) *ShellHandler {
	return &ShellHandler{service: service}
}

func RegisterShellRoutes(r chi.Router, h *ShellHandler) {
	if h == nil {
		return
	}

	r.Get("/api/notifications", h.ListNotifications)
	r.Post("/api/notifications/read", h.MarkNotificationsRead)
	r.Get("/api/activity-feed", h.ListActivity)
	r.Get("/api/search", h.Search)
}

func (h *ShellHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, service.ErrUnauthenticated)
		return
	}
	items, err := h.service.ListNotifications(r.Context(), user.ID, parseLimit(r, 20), domain.CanManageUsers(user.Role))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ShellHandler) MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, service.ErrUnauthenticated)
		return
	}
	readBefore, err := decodeMarkNotificationsReadPayload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.MarkNotificationsRead(r.Context(), user.ID, readBefore); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeMarkNotificationsReadPayload(r *http.Request) (time.Time, error) {
	defer r.Body.Close()
	var payload markNotificationsReadPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return time.Time{}, errors.New("readBefore 参数无效")
	}
	readBefore := strings.TrimSpace(payload.ReadBefore)
	if readBefore == "" {
		return time.Time{}, errors.New("readBefore 参数不能为空")
	}
	parsed, err := time.Parse(time.RFC3339, readBefore)
	if err != nil {
		return time.Time{}, errors.New("readBefore 参数无效")
	}
	return parsed.UTC(), nil
}

func (h *ShellHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListActivity(r.Context(), parseLimit(r, 20), currentUserCanManageInfrastructure(r), currentUserCanManageUsers(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, service.ShellEventList{Items: items})
}

func (h *ShellHandler) Search(w http.ResponseWriter, r *http.Request) {
	results, err := h.service.Search(r.Context(), r.URL.Query().Get("q"), parseLimit(r, 10), currentUserCanManageInfrastructure(r), currentUserCanManageUsers(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func currentUserCanManageInfrastructure(r *http.Request) bool {
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		return false
	}
	return domain.CanManageInfrastructure(user.Role)
}

func currentUserCanManageUsers(r *http.Request) bool {
	user, ok := authctx.CurrentUser(r.Context())
	if !ok {
		return false
	}
	return domain.CanManageUsers(user.Role)
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
