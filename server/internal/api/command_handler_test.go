package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

type fakeCommandRunner struct {
	mu             sync.Mutex
	targets        []sshx.Target
	errorByHost    map[string]error
	stdoutByHost   map[string]string
	stderrByHost   map[string]string
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
	return testsupport.OpenPostgresTestDB(t)
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
	result, err := svc.ExecuteWithExecutor(context.Background(), 1, "df -h", 15*time.Second, "operator")
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
	if logs[0].ExecutorUsername != "operator" {
		t.Fatalf("expected executor username to persist, got %+v", logs[0])
	}
}

func TestCommandRoutes_ExecuteReturnsResult(t *testing.T) {
	db := openCommandTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedCommandServer(t, serverRepo)
	auditRepo := storage.NewAuditEventRepository(db)
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
		api.NewServerHandler(serverRepo, nil, auditRepo),
		nil,
		nil,
		nil,
		api.NewCommandHandler(commandService, auditRepo),
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

	audits, err := auditRepo.ListRecent(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audits))
	}
	if audits[0].Kind != domain.AuditKindCommand || audits[0].Title != "执行命令" {
		t.Fatalf("unexpected audit event: %+v", audits[0])
	}
	if audits[0].ServerID != 1 || audits[0].Summary != "命令执行成功" {
		t.Fatalf("unexpected audit payload: %+v", audits[0])
	}
	if strings.Contains(audits[0].Summary, "df -h") {
		t.Fatalf("expected audit summary to redact command text, got %+v", audits[0])
	}
}

func TestCommandRoutes_ExecuteBatchReturnsPerServerResults(t *testing.T) {
	db := openCommandTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedCommandServer(t, serverRepo)
	logRepo := storage.NewCommandLogRepository(db)
	auditRepo := storage.NewAuditEventRepository(db)
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
		api.NewServerHandler(serverRepo, nil, auditRepo),
		nil,
		nil,
		nil,
		api.NewCommandHandler(commandService, auditRepo),
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

	audits, err := auditRepo.ListRecent(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) != 3 {
		t.Fatalf("expected 3 audit events for concrete servers, got %d", len(audits))
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

func TestCommandRoutes_ListHistoryReturnsFilteredLogs(t *testing.T) {
	db := openCommandTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	seedCommandServer(t, serverRepo)
	logRepo := storage.NewCommandLogRepository(db)
	now := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
	if err := logRepo.Create(context.Background(), domain.CommandLog{
		ServerID:         1,
		ExecutorUsername: "admin",
		Command:          "df -h",
		Stdout:           "ok",
		Stderr:           "",
		ExitCode:         0,
		DurationMS:       120,
		ExecutedAt:       now,
	}); err != nil {
		t.Fatalf("create log: %v", err)
	}
	if err := logRepo.Create(context.Background(), domain.CommandLog{
		ServerID:         2,
		ExecutorUsername: "viewer",
		Command:          "uptime",
		Stdout:           "load average",
		Stderr:           "",
		ExitCode:         0,
		DurationMS:       80,
		ExecutedAt:       now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("create second log: %v", err)
	}

	commandService := service.NewCommandService(
		service.NewServerConnectionService(
			serverRepo,
			storage.NewServerCredentialRepository(db),
			"test-master-key",
		),
		&fakeCommandRunner{},
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

	req := httptest.NewRequest(http.MethodGet, "/api/commands/history?serverId=1&executorUsername=admin&keyword=10.0.0.21&startTime=2026-04-21T08:59:30Z&endTime=2026-04-21T09:00:30Z&limit=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var items []domain.CommandLog
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
	if items[0].ServerID != 1 || items[0].ExecutorUsername != "admin" || items[0].Command != "df -h" {
		t.Fatalf("unexpected history item: %+v", items[0])
	}
}

func TestCommandRoutes_ListHistoryRejectsInvalidTimeRange(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/api/commands/history?startTime=2026-04-21T09:00:30Z&endTime=2026-04-21T08:59:30Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["error"] != "endTime 必须晚于或等于 startTime" {
		t.Fatalf("unexpected error message: %q", payload["error"])
	}
}

func TestCommandRoutes_ListHistoryRejectsNonPositiveNumericFilters(t *testing.T) {
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

	cases := []struct {
		name      string
		query     string
		wantError string
	}{
		{name: "serverId", query: "serverId=-1", wantError: "serverId 必须大于 0"},
		{name: "limit", query: "limit=0", wantError: "limit 必须大于 0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/commands/history?"+tc.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			var payload map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["error"] != tc.wantError {
				t.Fatalf("unexpected error message: %q", payload["error"])
			}
		})
	}
}

func TestCommandRoutes_ListTemplatesReturnsSharedTemplates(t *testing.T) {
	commandService := service.NewCommandService(
		service.NewServerConnectionService(nil, nil, "test-master-key"),
		&fakeCommandRunner{},
		storage.NewCommandLogRepository(openCommandTestDB(t)),
	)
	router := httpx.NewRouterWithHandlers(
		nil,
		nil,
		nil,
		nil,
		api.NewCommandHandler(commandService),
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/commands/templates", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []domain.CommandTemplate `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode templates: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatal("expected command templates")
	}
	if payload.Items[0].Scope != domain.CommandTemplateScopeShared {
		t.Fatalf("expected shared scope, got %+v", payload.Items[0])
	}
}
