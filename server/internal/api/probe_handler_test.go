package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

const collectorBatchCommand = `sh -c '
set -eu
section() { printf "__HOSTDECK__:%s\n" "$1"; }
section cpu
cat /proc/stat
sleep 1
cat /proc/stat
section meminfo
cat /proc/meminfo
section disk
df -P /
section loadavg
cat /proc/loadavg
section osrelease
cat /etc/os-release
section kernel
uname -r
section uptime
cat /proc/uptime
'`

const collectorBatchOutput = `__HOSTDECK__:cpu
cpu  100 0 100 900 0 0 0 0 0 0
cpu  140 0 140 940 0 0 0 0 0 0
__HOSTDECK__:meminfo
MemTotal: 2048000 kB
MemAvailable: 1024000 kB
__HOSTDECK__:disk
Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/vda1 100000 55000 45000 55% /
__HOSTDECK__:loadavg
0.15 0.20 0.25 1/100 12345
__HOSTDECK__:osrelease
PRETTY_NAME="Ubuntu 24.04 LTS"
__HOSTDECK__:kernel
6.8.0-31-generic
__HOSTDECK__:uptime
12345.67 23456.78
`

type fakeRunner struct {
	results     map[string]fakeRunResult
	targets     []sshx.Target
	fingerprint string
	err         error
}

type fakeRunResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (f *fakeRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	f.targets = append(f.targets, target)
	result, ok := f.results[command]
	if !ok {
		return "", "unexpected command", 127, nil
	}
	return result.stdout, result.stderr, result.exitCode, result.err
}

func (f *fakeRunner) GetHostKeyFingerprint(ctx context.Context, target sshx.Target) (string, error) {
	f.targets = append(f.targets, target)
	return f.fingerprint, f.err
}

type testSSHResponse struct {
	SSHOK                     bool   `json:"sshOk"`
	LatencyMS                 int64  `json:"latencyMs"`
	Error                     string `json:"error"`
	HostKeyFingerprint        string `json:"hostKeyFingerprint"`
	TrustedHostKeyFingerprint string `json:"trustedHostKeyFingerprint"`
	FingerprintMismatch       bool   `json:"fingerprintMismatch"`
	TrustRequired             bool   `json:"trustRequired"`
}

type probeResponse struct {
	Snapshot struct {
		Online            bool    `json:"online"`
		SSHOK             bool    `json:"sshOk"`
		CPUUsage          float64 `json:"cpuUsage"`
		MemoryUsage       float64 `json:"memoryUsage"`
		DiskUsage         float64 `json:"diskUsage"`
		Load1             float64 `json:"load1"`
		CollectDurationMS int64   `json:"collectDurationMs"`
	} `json:"snapshot"`
}

func openProbeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func seedProbeServer(t *testing.T, repo *storage.ServerRepository) {
	t.Helper()

	err := repo.Create(context.Background(), serverSeed())
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
}

func serverSeed() domain.Server {
	return domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       true,
	}
}

func TestProbeRoutes_TestSSHReturnsStatusAndLatency(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedProbeServer(t, serverRepo)
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	runner := &fakeRunner{
		fingerprint: "SHA256:first-seen",
		results: map[string]fakeRunResult{
			"true": {exitCode: 0},
		},
	}

	handler := api.NewProbeHandler(
		connectionService,
		runner,
		storage.NewStatusRepository(db),
		nil,
	)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/test-ssh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp testSSHResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.SSHOK {
		t.Fatalf("expected sshOk to be true")
	}
	if resp.HostKeyFingerprint != "SHA256:first-seen" {
		t.Fatalf("unexpected hostKeyFingerprint: %q", resp.HostKeyFingerprint)
	}
	if resp.TrustedHostKeyFingerprint != "" {
		t.Fatalf("unexpected trusted fingerprint: %q", resp.TrustedHostKeyFingerprint)
	}
	if resp.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", resp.LatencyMS)
	}
	if resp.TrustRequired {
		t.Fatalf("expected trustRequired to be false when ssh succeeds")
	}
	if len(runner.targets) != 2 {
		t.Fatalf("expected fingerprint probe and ssh run targets, got %d", len(runner.targets))
	}
	if runner.targets[0].Password != "super-secret" || runner.targets[1].Password != "super-secret" {
		t.Fatalf("expected decrypted password in ssh targets, got %+v", runner.targets)
	}
}

