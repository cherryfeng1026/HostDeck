package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
)

type fakeRunner struct {
	results map[string]fakeRunResult
	targets []sshx.Target
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

type testSSHResponse struct {
	SSHOK     bool  `json:"sshOk"`
	LatencyMS int64 `json:"latencyMs"`
}

type probeResponse struct {
	Snapshot struct {
		Online      bool    `json:"online"`
		SSHOK       bool    `json:"sshOk"`
		CPUUsage    float64 `json:"cpuUsage"`
		MemoryUsage float64 `json:"memoryUsage"`
		DiskUsage   float64 `json:"diskUsage"`
		Load1       float64 `json:"load1"`
	} `json:"snapshot"`
}

func openProbeTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:probe-handler-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := storage.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return db
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
		results: map[string]fakeRunResult{
			"true": {exitCode: 0},
		},
	}

	handler := api.NewProbeHandler(
		connectionService,
		runner,
		storage.NewStatusRepository(db),
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
	if resp.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", resp.LatencyMS)
	}
	if len(runner.targets) != 1 {
		t.Fatalf("expected one ssh target, got %d", len(runner.targets))
	}
	if runner.targets[0].Password != "super-secret" {
		t.Fatalf("expected decrypted password in ssh target, got %q", runner.targets[0].Password)
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
			"sh -c 'cat /proc/stat; sleep 1; cat /proc/stat'": {stdout: "cpu  100 0 100 900 0 0 0 0 0 0\ncpu  140 0 140 940 0 0 0 0 0 0\n"},
			"cat /proc/meminfo":                               {stdout: "MemTotal: 2048000 kB\nMemAvailable: 1024000 kB\n"},
			"df -P /":                                         {stdout: "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 100000 55000 45000 55% /\n"},
			"cat /proc/loadavg":                               {stdout: "0.15 0.20 0.25 1/100 12345\n"},
			"cat /etc/os-release":                             {stdout: "PRETTY_NAME=\"Ubuntu 24.04 LTS\"\n"},
			"uname -r":                                        {stdout: "6.8.0-31-generic\n"},
			"cat /proc/uptime":                                {stdout: "12345.67 23456.78\n"},
		},
	}

	handler := api.NewProbeHandler(
		connectionService,
		runner,
		storage.NewStatusRepository(db),
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
	handler := api.NewProbeHandler(connectionService, &fakeRunner{}, storage.NewStatusRepository(db))
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(serverRepo, nil), handler, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/test-ssh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}
