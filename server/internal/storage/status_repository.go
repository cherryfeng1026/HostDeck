package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"hostdeck/server/internal/collector"
)

type StatusRepository struct {
	db *sql.DB
}

type LatestStatus struct {
	ServerID int64 `json:"serverId"`
	collector.Snapshot
	LastReportAt time.Time `json:"lastReportAt"`
}

type HistoryPoint struct {
	SampledAt   time.Time `json:"sampledAt"`
	CPUUsage    float64   `json:"cpuUsage"`
	MemoryUsage float64   `json:"memoryUsage"`
	DiskUsage   float64   `json:"diskUsage"`
	Load1       float64   `json:"load1"`
	Load5       float64   `json:"load5"`
	Load15      float64   `json:"load15"`
}

func NewStatusRepository(db *sql.DB) *StatusRepository {
	return &StatusRepository{db: db}
}

func (r *StatusRepository) UpsertLatest(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_latest (
			server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
			kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT(server_id) DO UPDATE SET
			online = excluded.online,
			ssh_ok = excluded.ssh_ok,
			cpu_usage = excluded.cpu_usage,
			memory_usage = excluded.memory_usage,
			disk_usage = excluded.disk_usage,
			os_version = excluded.os_version,
			kernel_version = excluded.kernel_version,
			uptime_seconds = excluded.uptime_seconds,
			load_1 = excluded.load_1,
			load_5 = excluded.load_5,
			load_15 = excluded.load_15,
			last_report_at = excluded.last_report_at,
			source = excluded.source`,
		serverID,
		boolToInt(snapshot.Online),
		boolToInt(snapshot.SSHOK),
		snapshot.CPUUsage,
		snapshot.MemoryUsage,
		snapshot.DiskUsage,
		snapshot.OSVersion,
		snapshot.KernelVersion,
		snapshot.UptimeSeconds,
		snapshot.Load1,
		snapshot.Load5,
		snapshot.Load15,
		sampledAt.UTC().Format(time.RFC3339Nano),
		snapshot.Source,
	)
	return err
}

func (r *StatusRepository) AppendHistory(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_history (
			server_id, sampled_at, cpu_usage, memory_usage, disk_usage, load_1, load_5, load_15
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		serverID,
		sampledAt.UTC().Format(time.RFC3339Nano),
		snapshot.CPUUsage,
		snapshot.MemoryUsage,
		snapshot.DiskUsage,
		snapshot.Load1,
		snapshot.Load5,
		snapshot.Load15,
	)
	return err
}

func (r *StatusRepository) GetLatest(ctx context.Context, serverID int64) (LatestStatus, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
		        kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source
		   FROM server_status_latest
		  WHERE server_id = $1`,
		serverID,
	)

	return scanLatestStatus(row)
}

func (r *StatusRepository) ListLatest(ctx context.Context) ([]LatestStatus, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
		        kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source
		   FROM server_status_latest
		  ORDER BY server_id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]LatestStatus, 0)
	for rows.Next() {
		item, err := scanLatestStatus(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *StatusRepository) ListHistory(ctx context.Context, serverID int64, since time.Time) ([]HistoryPoint, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT sampled_at, cpu_usage, memory_usage, disk_usage, load_1, load_5, load_15
		   FROM server_status_history
		  WHERE server_id = $1 AND sampled_at >= $2
		  ORDER BY sampled_at ASC`,
		serverID,
		since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]HistoryPoint, 0)
	for rows.Next() {
		var (
			point     HistoryPoint
			sampledAt string
		)
		if err := rows.Scan(
			&sampledAt,
			&point.CPUUsage,
			&point.MemoryUsage,
			&point.DiskUsage,
			&point.Load1,
			&point.Load5,
			&point.Load15,
		); err != nil {
			return nil, err
		}

		point.SampledAt, err = time.Parse(time.RFC3339Nano, sampledAt)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (r *StatusRepository) DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM server_status_history WHERE sampled_at < $1`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func scanLatestStatus(scanner interface {
	Scan(dest ...any) error
}) (LatestStatus, error) {
	var (
		item         LatestStatus
		online       int
		sshOK        int
		lastReportAt string
	)

	err := scanner.Scan(
		&item.ServerID,
		&online,
		&sshOK,
		&item.CPUUsage,
		&item.MemoryUsage,
		&item.DiskUsage,
		&item.OSVersion,
		&item.KernelVersion,
		&item.UptimeSeconds,
		&item.Load1,
		&item.Load5,
		&item.Load15,
		&lastReportAt,
		&item.Source,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LatestStatus{}, err
		}
		return LatestStatus{}, err
	}

	item.Online = online == 1
	item.SSHOK = sshOK == 1
	item.LastReportAt, err = time.Parse(time.RFC3339Nano, lastReportAt)
	if err != nil {
		return LatestStatus{}, err
	}
	return item, nil
}
