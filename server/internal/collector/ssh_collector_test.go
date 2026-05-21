package collector_test

import (
	"context"
	"testing"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/sshx"
)

const batchCommand = `sh -c '
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

const batchOutput = `__HOSTDECK__:cpu
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

type sshCollectorRunnerStub struct {
	commands []string
}

func (s *sshCollectorRunnerStub) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	s.commands = append(s.commands, command)
	if command != batchCommand {
		return "", "unexpected command", 127, nil
	}
	return batchOutput, "", 0, nil
}

func TestSSHCollectorCollectUsesSingleBatchCommand(t *testing.T) {
	runner := &sshCollectorRunnerStub{}
	c := collector.NewSSHCollector(runner)

	snapshot, err := c.Collect(context.Background(), domain.Server{
		IP:       "10.0.0.21",
		SSHPort:  22,
		Username: "root",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.commands))
	}
	if runner.commands[0] != batchCommand {
		t.Fatalf("unexpected command: %q", runner.commands[0])
	}
	if !snapshot.Online || !snapshot.SSHOK {
		t.Fatalf("expected online snapshot, got %+v", snapshot)
	}
	if snapshot.CPUUsage != 66.66666666666667 || snapshot.MemoryUsage != 50 || snapshot.DiskUsage != 55 || snapshot.Load1 != 0.15 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.CollectDurationMS < 0 {
		t.Fatalf("expected non-negative collect duration, got %d", snapshot.CollectDurationMS)
	}
}
