package scheduler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
)

type recordingRunner struct {
	targets []sshx.Target
}

func (r *recordingRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	r.targets = append(r.targets, target)

	switch command {
	case "sh -c 'cat /proc/stat; sleep 1; cat /proc/stat'":
		return "cpu  100 0 100 900 0 0 0 0 0 0\ncpu  140 0 140 940 0 0 0 0 0 0\n", "", 0, nil
	case "cat /proc/meminfo":
		return "MemTotal: 2048000 kB\nMemAvailable: 1024000 kB\n", "", 0, nil
	case "df -P /":
		return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 100000 55000 45000 55% /\n", "", 0, nil
	case "cat /proc/loadavg":
		return "0.15 0.20 0.25 1/100 12345\n", "", 0, nil
	case "cat /etc/os-release":
		return "PRETTY_NAME=\"Ubuntu 24.04 LTS\"\n", "", 0, nil
	case "uname -r":
		return "6.8.0-31-generic\n", "", 0, nil
	case "cat /proc/uptime":
		return "12345.67 23456.78\n", "", 0, nil
	default:
		return "", "unexpected command", 127, nil
	}
}

func openPollerTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:poller-test?mode=memory&cache=shared")
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

func TestPollerCollectOnceUsesResolvedPasswordTarget(t *testing.T) {
	db := openPollerTestDB(t)
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
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	runner := &recordingRunner{}
	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	poller := NewPoller(
		serverRepo,
		connectionService,
		collector.NewSSHCollector(runner),
		storage.NewStatusRepository(db),
		time.Minute,
		2,
	)

	poller.collectOnce(context.Background())

	if len(runner.targets) == 0 {
		t.Fatal("expected poller to execute ssh commands")
	}
	for _, target := range runner.targets {
		if target.Password != "super-secret" {
			t.Fatalf("expected decrypted password in poller target, got %q", target.Password)
		}
	}
}