func TestProbeRoutes_TestSSHRequiresTrustWhenFingerprintIsUntrusted(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedProbeServer(t, serverRepo)
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	runner := &fakeRunner{
		fingerprint: "SHA256:first-seen",
		results: map[string]fakeRunResult{
			"true": {err: sshx.HostKeyTrustRequiredError{Actual: "SHA256:first-seen"}},
		},
	}

	handler := api.NewProbeHandler(
		connectionService,
		runner,
		storage.NewStatusRepository(db),
		nil,
	)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/test-ssh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp testSSHResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SSHOK {
		t.Fatalf("expected sshOk to be false")
	}
	if !resp.TrustRequired {
		t.Fatalf("expected trustRequired to be true")
	}
	if resp.HostKeyFingerprint != "SHA256:first-seen" {
		t.Fatalf("unexpected hostKeyFingerprint: %q", resp.HostKeyFingerprint)
	}
	if !strings.Contains(resp.Error, "SSH 主机指纹尚未信任") {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
}

func TestProbeRoutes_ProbePersistsLatestStatus(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedProbeServer(t, serverRepo)
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	runner := &fakeRunner{
		results: map[string]fakeRunResult{
			collectorBatchCommand: {stdout: collectorBatchOutput},
		},
	}

	handler := api.NewProbeHandler(
		connectionService,
		runner,
		storage.NewStatusRepository(db),
		nil,
	)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/probe", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp probeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Snapshot.Online || !resp.Snapshot.SSHOK {
		t.Fatalf("expected online ssh snapshot, got %+v", resp.Snapshot)
	}
	if resp.Snapshot.CPUUsage != 66.66666666666667 || resp.Snapshot.MemoryUsage != 50 || resp.Snapshot.DiskUsage != 55 || resp.Snapshot.Load1 != 0.15 {
		t.Fatalf("unexpected snapshot values: %+v", resp.Snapshot)
	}

	var memoryUsage float64
	if err := db.QueryRow(`SELECT memory_usage FROM server_status_latest WHERE server_id = 1`).Scan(&memoryUsage); err != nil {
		t.Fatalf("query latest status: %v", err)
	}
	if memoryUsage != 50 {
		t.Fatalf("expected persisted memory usage 50, got %v", memoryUsage)
	}
	if len(runner.targets) == 0 {
		t.Fatal("expected probe to execute ssh commands")
	}
	for _, target := range runner.targets {
		if target.Password != "super-secret" {
			t.Fatalf("expected decrypted password in probe target, got %q", target.Password)
		}
	}
}

func TestProbeRoutes_DisabledServerReturnsConflict(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "super-secret",
		CollectorMode: "ssh_only",
		Enabled:       false,
	})
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}

	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	handler := api.NewProbeHandler(connectionService, &fakeRunner{}, storage.NewStatusRepository(db), nil)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/test-ssh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestProbeRoutes_TestSSHReturnsFingerprintMismatch(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seed := serverSeed()
	seed.TrustedHostKeyFingerprint = "SHA256:trusted"
	if err := serverRepo.Create(context.Background(), seed); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	runner := &fakeRunner{
		fingerprint: "SHA256:actual",
		err:         sshx.HostKeyMismatchError{Expected: "SHA256:trusted", Actual: "SHA256:actual"},
		results: map[string]fakeRunResult{
			"true": {exitCode: 0},
		},
	}

	handler := api.NewProbeHandler(connectionService, runner, storage.NewStatusRepository(db), nil)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/test-ssh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp testSSHResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SSHOK {
		t.Fatal("expected sshOk to be false")
	}
	if !resp.FingerprintMismatch {
		t.Fatal("expected fingerprintMismatch to be true")
	}
	if resp.HostKeyFingerprint != "SHA256:actual" {
		t.Fatalf("unexpected hostKeyFingerprint: %q", resp.HostKeyFingerprint)
	}
	if resp.TrustedHostKeyFingerprint != "SHA256:trusted" {
		t.Fatalf("unexpected trustedHostKeyFingerprint: %q", resp.TrustedHostKeyFingerprint)
	}
	if !strings.Contains(resp.Error, "SSH 主机指纹不匹配") {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
}

func TestProbeRoutes_TrustHostKeyPersistsFingerprint(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedProbeServer(t, serverRepo)
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	handler := api.NewProbeHandler(connectionService, &fakeRunner{}, storage.NewStatusRepository(db), nil)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/trust-host-key", strings.NewReader(`{"fingerprint":"  SHA256:new-fingerprint  "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp struct {
		TrustedHostKeyFingerprint string `json:"trustedHostKeyFingerprint"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TrustedHostKeyFingerprint != "SHA256:new-fingerprint" {
		t.Fatalf("unexpected response fingerprint: %q", resp.TrustedHostKeyFingerprint)
	}

	servers, err := serverRepo.List(context.Background(), storage.ServerFilter{ID: 1})
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected one server, got %d", len(servers))
	}
	if servers[0].TrustedHostKeyFingerprint != "SHA256:new-fingerprint" {
		t.Fatalf("unexpected persisted fingerprint: %q", servers[0].TrustedHostKeyFingerprint)
	}
}

func TestProbeRoutes_TrustHostKeyReturnsNotFound(t *testing.T) {
	db := openProbeTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	handler := api.NewProbeHandler(connectionService, &fakeRunner{}, storage.NewStatusRepository(db), nil)
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/999/trust-host-key", strings.NewReader(`{"fingerprint":"SHA256:new-fingerprint"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}
