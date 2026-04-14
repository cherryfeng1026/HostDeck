package service

import (
	"context"
	"errors"
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
	servers CommandServerResolver
	runner  sshx.Runner
	logs    CommandLogStore
}

func NewCommandService(servers CommandServerResolver, runner sshx.Runner, logs CommandLogStore) *CommandService {
	return &CommandService{
		servers: servers,
		runner:  runner,
		logs:    logs,
	}
}

func (s *CommandService) Execute(ctx context.Context, serverID int64, command string, timeout time.Duration) (CommandResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return CommandResult{}, errors.New("命令不能为空")
	}

	server, err := s.servers.ResolveServer(ctx, serverID)
	if err != nil {
		return CommandResult{}, err
	}

	return s.executeOnServer(ctx, server, command, timeout)
}

func (s *CommandService) ExecuteBatch(ctx context.Context, serverIDs []int64, command string, timeout time.Duration) ([]BatchCommandResult, error) {
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

			result, execErr := s.executeOnServer(ctx, server, command, timeout)
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

func (s *CommandService) executeOnServer(ctx context.Context, server domain.Server, command string, timeout time.Duration) (CommandResult, error) {
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	startedAt := time.Now()
	stdout, stderr, exitCode, err := s.runner.Run(runCtx, sshx.Target{
		Host:     server.IP,
		Port:     server.SSHPort,
		Username: server.Username,
		Password: server.Password,
		Timeout:  timeout,
	}, command)
	if err != nil {
		return CommandResult{}, err
	}

	result := CommandResult{
		Command:    command,
		Stdout:     stdout,
		Stderr:     stderr,
		ExitCode:   exitCode,
		DurationMS: time.Since(startedAt).Milliseconds(),
		ExecutedAt: time.Now(),
	}
	if err := s.logs.Create(ctx, domain.CommandLog{
		ServerID:   server.ID,
		Command:    result.Command,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		ExitCode:   result.ExitCode,
		DurationMS: result.DurationMS,
		ExecutedAt: result.ExecutedAt,
	}); err != nil {
		return CommandResult{}, err
	}

	return result, nil
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
