package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/sshx"
)

type CommandServerResolver interface {
	ResolveServer(ctx context.Context, serverID int64) (domain.Server, error)
}

type CommandLogStore interface {
	Create(ctx context.Context, log domain.CommandLog) error
	ListHistory(ctx context.Context, filter domain.CommandHistoryFilter) ([]domain.CommandLog, error)
}

type CommandTemplateStore interface {
	EnsureDefaults(ctx context.Context, items []domain.CommandTemplate) error
	List(ctx context.Context, filter domain.CommandTemplateFilter) ([]domain.CommandTemplate, error)
	Create(ctx context.Context, input domain.CommandTemplateCreateInput, username string) (domain.CommandTemplate, error)
	SetFavorite(ctx context.Context, templateID string, username string, favorite bool) error
}

type CommandResult struct {
	Command    string    `json:"command"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	ExitCode   int       `json:"exitCode"`
	DurationMS int64     `json:"durationMs"`
	ExecutedAt time.Time `json:"executedAt"`
}

type BatchCommandResult struct {
	ServerID   int64         `json:"serverId"`
	ServerName string        `json:"serverName"`
	Success    bool          `json:"success"`
	Result     CommandResult `json:"result"`
	Error      string        `json:"error,omitempty"`
}

type CommandService struct {
	servers   CommandServerResolver
	runner    sshx.Runner
	logs      CommandLogStore
	templates CommandTemplateStore
}

func NewCommandService(servers CommandServerResolver, runner sshx.Runner, logs CommandLogStore, templates CommandTemplateStore) *CommandService {
	return &CommandService{
		servers:   servers,
		runner:    runner,
		logs:      logs,
		templates: templates,
	}
}

func (s *CommandService) Execute(ctx context.Context, serverID int64, command string, timeout time.Duration) (CommandResult, error) {
	return s.ExecuteWithExecutor(ctx, serverID, command, timeout, "")
}

func (s *CommandService) ExecuteWithExecutor(ctx context.Context, serverID int64, command string, timeout time.Duration, executorUsername string) (CommandResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return CommandResult{}, errors.New("命令不能为空")
	}

	server, err := s.servers.ResolveServer(ctx, serverID)
	if err != nil {
		return CommandResult{}, err
	}

	return s.executeOnServer(ctx, server, command, timeout, executorUsername)
}

func (s *CommandService) ExecuteBatch(ctx context.Context, serverIDs []int64, command string, timeout time.Duration) ([]BatchCommandResult, error) {
	return s.ExecuteBatchWithExecutor(ctx, serverIDs, command, timeout, "")
}

func (s *CommandService) ExecuteBatchWithExecutor(ctx context.Context, serverIDs []int64, command string, timeout time.Duration, executorUsername string) ([]BatchCommandResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("命令不能为空")
	}

	ids := normalizeServerIDs(serverIDs)
	if len(ids) == 0 {
		return nil, errors.New("目标服务器不能为空")
	}
	if len(ids) > 20 {
		return nil, errors.New("目标服务器数量不能超过 20 台")
	}

	results := make([]BatchCommandResult, len(ids))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, serverID := range ids {
		wg.Add(1)
		go func(index int, id int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			server, err := s.servers.ResolveServer(ctx, id)
			if err != nil {
				results[index] = BatchCommandResult{
					ServerID: id,
					Success:  false,
					Error:    err.Error(),
				}
				return
			}

			result, execErr := s.executeOnServer(ctx, server, command, timeout, executorUsername)
			results[index] = BatchCommandResult{
				ServerID:   id,
				ServerName: server.Name,
				Success:    execErr == nil,
				Result:     result,
			}
			if execErr != nil {
				results[index].Error = execErr.Error()
			}
		}(i, serverID)
	}

	wg.Wait()
	return results, nil
}

func (s *CommandService) ListHistory(ctx context.Context, filter domain.CommandHistoryFilter) ([]domain.CommandLog, error) {
	return s.logs.ListHistory(ctx, filter)
}

func (s *CommandService) executeOnServer(ctx context.Context, server domain.Server, command string, timeout time.Duration, executorUsername string) (CommandResult, error) {
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	startedAt := time.Now()
	stdout, stderr, exitCode, err := s.runner.Run(runCtx, sshx.Target{
		Host:                      server.IP,
		Port:                      server.SSHPort,
		Username:                  server.Username,
		Password:                  server.Password,
		TrustedHostKeyFingerprint: server.TrustedHostKeyFingerprint,
		Timeout:                   timeout,
	}, command)
	if err != nil {
		return CommandResult{}, err
	}

	result := CommandResult{
		Command:    command,
		Stdout:     sanitizeCommandOutput(stdout),
		Stderr:     sanitizeCommandOutput(stderr),
		ExitCode:   exitCode,
		DurationMS: time.Since(startedAt).Milliseconds(),
		ExecutedAt: time.Now(),
	}
	if err := s.logs.Create(ctx, domain.CommandLog{
		ServerID:         server.ID,
		ExecutorUsername: strings.TrimSpace(executorUsername),
		Command:          result.Command,
		Stdout:           result.Stdout,
		Stderr:           result.Stderr,
		ExitCode:         result.ExitCode,
		DurationMS:       result.DurationMS,
		ExecutedAt:       result.ExecutedAt,
	}); err != nil {
		return CommandResult{}, err
	}

	return result, nil
}

func (s *CommandService) EnsureDefaultTemplates(ctx context.Context) error {
	if s.templates == nil {
		return nil
	}
	return s.templates.EnsureDefaults(ctx, defaultCommandTemplates())
}

func (s *CommandService) ListTemplates(ctx context.Context, username string) ([]domain.CommandTemplate, error) {
	if s.templates == nil {
		items := make([]domain.CommandTemplate, len(defaultCommandTemplates()))
		copy(items, defaultCommandTemplates())
		return items, nil
	}
	return s.templates.List(ctx, domain.CommandTemplateFilter{Username: strings.TrimSpace(username)})
}

func (s *CommandService) CreateTemplate(ctx context.Context, input domain.CommandTemplateCreateInput, username string) (domain.CommandTemplate, error) {
	if s.templates == nil {
		return domain.CommandTemplate{}, errors.New("命令模板存储未配置")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Command = strings.TrimSpace(input.Command)
	if len(input.Variables) == 0 {
		input.Variables = extractCommandTemplateVariables(input.Command)
	}
	return s.templates.Create(ctx, input, strings.TrimSpace(username))
}

func (s *CommandService) SetTemplateFavorite(ctx context.Context, templateID string, username string, favorite bool) error {
	if s.templates == nil {
		return errors.New("命令模板存储未配置")
	}
	return s.templates.SetFavorite(ctx, templateID, strings.TrimSpace(username), favorite)
}

func normalizeServerIDs(serverIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(serverIDs))
	ids := make([]int64, 0, len(serverIDs))
	for _, id := range serverIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

const maxCommandOutputLength = 16 * 1024

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)((?:password|passwd|pwd)\s*[:=]\s*)([^\s]+)`),
	regexp.MustCompile(`(?im)((?:token|access_token|refresh_token|secret|api[_-]?key)\s*[:=]\s*)([^\s]+)`),
	regexp.MustCompile(`(?im)(authorization\s*:\s*(?:bearer|basic)\s+)([^\s]+)`),
}

