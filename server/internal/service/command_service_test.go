package service_test

import (
	"context"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
)

type commandTestResolver struct {
	server domain.Server
}

func (r commandTestResolver) ResolveServer(ctx context.Context, serverID int64) (domain.Server, error) {
	server := r.server
	server.ID = serverID
	if server.Name == "" {
		server.Name = "prod-web-01"
	}
	if server.IP == "" {
		server.IP = "10.0.0.21"
	}
	if server.SSHPort == 0 {
		server.SSHPort = 22
	}
	if server.Username == "" {
		server.Username = "root"
	}
	return server, nil
}

type commandTestRunner struct {
	stdout string
	stderr string
}

func (r commandTestRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	return r.stdout, r.stderr, 0, nil
}

type commandTestLogStore struct {
	items []domain.CommandLog
}

func (s *commandTestLogStore) Create(ctx context.Context, log domain.CommandLog) error {
	s.items = append(s.items, log)
	return nil
}

func (s *commandTestLogStore) ListHistory(ctx context.Context, filter domain.CommandHistoryFilter) ([]domain.CommandLog, error) {
	return nil, nil
}

func TestCommandService_ListTemplatesReturnsSharedTemplates(t *testing.T) {
	svc := service.NewCommandService(
		commandTestResolver{},
		commandTestRunner{},
		&commandTestLogStore{},
	)

	items, err := svc.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected default templates")
	}
	if items[0].Scope != domain.CommandTemplateScopeShared {
		t.Fatalf("expected shared scope, got %+v", items[0])
	}

	foundVariableTemplate := false
	foundDangerousTemplate := false
	for _, item := range items {
		if len(item.Variables) > 0 {
			foundVariableTemplate = true
		}
		if item.RiskLevel == domain.CommandTemplateRiskDangerous {
			foundDangerousTemplate = true
		}
	}
	if !foundVariableTemplate {
		t.Fatal("expected at least one parameterized template")
	}
	if !foundDangerousTemplate {
		t.Fatal("expected at least one dangerous template")
	}
}

func TestCommandService_ExecuteRedactsSensitiveOutputAndTruncates(t *testing.T) {
	logs := &commandTestLogStore{}
	svc := service.NewCommandService(
		commandTestResolver{server: domain.Server{Password: "secret"}},
		commandTestRunner{
			stdout: "password=super-secret\ntoken: abc123\nAuthorization: Bearer xyz\n" + longCommandOutput(17000),
			stderr: "api_key=my-key\n",
		},
		logs,
	)

	result, err := svc.ExecuteWithExecutor(context.Background(), 1, "printenv", 5*time.Second, "operator")
	if err != nil {
		t.Fatalf("execute with redaction: %v", err)
	}
	if containsSensitiveToken(result.Stdout) || containsSensitiveToken(result.Stderr) {
		t.Fatalf("expected redacted outputs, got stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if !containsRedactionMarker(result.Stdout) || !containsRedactionMarker(result.Stderr) {
		t.Fatalf("expected redaction markers, got stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if !containsTruncationMarker(result.Stdout) {
		t.Fatalf("expected truncation marker, got stdout length=%d", len(result.Stdout))
	}
	if len(logs.items) != 1 {
		t.Fatalf("expected 1 log item, got %d", len(logs.items))
	}
	if logs.items[0].Stdout != result.Stdout || logs.items[0].Stderr != result.Stderr {
		t.Fatalf("expected stored log to use sanitized output, got %+v", logs.items[0])
	}
}

func longCommandOutput(size int) string {
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = 'a'
	}
	return string(buf)
}

func containsSensitiveToken(value string) bool {
	for _, token := range []string{"super-secret", "abc123", "xyz", "my-key"} {
		if len(token) > 0 && contains(value, token) {
			return true
		}
	}
	return false
}

func containsRedactionMarker(value string) bool {
	return contains(value, "[REDACTED]")
}

func containsTruncationMarker(value string) bool {
	return contains(value, "输出已截断")
}

func contains(value string, target string) bool {
	return len(target) > 0 && len(value) >= len(target) && (func() bool { return stringIndex(value, target) >= 0 })()
}

func stringIndex(value string, target string) int {
	for i := 0; i+len(target) <= len(value); i++ {
		if value[i:i+len(target)] == target {
			return i
		}
	}
	return -1
}
