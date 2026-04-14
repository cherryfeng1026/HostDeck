package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/service"
)

type CommandHandler struct {
	service *service.CommandService
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

func NewCommandHandler(service *service.CommandService) *CommandHandler {
	return &CommandHandler{service: service}
}

func RegisterCommandRoutes(r chi.Router, h *CommandHandler) {
	if h == nil {
		return
	}

	r.Post("/api/servers/{id}/commands/execute", h.Execute)
	r.Post("/api/commands/execute", h.ExecuteBatch)
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

	timeout := time.Duration(payload.TimeoutSeconds) * time.Second
	result, err := h.service.Execute(r.Context(), id, payload.Command, timeout)
	if err != nil {
		writeCommandError(w, err)
		return
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

	timeout := time.Duration(payload.TimeoutSeconds) * time.Second
	results, err := h.service.ExecuteBatch(r.Context(), payload.ServerIDs, payload.Command, timeout)
	if err != nil {
		writeCommandError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
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
