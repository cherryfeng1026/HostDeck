package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/sshx"
)

const (
	collectSectionCPU       = "cpu"
	collectSectionMemInfo   = "meminfo"
	collectSectionDisk      = "disk"
	collectSectionLoad      = "loadavg"
	collectSectionOSRelease = "osrelease"
	collectSectionKernel    = "kernel"
	collectSectionUptime    = "uptime"
)

const batchCollectCommand = `sh -c '
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

type collectCommandFailedError struct {
	message string
}

func (e collectCommandFailedError) Error() string {
	return e.message
}

type SSHCollector struct {
	runner sshx.Runner
}

func NewSSHCollector(runner sshx.Runner) *SSHCollector {
	return &SSHCollector{runner: runner}
}

func (c *SSHCollector) Collect(ctx context.Context, server domain.Server) (Snapshot, error) {
	target := sshx.Target{
		Host:                      server.IP,
		Port:                      server.SSHPort,
		Username:                  server.Username,
		Password:                  server.Password,
		PrivateKeyPEM:             server.PrivateKey,
		TrustedHostKeyFingerprint: server.TrustedHostKeyFingerprint,
	}

	startedAt := time.Now()
	raw, err := c.runCommand(ctx, target, batchCollectCommand)
	if err != nil {
		if isHostKeyTrustError(err) {
			return Snapshot{}, err
		}
		var commandFailedErr collectCommandFailedError
		if errors.As(err, &commandFailedErr) {
			return Snapshot{}, err
		}
		return Snapshot{Online: false, SSHOK: false, Source: "ssh"}, nil
	}

	sections, err := parseBatchCollectOutput(raw)
	if err != nil {
		return Snapshot{}, err
	}
	cpuUsage, err := parseCPUUsage(sections[collectSectionCPU])
	if err != nil {
		return Snapshot{}, err
	}
	memoryUsage, err := ParseMemInfo(sections[collectSectionMemInfo])
	if err != nil {
		return Snapshot{}, err
	}
	diskUsage, err := ParseDF(sections[collectSectionDisk])
	if err != nil {
		return Snapshot{}, err
	}
	load1, load5, load15, err := ParseLoadAvg(sections[collectSectionLoad])
	if err != nil {
		return Snapshot{}, err
	}
	uptimeSeconds, err := ParseUptimeSeconds(sections[collectSectionUptime])
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Online:            true,
		SSHOK:             true,
		CPUUsage:          cpuUsage,
		MemoryUsage:       memoryUsage,
		DiskUsage:         diskUsage,
		OSVersion:         ParseOSRelease(sections[collectSectionOSRelease]),
		KernelVersion:     strings.TrimSpace(sections[collectSectionKernel]),
		UptimeSeconds:     uptimeSeconds,
		Load1:             load1,
		Load5:             load5,
		Load15:            load15,
		CollectDurationMS: time.Since(startedAt).Milliseconds(),
		Source:            "ssh",
	}, nil
}

func (c *SSHCollector) runCommand(ctx context.Context, target sshx.Target, command string) (string, error) {
	stdout, stderr, exitCode, err := c.runner.Run(ctx, target, command)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", collectCommandFailedError{message: fmt.Sprintf("执行采集命令失败: %s", strings.TrimSpace(stderr))}
	}
	return stdout, nil
}

func isHostKeyTrustError(err error) bool {
	var trustRequired sshx.HostKeyTrustRequiredError
	if errors.As(err, &trustRequired) {
		return true
	}
	var mismatch sshx.HostKeyMismatchError
	return errors.As(err, &mismatch)
}

func parseBatchCollectOutput(raw string) (map[string]string, error) {
	sections := make(map[string][]string)
	current := ""
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "__HOSTDECK__:") {
			current = strings.TrimSpace(strings.TrimPrefix(line, "__HOSTDECK__:"))
			if current == "" {
				return nil, fmt.Errorf("采集输出分段标记为空")
			}
			if _, ok := sections[current]; !ok {
				sections[current] = nil
			}
			continue
		}
		if current == "" {
			if strings.TrimSpace(line) == "" {
				continue
			}
			return nil, fmt.Errorf("采集输出缺少分段标记")
		}
		sections[current] = append(sections[current], line)
	}

	result := make(map[string]string, 7)
	for _, name := range []string{
		collectSectionCPU,
		collectSectionMemInfo,
		collectSectionDisk,
		collectSectionLoad,
		collectSectionOSRelease,
		collectSectionKernel,
		collectSectionUptime,
	} {
		lines, ok := sections[name]
		if !ok || len(lines) == 0 {
			return nil, fmt.Errorf("采集输出缺少 %s 分段", name)
		}
		result[name] = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return result, nil
}

func parseCPUUsage(raw string) (float64, error) {
	samples := make([]CPUStatSample, 0, 2)
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "cpu ") {
			continue
		}
		sample, err := ParseCPUStat(trimmed)
		if err != nil {
			return 0, err
		}
		samples = append(samples, sample)
		if len(samples) == 2 {
			break
		}
	}
	if len(samples) < 2 {
		return 0, fmt.Errorf("CPU 统计输出不完整")
	}
	return CalculateCPUUsage(samples[0], samples[1])
}
