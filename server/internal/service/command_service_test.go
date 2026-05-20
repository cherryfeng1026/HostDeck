package service_test

import (
	"context"
	"errors"
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
	err    error
}

func (r commandTestRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	return r.stdout, r.stderr, 0, r.err
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

type commandTestTemplateStore struct {
	items   []domain.CommandTemplate
	ensured []domain.CommandTemplate
}

func (s *commandTestTemplateStore) EnsureDefaults(ctx context.Context, items []domain.CommandTemplate) error {
	s.ensured = append([]domain.CommandTemplate(nil), items...)
	s.items = append([]domain.CommandTemplate(nil), items...)
	return nil
}

func (s *commandTestTemplateStore) List(ctx context.Context, filter domain.CommandTemplateFilter) ([]domain.CommandTemplate, error) {
	if len(s.items) == 0 {
		s.items = defaultCommandTemplatesForTest()
	}
	items := make([]domain.CommandTemplate, len(s.items))
	copy(items, s.items)
	return items, nil
}

func (s *commandTestTemplateStore) GetByID(ctx context.Context, templateID string, username string) (domain.CommandTemplate, error) {
	if len(s.items) == 0 {
		s.items = defaultCommandTemplatesForTest()
	}
	for _, item := range s.items {
		if item.ID == templateID {
			return item, nil
		}
	}
	return domain.CommandTemplate{}, nil
}

func (s *commandTestTemplateStore) Create(ctx context.Context, input domain.CommandTemplateCreateInput, username string) (domain.CommandTemplate, error) {
	item := domain.CommandTemplate{
		ID:          "personal-test-template",
		Name:        input.Name,
		Description: input.Description,
		Command:     input.Command,
		Scope:       input.Scope,
		RiskLevel:   input.RiskLevel,
		CreatedBy:   username,
		IsFavorite:  input.Scope == domain.CommandTemplateScopePersonal,
		Variables:   input.Variables,
	}
	s.items = append(s.items, item)
	return item, nil
}

func (s *commandTestTemplateStore) SetFavorite(ctx context.Context, templateID string, username string, favorite bool) error {
	for index := range s.items {
		if s.items[index].ID == templateID {
			s.items[index].IsFavorite = favorite
		}
	}
	return nil
}

func TestCommandService_ListTemplatesReturnsSharedTemplates(t *testing.T) {
	svc := service.NewCommandService(
		commandTestResolver{},
		commandTestRunner{},
		&commandTestLogStore{},
		&commandTestTemplateStore{},
	)

	items, err := svc.ListTemplates(context.Background(), "operator")
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

func TestCommandService_EnsureDefaultTemplatesSeedsStore(t *testing.T) {
	store := &commandTestTemplateStore{}
	svc := service.NewCommandService(
		commandTestResolver{},
		commandTestRunner{},
		&commandTestLogStore{},
		store,
	)

	if err := svc.EnsureDefaultTemplates(context.Background()); err != nil {
		t.Fatalf("ensure default templates: %v", err)
	}
	if len(store.ensured) == 0 {
		t.Fatal("expected default templates to be seeded")
	}
}

func TestCommandService_CreateTemplateExtractsVariables(t *testing.T) {
	store := &commandTestTemplateStore{}
	svc := service.NewCommandService(
		commandTestResolver{},
		commandTestRunner{},
		&commandTestLogStore{},
		store,
	)

	item, err := svc.CreateTemplate(context.Background(), domain.CommandTemplateCreateInput{
		Name:      "检查服务状态",
		Command:   "systemctl status {{service}} --no-pager",
		Scope:     domain.CommandTemplateScopePersonal,
		RiskLevel: domain.CommandTemplateRiskNormal,
	}, "operator")
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if item.CreatedBy != "operator" {
		t.Fatalf("expected created by operator, got %+v", item)
	}
	if len(item.Variables) != 1 || item.Variables[0].Name != "service" {
		t.Fatalf("expected extracted service variable, got %+v", item.Variables)
	}
	if len(store.items) != 1 || !store.items[0].IsFavorite {
		t.Fatalf("expected personal template stored as favorite, got %+v", store.items)
	}
}

func TestCommandService_SetTemplateFavoriteUpdatesStore(t *testing.T) {
	store := &commandTestTemplateStore{items: defaultCommandTemplatesForTest()}
	svc := service.NewCommandService(
		commandTestResolver{},
		commandTestRunner{},
		&commandTestLogStore{},
		store,
	)

	if err := svc.SetTemplateFavorite(context.Background(), "system-disk-usage", "operator", true); err != nil {
		t.Fatalf("set template favorite: %v", err)
	}
	if !store.items[0].IsFavorite {
		t.Fatalf("expected template to be favorite, got %+v", store.items[0])
	}
}

func TestCommandService_ExecuteDetectsDangerousRemoveFlagVariants(t *testing.T) {
	tests := []string{
		"rm -fr /tmp/hostdeck",
		"rm -Rf /tmp/hostdeck",
		"rm -r -f /tmp/hostdeck",
	}

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			svc := service.NewCommandService(
				commandTestResolver{server: domain.Server{Password: "secret"}},
				commandTestRunner{},
				&commandTestLogStore{},
				&commandTestTemplateStore{},
			)

			_, err := svc.ExecuteWithExecutor(context.Background(), 1, command, 5*time.Second, "operator")
			if !errors.Is(err, service.ErrCommandRiskConfirmationRequired) {
				t.Fatalf("expected risk confirmation for %q, got %v", command, err)
			}
		})
	}
}