func sanitizeCommandOutput(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, pattern := range redactionPatterns {
		trimmed = pattern.ReplaceAllString(trimmed, `${1}[REDACTED]`)
	}
	if len(trimmed) <= maxCommandOutputLength {
		return trimmed
	}
	remaining := len(trimmed) - maxCommandOutputLength
	return fmt.Sprintf("%s\n...[输出已截断，省略 %d 字符]", trimmed[:maxCommandOutputLength], remaining)
}

var commandTemplateVariablePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_-]+)\s*\}\}`)

func extractCommandTemplateVariables(command string) []domain.CommandTemplateVariable {
	matches := commandTemplateVariablePattern.FindAllStringSubmatch(command, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	variables := make([]domain.CommandTemplateVariable, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		variables = append(variables, domain.CommandTemplateVariable{
			Name:     name,
			Label:    name,
			Required: true,
		})
	}
	return variables
}

func defaultCommandTemplates() []domain.CommandTemplate {
	items := []domain.CommandTemplate{
		{
			ID:          "system-disk-usage",
			Name:        "磁盘使用率",
			Description: "检查根分区与挂载点容量",
			Command:     "df -h",
			Scope:       domain.CommandTemplateScopeShared,
			RiskLevel:   domain.CommandTemplateRiskNormal,
		},
		{
			ID:          "system-top-process",
			Name:        "高 CPU 进程",
			Description: "查看 CPU 占用最高的前 10 个进程",
			Command:     "ps -eo pid,ppid,cmd,%cpu,%mem --sort=-%cpu | head -n 11",
			Scope:       domain.CommandTemplateScopeShared,
			RiskLevel:   domain.CommandTemplateRiskNormal,
		},
		{
			ID:          "service-status",
			Name:        "服务状态检查",
			Description: "检查指定 systemd 服务状态",
			Command:     "systemctl status {{service}} --no-pager",
			Scope:       domain.CommandTemplateScopeShared,
			RiskLevel:   domain.CommandTemplateRiskNormal,
			Variables: []domain.CommandTemplateVariable{{
				Name:        "service",
				Label:       "服务名",
				Placeholder: "nginx",
				Required:    true,
			}},
		},
		{
			ID:          "journal-tail",
			Name:        "最近日志",
			Description: "查看指定服务最近 N 行日志",
			Command:     "journalctl -u {{service}} -n {{lines}} --no-pager",
			Scope:       domain.CommandTemplateScopeShared,
			RiskLevel:   domain.CommandTemplateRiskNormal,
			Variables: []domain.CommandTemplateVariable{
				{
					Name:        "service",
					Label:       "服务名",
					Placeholder: "nginx",
					Required:    true,
				},
				{
					Name:         "lines",
					Label:        "日志行数",
					Placeholder:  "200",
					DefaultValue: "200",
					Required:     true,
				},
			},
		},
		{
			ID:          "service-restart",
			Name:        "重启服务",
			Description: "重启指定 systemd 服务，请确认业务影响",
			Command:     "sudo systemctl restart {{service}}",
			Scope:       domain.CommandTemplateScopeShared,
			RiskLevel:   domain.CommandTemplateRiskDangerous,
			Variables: []domain.CommandTemplateVariable{{
				Name:        "service",
				Label:       "服务名",
				Placeholder: "nginx",
				Required:    true,
			}},
		},
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}
