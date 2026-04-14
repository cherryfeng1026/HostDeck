package service

import (
	"context"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

type LiveServerStore interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
}

type LiveStatusStore interface {
	ListLatest(ctx context.Context) ([]storage.LatestStatus, error)
}

type LiveServerItem struct {
	domain.Server
	Online        bool      `json:"online"`
	SSHOK         bool      `json:"sshOk"`
	CPUUsage      float64   `json:"cpuUsage"`
	MemoryUsage   float64   `json:"memoryUsage"`
	DiskUsage     float64   `json:"diskUsage"`
	OSVersion     string    `json:"osVersion"`
	KernelVersion string    `json:"kernelVersion"`
	UptimeSeconds int64     `json:"uptimeSeconds"`
	Load1         float64   `json:"load1"`
	Load5         float64   `json:"load5"`
	Load15        float64   `json:"load15"`
	LastReportAt  time.Time `json:"lastReportAt"`
	Source        string    `json:"source"`
}

type ServerViewService struct {
	servers  LiveServerStore
	statuses LiveStatusStore
}

func NewServerViewService(servers LiveServerStore, statuses LiveStatusStore) *ServerViewService {
	return &ServerViewService{
		servers:  servers,
		statuses: statuses,
	}
}

func (s *ServerViewService) ListLive(ctx context.Context, filter storage.ServerFilter) ([]LiveServerItem, error) {
	servers, err := s.servers.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	latest, err := s.statuses.ListLatest(ctx)
	if err != nil {
		return nil, err
	}

	latestByServerID := make(map[int64]storage.LatestStatus, len(latest))
	for _, item := range latest {
		latestByServerID[item.ServerID] = item
	}

	items := make([]LiveServerItem, 0, len(servers))
	for _, server := range servers {
		item := LiveServerItem{Server: server}
		if status, ok := latestByServerID[server.ID]; ok {
			item.Online = status.Online
			item.SSHOK = status.SSHOK
			item.CPUUsage = status.CPUUsage
			item.MemoryUsage = status.MemoryUsage
			item.DiskUsage = status.DiskUsage
			item.OSVersion = status.OSVersion
			item.KernelVersion = status.KernelVersion
			item.UptimeSeconds = status.UptimeSeconds
			item.Load1 = status.Load1
			item.Load5 = status.Load5
			item.Load15 = status.Load15
			item.LastReportAt = status.LastReportAt
			item.Source = status.Source
		}
		items = append(items, item)
	}

	return items, nil
}
