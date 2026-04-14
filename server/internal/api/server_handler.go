package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

type ServerStore interface {
	Create(ctx context.Context, item domain.Server) error
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
	Update(ctx context.Context, item domain.Server) error
	Delete(ctx context.Context, id int64) error
}

type LiveServerLister interface {
	ListLive(ctx context.Context, filter storage.ServerFilter) ([]service.LiveServerItem, error)
}

type ServerHandler struct {
	store      ServerStore
	liveLister LiveServerLister
}

type serverPayload struct {
	Name          string   `json:"name"`
	Hostname      string   `json:"hostname"`
	IP            string   `json:"ip"`
	SSHPort       int      `json:"sshPort"`
	Username      string   `json:"username"`
	AuthType      string   `json:"authType"`
	Password      string   `json:"password"`
	CollectorMode string   `json:"collectorMode"`
	Tags          []string `json:"tags"`
	Purpose       string   `json:"purpose"`
	Remark        string   `json:"remark"`
	Enabled       *bool    `json:"enabled"`
}

func NewServerHandler(store ServerStore, liveLister LiveServerLister) *ServerHandler {
	return &ServerHandler{store: store, liveLister: liveLister}
}

func RegisterServerReadRoutes(r chi.Router, h *ServerHandler) {
	if h == nil {
		return
	}

	r.Get("/api/servers", h.List)
}

func RegisterServerWriteRoutes(r chi.Router, h *ServerHandler) {
	if h == nil {
		return
	}

	r.Post("/api/servers", h.Create)
	r.Put("/api/servers/{id}", h.Update)
	r.Delete("/api/servers/{id}", h.Delete)
}

func RegisterServerRoutes(r chi.Router, h *ServerHandler) {
	RegisterServerReadRoutes(r, h)
	RegisterServerWriteRoutes(r, h)
}

func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	collectorMode := strings.TrimSpace(r.URL.Query().Get("collectorMode"))
	if collectorMode != "" {
		collectorMode = domain.NormalizeCollectorMode(collectorMode)
	}

	filter := storage.ServerFilter{
		Keyword:       r.URL.Query().Get("keyword"),
		Tag:           r.URL.Query().Get("tag"),
		CollectorMode: collectorMode,
	}
	if r.URL.Query().Get("includeStatus") == "1" {
		if h.liveLister == nil {
			writeError(w, http.StatusInternalServerError, errors.New("服务器实时列表服务未配置"))
			return
		}
		items, err := h.liveLister.ListLive(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}

	items, err := h.store.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	item, err := decodeServerPayload(r, 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.store.Create(r.Context(), item); err != nil {
		writeServerStoreError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *ServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	item, err := decodeServerPayload(r, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.store.Update(r.Context(), item); err != nil {
		writeServerStoreError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeServerPayload(r *http.Request, id int64) (domain.Server, error) {
	defer r.Body.Close()

	var payload serverPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&payload); err != nil {
		return domain.Server{}, err
	}

	return payload.toDomain(id)
}

func (p serverPayload) toDomain(id int64) (domain.Server, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return domain.Server{}, errors.New("服务器名称不能为空")
	}

	hostname := strings.TrimSpace(p.Hostname)
	if hostname == "" {
		return domain.Server{}, errors.New("主机名不能为空")
	}

	ip := strings.TrimSpace(p.IP)
	if ip == "" {
		return domain.Server{}, errors.New("IP 地址不能为空")
	}

	username := strings.TrimSpace(p.Username)
	if username == "" {
		return domain.Server{}, errors.New("用户名不能为空")
	}

	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}

	sshPort := p.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	authType := strings.TrimSpace(p.AuthType)
	if authType == "" {
		authType = "password"
	}
	password := strings.TrimSpace(p.Password)
	if authType == "password" && id == 0 && password == "" {
		return domain.Server{}, errors.New("SSH 密码不能为空")
	}

	collectorMode := domain.NormalizeCollectorMode(p.CollectorMode)

	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}

	return domain.Server{
		ID:            id,
		Name:          name,
		Hostname:      hostname,
		IP:            ip,
		SSHPort:       sshPort,
		Username:      username,
		AuthType:      authType,
		Password:      password,
		CollectorMode: collectorMode,
		Tags:          tags,
		Purpose:       strings.TrimSpace(p.Purpose),
		Remark:        strings.TrimSpace(p.Remark),
		Enabled:       enabled,
	}, nil
}

func parseServerID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func writeServerStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrServerIPConflict) {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}