func TestCommandService_ExecuteRejectsTemplateCommandMismatch(t *testing.T) {
	svc := service.NewCommandService(
		commandTestResolver{server: domain.Server{Password: "secret"}},
		commandTestRunner{},
		&commandTestLogStore{},
		&commandTestTemplateStore{},
	)

	_, err := svc.ExecuteCommand(context.Background(), domain.CommandExecutionInput{
		ServerID:           1,
		Command:            "uptime",
		TemplateID:         "service-restart",
		RiskConfirmed:      true,
		ExecutorUsername:   "operator",
		ExecutorAuthMethod: "password",
	})
	if !errors.Is(err, service.ErrCommandTemplateMismatch) {
		t.Fatalf("expected template mismatch error, got %v", err)
	}
}

func TestCommandService_ExecuteAllowsResolvedTemplateCommand(t *testing.T) {
	logs := &commandTestLogStore{}
	svc := service.NewCommandService(
		commandTestResolver{server: domain.Server{Password: "secret"}},
		commandTestRunner{},
		logs,
		&commandTestTemplateStore{},
	)

	result, err := svc.ExecuteCommand(context.Background(), domain.CommandExecutionInput{
		ServerID:           1,
		Command:            "sudo systemctl restart nginx",
		TemplateID:         "service-restart",
		RiskConfirmed:      true,
		ExecutorUsername:   "operator",
		ExecutorAuthMethod: "password",
	})
	if err != nil {
		t.Fatalf("execute resolved template command: %v", err)
	}
	if result.Source != domain.CommandSourceTemplate || result.TemplateID != "service-restart" {
		t.Fatalf("expected template execution metadata, got %+v", result)
	}
	if len(logs.items) != 1 || logs.items[0].Source != domain.CommandSourceTemplate {
		t.Fatalf("expected template command log, got %+v", logs.items)
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
		&commandTestTemplateStore{},
	)

	result, err := svc.ExecuteWithExecutor(context.Background(), 1, "echo token=abc123", 5*time.Second, "operator")
	if err != nil {
		t.Fatalf("execute with redaction: %v", err)
	}
	if contains(result.Command, "abc123") || !containsRedactionMarker(result.Command) {
		t.Fatalf("expected redacted command text, got %q", result.Command)
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
	if logs.items[0].Command != result.Command {
		t.Fatalf("expected stored log to use sanitized command, got %+v", logs.items[0])
	}
}

func TestCommandService_ExecuteLogsRunnerFailures(t *testing.T) {
	logs := &commandTestLogStore{}
	svc := service.NewCommandService(
		commandTestResolver{server: domain.Server{Password: "secret"}},
		commandTestRunner{err: errors.New("ssh handshake failed")},
		logs,
		&commandTestTemplateStore{},
	)

	result, err := svc.ExecuteWithExecutor(context.Background(), 1, "uptime", 5*time.Second, "operator")
	if err == nil {
		t.Fatal("expected execution error")
	}
	if result.ExitCode != -1 || !contains(result.Stderr, "ssh handshake failed") {
		t.Fatalf("expected failed result to include execution error, got %+v", result)
	}
	if len(logs.items) != 1 {
		t.Fatalf("expected failed command log, got %d", len(logs.items))
	}
	if logs.items[0].ExitCode != -1 || !contains(logs.items[0].Stderr, "ssh handshake failed") {
		t.Fatalf("expected failed log to include execution error, got %+v", logs.items[0])
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

func defaultCommandTemplatesForTest() []domain.CommandTemplate {
	return []domain.CommandTemplate{
		{
			ID:        "system-disk-usage",
			Name:      "磁盘使用率",
			Command:   "df -h",
			Scope:     domain.CommandTemplateScopeShared,
			RiskLevel: domain.CommandTemplateRiskNormal,
		},
		{
			ID:        "service-restart",
			Name:      "重启服务",
			Command:   "sudo systemctl restart {{service}}",
			Scope:     domain.CommandTemplateScopeShared,
			RiskLevel: domain.CommandTemplateRiskDangerous,
			Variables: []domain.CommandTemplateVariable{{
				Name:     "service",
				Label:    "服务名",
				Required: true,
			}},
		},
	}
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
