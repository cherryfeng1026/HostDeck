package api

import (
	"context"
	"encoding/json"
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

type ProbeAlertEvaluator interface {
	EvaluateServerSnapshot(ctx context.Context, server domain.Server, snapshot collector.Snapshot, sampledAt time.Time) error
}

type ProbeHandler struct {
	servers      ProbeServerResolver
	runner       sshx.Runner
	fingerprints sshx.HostKeyFingerprintReader
	statuses     LatestStatusWriter
	alerts       ProbeAlertEvaluator
}

type ProbeServerResolver interface {
	ResolveServer(ctx context.Context, serverID int64) (domain.Server, error)
	TrustHostKeyFingerprint(ctx context.Context, serverID int64, fingerprint string) error
}

type trustHostKeyPayload struct {
	Fingerprint string `json:"fingerprint"`
}

func NewProbeHandler(servers ProbeServerResolver, runner sshx.Runner, statuses LatestStatusWriter, alerts ProbeAlertEvaluator) *ProbeHandler {
	fingerprints, _ := runner.(sshx.HostKeyFingerprintReader)
	return &ProbeHandler{
		servers:      servers,
		runner:       runner,
		fingerprints: fingerprints,
		statuses:     statuses,
		alerts:       alerts,
	}
}

func RegisterProbeRoutes(r chi.Router, h *ProbeHandler) {
	if h == nil {
		return
	}

	r.Post("/api/servers/{id}/test-ssh", h.TestSSH)
	r.Post("/api/servers/{id}/trust-host-key", h.TrustHostKey)
	r.Post("/api/servers/{id}/probe", h.ProbeNow)
}

func (h *ProbeHandler) TestSSH(w http.ResponseWriter, r *http.Request) {
	server, err := h.loadServer(r)
	if err != nil {
		writeProbeResolveError(w, err)
		return
	}

	response := map[string]any{}
	if h.fingerprints != nil {
		fingerprint, fingerprintErr := h.fingerprints.GetHostKeyFingerprint(r.Context(), sshTargetFromServer(server))
		if fingerprint != "" {
			response["hostKeyFingerprint"] = fingerprint
		}
		if fingerprintErr != nil {
			var mismatch sshx.HostKeyMismatchError
			if errors.As(fingerprintErr, &mismatch) {
				response["sshOk"] = false
				response["fingerprintMismatch"] = true
				response["trustedHostKeyFingerprint"] = mismatch.Expected
				response["error"] = mismatch.Error()
				writeJSON(w, http.StatusOK, response)
				return
			}
		}
	}

	startedAt := time.Now()
	_, stderr, exitCode, runErr := h.runner.Run(r.Context(), sshTargetFromServer(server), "true")
	latencyMS := time.Since(startedAt).Milliseconds()

	response["sshOk"] = runErr == nil && exitCode == 0
	response["latencyMs"] = latencyMS
	response["trustedHostKeyFingerprint"] = server.TrustedHostKeyFingerprint
	if runErr != nil {
		response["error"] = runErr.Error()
		var mismatch sshx.HostKeyMismatchError
		if errors.As(runErr, &mismatch) {
			response["fingerprintMismatch"] = true
			response["hostKeyFingerprint"] = mismatch.Actual
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if exitCode != 0 && strings.TrimSpace(stderr) != "" {
		response["error"] = strings.TrimSpace(stderr)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *ProbeHandler) TrustHostKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseServerID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	defer r.Body.Close()
	var payload trustHostKeyPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.servers.TrustHostKeyFingerprint(r.Context(), id, payload.Fingerprint); err != nil {
		if errors.Is(err, service.ErrConnectionServerNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trustedHostKeyFingerprint": strings.TrimSpace(payload.Fingerprint),
	})
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
	sampledAt := time.Now()
	if err := h.statuses.UpsertLatest(r.Context(), server.ID, snapshot, sampledAt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if h.alerts != nil {
		if err := h.alerts.EvaluateServerSnapshot(r.Context(), server, snapshot, sampledAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
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
		Host:                      server.IP,
		Port:                      server.SSHPort,
		Username:                  server.Username,
		Password:                  server.Password,
		TrustedHostKeyFingerprint: server.TrustedHostKeyFingerprint,
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
