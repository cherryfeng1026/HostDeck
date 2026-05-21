package scheduler

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

const collectorBatchCommand = `sh -c '
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

const collectorBatchOutput = `__HOSTDECK__:cpu
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

type recordingRunner struct {
	mu      sync.Mutex
	targets []sshx.Target
}

func (r *recordingRunner) Run(ctx context.Context, target sshx.Target, command string) (string, string, int, error) {
	r.mu.Lock()
	r.targets = append(r.targets, target)
	r.mu.Unlock()

	switch command {
	case collectorBatchCommand:
		return collectorBatchOutput, "", 0, nil
	default:
		return "", "unexpected command", 127, nil
	}
}

func (r *recordingRunner) Targets() []sshx.Target {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sshx.Target(nil), r.targets...)
}

type disabledServerLister struct {
	servers []domain.Server
}

func (l disabledServerLister) List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error) {
	return l.servers, nil
}

type recordingStatusWriter struct {
	disabledServerIDs []int64
}

func (w *recordingStatusWriter) UpsertLatest(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error {
	return nil
}

func (w *recordingStatusWriter) AppendHistory(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error {
	return nil
}

func (w *recordingStatusWriter) MarkCollectStarted(ctx context.Context, serverID int64, startedAt time.Time) error {
	return nil
}

func (w *recordingStatusWriter) MarkCollectFailure(ctx context.Context, serverID int64, err error, finishedAt time.Time) error {
	return nil
}

func (w *recordingStatusWriter) MarkDisabled(ctx context.Context, serverID int64, finishedAt time.Time) error {
	w.disabledServerIDs = append(w.disabledServerIDs, serverID)
	return nil
}

func (w *recordingStatusWriter) MarkStaleBefore(ctx context.Context, cutoff time.Time) error {
	return nil
}

type recordingDisabledAlertResolver struct {
	resolved []domain.Server
	details  []string
}

func (r *recordingDisabledAlertResolver) EvaluateServerSnapshot(ctx context.Context, server domain.Server, snapshot collector.Snapshot, sampledAt time.Time) error {
	return nil
}

func (r *recordingDisabledAlertResolver) ResolveServerAlerts(ctx context.Context, server domain.Server, detail string, occurredAt time.Time) error {
	r.resolved = append(r.resolved, server)
	r.details = append(r.details, detail)
	return nil
}

func openPollerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func TestPollerCollectOnceUsesSingleBatchTimestamp(t *testing.T) {
	db := openPollerTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	for _, item := range []domain.Server{
		{
			Name:          "prod-web-01",
			Hostname:      "prod-web-01",
			IP:            "10.0.0.21",
			SSHPort:       22,
			Username:      "root",
			AuthType:      "password",
			Password:      "secret-one",
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
			Password:      "secret-two",
			CollectorMode: "ssh_only",
			Enabled:       true,
		},
	} {
		if err := serverRepo.Create(context.Background(), item); err != nil {
			t.Fatalf("create server %s: %v", item.Name, err)
		}
	}

	connectionService := service.NewServerConnectionService(
		serverRepo,
		storage.NewServerCredentialRepository(db),
		"test-master-key",
	)
	poller := NewPoller(
		serverRepo,
		connectionService,
		collector.NewSSHCollector(&recordingRunner{}),
		storage.NewStatusRepository(db),
		nil,
		time.Minute,
		2,
	)

	poller.collectOnce(context.Background())

	rows, err := db.QueryContext(context.Background(), `SELECT DISTINCT sampled_at FROM server_status_history`)
	if err != nil {
		t.Fatalf("query sampled_at values: %v", err)
	}
	defer rows.Close()

	values := make([]string, 0)
	for rows.Next() {
		var sampledAt string
		if err := rows.Scan(&sampledAt); err != nil {
			t.Fatalf("scan sampled_at: %v", err)
		}
		values = append(values, sampledAt)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sampled_at values: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("expected one shared sampled_at value, got %d values: %v", len(values), values)
	}

	fleetPoints, err := storage.NewStatusRepository(db).ListFleetHistory(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("list fleet history: %v", err)
	}
	if len(fleetPoints) != 1 || fleetPoints[0].SampleCount != 2 {
		t.Fatalf("expected one fleet point with sampleCount=2, got %+v", fleetPoints)
	}
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
		nil,
		time.Minute,
		2,
	)

	poller.collectOnce(context.Background())

	targets := runner.Targets()
	if len(targets) == 0 {
		t.Fatal("expected poller to execute ssh commands")
	}
	for _, target := range targets {
		if target.Password != "super-secret" {
			t.Fatalf("expected decrypted password in poller target, got %q", target.Password)
		}
	}
}

func TestPollerCollectOnceResolvesAlertsForDisabledServers(t *testing.T) {
	statuses := &recordingStatusWriter{}
	alerts := &recordingDisabledAlertResolver{}
	poller := NewPoller(
		disabledServerLister{servers: []domain.Server{{ID: 7, Name: "prod-web-01", Enabled: false}}},
		nil,
		nil,
		statuses,
		alerts,
		time.Minute,
		1,
	)

	poller.collectOnce(context.Background())

	if len(statuses.disabledServerIDs) != 1 || statuses.disabledServerIDs[0] != 7 {
		t.Fatalf("expected server 7 to be marked disabled, got %+v", statuses.disabledServerIDs)
	}
	if len(alerts.resolved) != 1 || alerts.resolved[0].ID != 7 {
		t.Fatalf("expected disabled server alerts to resolve, got %+v", alerts.resolved)
	}
	if alerts.details[0] != "server disabled" {
		t.Fatalf("unexpected resolve detail: %+v", alerts.details)
	}
}
