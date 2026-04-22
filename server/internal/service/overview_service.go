package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

type OverviewStore interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
}

type OverviewStatusStore interface {
	ListLatest(ctx context.Context) ([]storage.LatestStatus, error)
	GetLatest(ctx context.Context, serverID int64) (storage.LatestStatus, error)
	ListHistory(ctx context.Context, serverID int64, since time.Time) ([]storage.HistoryPoint, error)
}

type Overview struct {
	TotalServers   int `json:"totalServers"`
	OnlineServers  int `json:"onlineServers"`
	OfflineServers int `json:"offlineServers"`
	ActiveAlerts   int `json:"activeAlerts"`
	SSHFailures    int `json:"sshFailures"`
}

type ServerStatusDetail struct {
	ID                        int64     `json:"id"`
	Name                      string    `json:"name"`
	Hostname                  string    `json:"hostname"`
	IP                        string    `json:"ip"`
	SSHPort                   int       `json:"sshPort"`
	Username                  string    `json:"username"`
	CollectorMode             string    `json:"collectorMode"`
	TrustedHostKeyFingerprint string    `json:"trustedHostKeyFingerprint"`
	Enabled                   bool      `json:"enabled"`
	Online                    bool      `json:"online"`
	SSHOK                     bool      `json:"sshOk"`
	CPUUsage                  float64   `json:"cpuUsage"`
	MemoryUsage               float64   `json:"memoryUsage"`
	DiskUsage                 float64   `json:"diskUsage"`
	OSVersion                 string    `json:"osVersion"`
	KernelVersion             string    `json:"kernelVersion"`
	UptimeSeconds             int64     `json:"uptimeSeconds"`
	Load1                     float64   `json:"load1"`
	Load5                     float64   `json:"load5"`
	Load15                    float64   `json:"load15"`
	LastReportAt              time.Time `json:"lastReportAt"`
	Source                    string    `json:"source"`
}

type OverviewAlertReader interface {
	ListCurrentAlerts(ctx context.Context) ([]domain.AlertEvent, error)
}

type OverviewService struct {
	servers  OverviewStore
	statuses OverviewStatusStore
	alerts   OverviewAlertReader
}

func NewOverviewService(servers OverviewStore, statuses OverviewStatusStore, alerts OverviewAlertReader) *OverviewService {
	return &OverviewService{
		servers:  servers,
		statuses: statuses,
		alerts:   alerts,
	}
}

func (s *OverviewService) GetOverview(ctx context.Context) (Overview, error) {
	servers, err := s.servers.List(ctx, storage.ServerFilter{})
	if err != nil {
		return Overview{}, err
	}
	latest, err := s.statuses.ListLatest(ctx)
	if err != nil {
		return Overview{}, err
	}

	overview := Overview{
		TotalServers:   len(servers),
		OfflineServers: len(servers),
	}
	if s.alerts != nil {
		alerts, err := s.alerts.ListCurrentAlerts(ctx)
		if err != nil {
			return Overview{}, err
		}
		overview.ActiveAlerts = len(alerts)
	}
	for _, item := range latest {
		if item.Online {
			overview.OnlineServers++
			overview.OfflineServers--
		}
		if !item.SSHOK {
			overview.SSHFailures++
		}
	}
	return overview, nil
}

func (s *OverviewService) GetServerStatus(ctx context.Context, serverID int64) (ServerStatusDetail, error) {
	servers, err := s.servers.List(ctx, storage.ServerFilter{ID: serverID})
	if err != nil {
		return ServerStatusDetail{}, err
	}
	if len(servers) == 0 {
		return ServerStatusDetail{}, errors.New("服务器不存在")
	}

	latest, err := s.statuses.GetLatest(ctx, serverID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ServerStatusDetail{}, err
	}

	server := servers[0]
	return ServerStatusDetail{
		ID:                        server.ID,
		Name:                      server.Name,
		Hostname:                  server.Hostname,
		IP:                        server.IP,
		SSHPort:                   server.SSHPort,
		Username:                  server.Username,
		CollectorMode:             server.CollectorMode,
		TrustedHostKeyFingerprint: server.TrustedHostKeyFingerprint,
		Enabled:                   server.Enabled,
		Online:                    latest.Online,
		SSHOK:                     latest.SSHOK,
		CPUUsage:                  latest.CPUUsage,
		MemoryUsage:               latest.MemoryUsage,
		DiskUsage:                 latest.DiskUsage,
		OSVersion:                 latest.OSVersion,
		KernelVersion:             latest.KernelVersion,
		UptimeSeconds:             latest.UptimeSeconds,
		Load1:                     latest.Load1,
		Load5:                     latest.Load5,
		Load15:                    latest.Load15,
		LastReportAt:              latest.LastReportAt,
		Source:                    latest.Source,
	}, nil
}

func (s *OverviewService) GetServerMetrics(ctx context.Context, serverID int64, since time.Time) ([]storage.HistoryPoint, error) {
	servers, err := s.servers.List(ctx, storage.ServerFilter{ID: serverID})
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, errors.New("服务器不存在")
	}
	return s.statuses.ListHistory(ctx, serverID, since)
}
