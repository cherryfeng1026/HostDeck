package collector

import (
	"context"
	"fmt"
	"strings"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/sshx"
)

type SSHCollector struct {
	runner sshx.Runner
}

func NewSSHCollector(runner sshx.Runner) *SSHCollector {
	return &SSHCollector{runner: runner}
}

func (c *SSHCollector) Collect(ctx context.Context, server domain.Server) (Snapshot, error) {
	target := sshx.Target{
		Host:     server.IP,
		Port:     server.SSHPort,
		Username: server.Username,
		Password: server.Password,
	}

	cpuRaw, err := c.runCommand(ctx, target, "sh -c 'cat /proc/stat; sleep 1; cat /proc/stat'")
	if err != nil {
		return Snapshot{Online: false, SSHOK: false, Source: "ssh"}, nil
	}
	memRaw, err := c.runCommand(ctx, target, "cat /proc/meminfo")
	if err != nil {
		return Snapshot{Online: false, SSHOK: false, Source: "ssh"}, nil
	}
	diskRaw, err := c.runCommand(ctx, target, "df -P /")
	if err != nil {
		return Snapshot{}, err
	}
	loadRaw, err := c.runCommand(ctx, target, "cat /proc/loadavg")
	if err != nil {
		return Snapshot{}, err
	}
	osReleaseRaw, err := c.runCommand(ctx, target, "cat /etc/os-release")
	if err != nil {
		return Snapshot{}, err
	}
	kernelRaw, err := c.runCommand(ctx, target, "uname -r")
	if err != nil {
		return Snapshot{}, err
	}
	uptimeRaw, err := c.runCommand(ctx, target, "cat /proc/uptime")
	if err != nil {
		return Snapshot{}, err
	}

	cpuUsage, err := parseCPUUsage(cpuRaw)
	if err != nil {
		return Snapshot{}, err
	}
	memoryUsage, err := ParseMemInfo(memRaw)
	if err != nil {
		return Snapshot{}, err
	}
	diskUsage, err := ParseDF(diskRaw)
	if err != nil {
		return Snapshot{}, err
	}
	load1, load5, load15, err := ParseLoadAvg(loadRaw)
	if err != nil {
		return Snapshot{}, err
	}
	uptimeSeconds, err := ParseUptimeSeconds(uptimeRaw)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Online:        true,
		SSHOK:         true,
		CPUUsage:      cpuUsage,
		MemoryUsage:   memoryUsage,
		DiskUsage:     diskUsage,
		OSVersion:     ParseOSRelease(osReleaseRaw),
		KernelVersion: strings.TrimSpace(kernelRaw),
		UptimeSeconds: uptimeSeconds,
		Load1:         load1,
		Load5:         load5,
		Load15:        load15,
		Source:        "ssh",
	}, nil
}

func (c *SSHCollector) runCommand(ctx context.Context, target sshx.Target, command string) (string, error) {
	stdout, stderr, exitCode, err := c.runner.Run(ctx, target, command)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", fmt.Errorf("执行采集命令失败: %s", strings.TrimSpace(stderr))
	}
	return stdout, nil
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
