package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
)

type fakeCommandRunner struct {
	mu           sync.Mutex
	targets       []sshx.Target
	errorByHost   map[string]error
	stdoutByHost  map[string]string
	stderrByHost  map[string]string
	exitCodeByHost map[string]int
}

func (f *fakeCommandRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	f.mu.Lock()
	f.targets = append(f.targets, target)
	f.mu.Unlock()
	if err := f.errorByHost[target.Host]; err != nil {
		return "", "", -1, err
	}
	stdout := "Filesystem Size Used Avail Use% Mounted on\n"
	if value, ok := f.stdoutByHost[target.Host]; ok {
		stdout = value
	}
	stderr := f.stderrByHost[target.Host]
	exitCode := 0
	if value, ok := f.exitCodeByHost[target.Host]; ok {
		exitCode = value
	}
	return stdout, stderr, exitCode, nil
}

func openCommandTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:command-handler-test?mode=memory&cache=shared")
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

func seedCommandServer(t *testing.T, repo *storage.ServerRepository) {
	t.Helper()

	servers := []domain.Server{
		{
			Name:          "prod-web-01",
			Hostname:      "prod-web-01",
			IP:            "10.0.0.21",
			SSHPort:       22,
			Username:      "root",
			AuthType:      "password",
			Password:      "super-secret",
			CollectorMode: "ssh_only",
			Enabled:       true,
		},
		{
			Name:          "prod-web-02",
			Hostname:      "prod-web-02",
			IP:            "10.0.0.22",
			SSHPort:       22,
			Username:      "root",
			AuthType:      "password",
			Password:      "super-secret-2",
			CollectorMode: "ssh_only",
			Enabled:       true,
		},
		{
			Name:          "prod-web-03",
			Hostname:      "prod-web-03",
			IP:            "10.0.0.23",
			SSHPort:       22,
			Username:      "root",
			AuthType:      "password",
			Password:      "super-secret-3",
			CollectorMode: "ssh_only",
			Enabled:       true,
		},
	}

	for _, server := range servers {
		if err := repo.Create(context.Background(), server); err != nil {
			t.Fatalf("seed server: %v", err)
		}
	}
}

func TestExecuteCommand_StoresLog(t *testing.T) {
	db := openCommandTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedCommandServer(t, serverRepo)
	logRepo := storage.NewCommandLogRepository(db)
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	runner := &fakeCommandRunner{}

	svc := service.NewCommandService(connectionService, runner, logRepo)
	result, err := svc.Execute(context.Background(), 1, "df -h", 15*time.Second)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.Command != "df -h" || result.ExitCode != 0 {
		t.Fatalf("unexpected command result: %+v", result)
	}
	if len(runner.targets) != 1 {
		t.Fatalf("expected one ssh target, got %d", len(runner.targets))
	}
	if runner.targets[0].Password != "super-secret" {
		t.Fatalf("expected decrypted password in ssh target, got %q", runner.targets[0].Password)
	}

	logs, err := logRepo.ListByServerID(context.Background(), 1)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 command log, got %d", len(logs))
	}
}

func TestCommandRoutes_ExecuteReturnsResult(t *testing.T) {
	db := openCommandTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedCommandServer(t, serverRepo)
	commandService := service.NewCommandService(
		service.NewServerConnectionService(
			serverRepo,
			storage.NewServerCredentialRepository(db),
			"test-master-key",
		),
		&fakeCommandRunner{},
		storage.NewCommandLogRepository(db),
	)

	router := httpx.NewRouterWithHandlers(
		api.NewServerHandler(serverRepo, nil),
		nil,
		nil,
		nil,
		api.NewCommandHandler(commandService),
		nil,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/servers/1/commands/execute",
		bytes.NewBufferString(`{"command":"df -h","timeoutSeconds":15,"source":"custom"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var result service.CommandResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Command != "df -h" || result.ExitCode != 0 {
		t.Fatalf("unexpected command result: %+v", result)
	}
}

func TestCommandRoutes_ExecuteBatchReturnsPerServerResults(t *testing.T) {
	db := openCommandTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedCommandServer(t, serverRepo)
	logRepo := storage.NewCommandLogRepository(db)
	runner := &fakeCommandRunner{
		errorByHost: map[string]error{
			"10.0.0.22": context.DeadlineExceeded,
		},
		stdoutByHost: map[string]string{
			"10.0.0.21": "server1 ok\n",
			"10.0.0.23": "server3 ok\n",
		},
	}
	commandService := service.NewCommandService(
		service.NewServerConnectionService(
			serverRepo,
			storage.NewServerCredentialRepository(db),
			"test-master-key",
		),
		runner,
		logRepo,
	)

	router := httpx.NewRouterWithHandlers(
		api.NewServerHandler(serverRepo, nil),
		nil,
		nil,
		nil,
		api.NewCommandHandler(commandService),
		nil,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/commands/execute",
		bytes.NewBufferString(`{"serverIds":[1,2,2,99,3],"command":"df -h","timeoutSeconds":15}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Results []service.BatchCommandResult `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(payload.Results) != 4 {
		t.Fatalf("expected 4 batch results after dedupe, got %d", len(payload.Results))
	}
	if !payload.Results[0].Success || payload.Results[0].ServerID != 1 {
		t.Fatalf("unexpected first result: %+v", payload.Results[0])
	}
	if payload.Results[1].Success || payload.Results[1].Error == "" || payload.Results[1].ServerID != 2 {
		t.Fatalf("unexpected second result: %+v", payload.Results[1])
	}
	if payload.Results[2].Success || payload.Results[2].Error == "" || payload.Results[2].ServerID != 99 {
		t.Fatalf("unexpected missing-server result: %+v", payload.Results[2])
	}
	if !payload.Results[3].Success || payload.Results[3].ServerID != 3 {
		t.Fatalf("unexpected last result: %+v", payload.Results[3])
	}

	logs1, err := logRepo.ListByServerID(context.Background(), 1)
	if err != nil {
		t.Fatalf("list logs for server 1: %v", err)
	}
	if len(logs1) != 1 {
		t.Fatalf("expected 1 log for server 1, got %d", len(logs1))
	}
	logs2, err := logRepo.ListByServerID(context.Background(), 2)
	if err != nil {
		t.Fatalf("list logs for server 2: %v", err)
	}
	if len(logs2) != 0 {
		t.Fatalf("expected 0 logs for failed server 2, got %d", len(logs2))
	}
	logs3, err := logRepo.ListByServerID(context.Background(), 3)
	if err != nil {
		t.Fatalf("list logs for server 3: %v", err)
	}
	if len(logs3) != 1 {
		t.Fatalf("expected 1 log for server 3, got %d", len(logs3))
	}
}

func TestCommandRoutes_ExecuteReturnsConflictForDisabledServer(t *testing.T) {
	db := openCommandTestDB(t)
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

	commandService := service.NewCommandService(
		service.NewServerConnectionService(
			serverRepo,
			storage.NewServerCredentialRepository(db),
			"test-master-key",
		),
		&fakeCommandRunner{},
		storage.NewCommandLogRepository(db),
	)

	router := httpx.NewRouterWithHandlers(
		api.NewServerHandler(serverRepo, nil),
		nil,
		nil,
		nil,
		api.NewCommandHandler(commandService),
		nil,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/servers/1/commands/execute",
		bytes.NewBufferString(`{"command":"df -h","timeoutSeconds":15,"source":"custom"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}
