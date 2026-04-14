package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
)

type LatestStatusWriter interface {
	UpsertLatest(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error
}

type ProbeHandler struct {
	servers  ProbeServerResolver
	runner   sshx.Runner
	statuses LatestStatusWriter
}

type ProbeServerResolver interface {
	ResolveServer(ctx context.Context, serverID int64) (domain.Server, error)
}

func NewProbeHandler(servers ProbeServerResolver, runner sshx.Runner, statuses LatestStatusWriter) *ProbeHandler {
	return &ProbeHandler{
		servers:  servers,
		runner:   runner,
		statuses: statuses,
	}
}

func RegisterProbeRoutes(r chi.Router, h *ProbeHandler) {
	if h == nil {
		return
	}

	r.Post("/api/servers/{id}/test-ssh", h.TestSSH)
	r.Post("/api/servers/{id}/probe", h.ProbeNow)
}

func (h *ProbeHandler) TestSSH(w http.ResponseWriter, r *http.Request) {
	server, err := h.loadServer(r)
	if err != nil {
		writeProbeResolveError(w, err)
		return
	}

	startedAt := time.Now()
	_, stderr, exitCode, runErr := h.runner.Run(r.Context(), sshTargetFromServer(server), "true")
	latencyMS := time.Since(startedAt).Milliseconds()

	response := map[string]any{
		"sshOk":     runErr == nil && exitCode == 0,
		"latencyMs": latencyMS,
	}
	if runErr != nil {
		response["error"] = runErr.Error()
	} else if exitCode != 0 && strings.TrimSpace(stderr) != "" {
		response["error"] = strings.TrimSpace(stderr)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *ProbeHandler) ProbeNow(w http.ResponseWriter, r *http.Request) {
	server, err := h.loadServer(r)
	if err != nil {
		writeProbeResolveError(w, err)
		return
	}

	snapshot, err := collector.NewSSHCollector(h.runner).Collect(r.Context(), server)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := h.statuses.UpsertLatest(r.Context(), server.ID, snapshot, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": snapshot,
	})
}

func (h *ProbeHandler) loadServer(r *http.Request) (domain.Server, error) {
	id, err := parseServerID(r)
	if err != nil {
		return domain.Server{}, err
	}
	return h.servers.ResolveServer(r.Context(), id)
}

func sshTargetFromServer(server domain.Server) sshx.Target {
	return sshx.Target{
		Host:     server.IP,
		Port:     server.SSHPort,
		Username: server.Username,
		Password: server.Password,
	}
}

func writeProbeResolveError(w http.ResponseWriter, err error) {
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
