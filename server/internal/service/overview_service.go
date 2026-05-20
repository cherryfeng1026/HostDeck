package service

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

const dashboardTopServerCap = 5

type OverviewStore interface {
	List(ctx context.Context, filter storage.ServerFilter) ([]domain.Server, error)
}

type OverviewStatusStore interface {
	ListLatest(ctx context.Context) ([]storage.LatestStatus, error)
	GetLatest(ctx context.Context, serverID int64) (storage.LatestStatus, error)
	ListHistory(ctx context.Context, serverID int64, since time.Time) ([]storage.HistoryPoint, error)
	ListFleetHistory(ctx context.Context, since time.Time) ([]storage.FleetHistoryPoint, error)
}

type Overview struct {
	TotalServers         int `json:"totalServers"`
	OnlineServers        int `json:"onlineServers"`
	OfflineServers       int `json:"offlineServers"`
	ActiveAlerts         int `json:"activeAlerts"`
	SSHFailures          int `json:"sshFailures"`
	CollectFailedServers int `json:"collectFailedServers"`
	CollectStaleServers  int `json:"collectStaleServers"`
}

type DashboardHeadline struct {
	TotalServers         int       `json:"totalServers"`
	OnlineServers        int       `json:"onlineServers"`
	OfflineServers       int       `json:"offlineServers"`
	ActiveAlerts         int       `json:"activeAlerts"`
	SSHFailures          int       `json:"sshFailures"`
	CollectFailedServers int       `json:"collectFailedServers"`
	CollectStaleServers  int       `json:"collectStaleServers"`
	LastUpdatedAt        time.Time `json:"lastUpdatedAt"`
}

type DashboardTrendPoint struct {
	SampledAt      time.Time `json:"sampledAt"`
	AvgCPUUsage    float64   `json:"avgCpuUsage"`
	AvgMemoryUsage float64   `json:"avgMemoryUsage"`
	AvgDiskUsage   float64   `json:"avgDiskUsage"`
	AvgLoad1       float64   `json:"avgLoad1"`
	AvgLoad5       float64   `json:"avgLoad5"`
	AvgLoad15      float64   `json:"avgLoad15"`
	SampleCount    int       `json:"sampleCount"`
	Fallback       bool      `json:"fallback"`
}

type DashboardTopServer struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Hostname     string    `json:"hostname"`
	IP           string    `json:"ip"`
	Purpose      string    `json:"purpose"`
	Online       bool      `json:"online"`
	SSHOK        bool      `json:"sshOk"`
	CPUUsage     float64   `json:"cpuUsage"`
	MemoryUsage  float64   `json:"memoryUsage"`
	DiskUsage    float64   `json:"diskUsage"`
	Load1        float64   `json:"load1"`
	LastReportAt time.Time `json:"lastReportAt"`
	RankReason   string    `json:"rankReason"`
}

type DashboardResourceSummary struct {
	ReportingServers     int     `json:"reportingServers"`
	UnhealthyServers     int     `json:"unhealthyServers"`
	CollectFailedServers int     `json:"collectFailedServers"`
	CollectStaleServers  int     `json:"collectStaleServers"`
	AvgCPUUsage          float64 `json:"avgCpuUsage"`
	AvgMemoryUsage       float64 `json:"avgMemoryUsage"`
	AvgDiskUsage         float64 `json:"avgDiskUsage"`
	PeakCPUUsage         float64 `json:"peakCpuUsage"`
	PeakMemoryUsage      float64 `json:"peakMemoryUsage"`
	PeakDiskUsage        float64 `json:"peakDiskUsage"`
}

type DashboardAlertSummary struct {
	Total        int `json:"total"`
	Critical     int `json:"critical"`
	Warning      int `json:"warning"`
	Acknowledged int `json:"acknowledged"`
	Muted        int `json:"muted"`
}

type DashboardOverview struct {
	Headline        DashboardHeadline        `json:"headline"`
	Trends          []DashboardTrendPoint    `json:"trends"`
	TopServers      []DashboardTopServer     `json:"topServers"`
	ResourceSummary DashboardResourceSummary `json:"resourceSummary"`
	AlertSummary    DashboardAlertSummary    `json:"alertSummary"`
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
	CollectStatus             string    `json:"collectStatus"`
	LastCollectStartedAt      time.Time `json:"lastCollectStartedAt"`
	LastCollectFinishedAt     time.Time `json:"lastCollectFinishedAt"`
	LastSuccessAt             time.Time `json:"lastSuccessAt"`
	LastCollectError          string    `json:"lastCollectError"`
	CollectFailureCount       int       `json:"collectFailureCount"`
	CollectDurationMS         int64     `json:"collectDurationMs"`
	Stale                     bool      `json:"stale"`
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
		overview.ActiveAlerts = countAlertsByStatus(alerts, domain.AlertStatusActive)
	}
	for _, item := range latest {
		if item.Online {
			overview.OnlineServers++
			overview.OfflineServers--
		}
		if !item.SSHOK {
			overview.SSHFailures++
		}
		if item.CollectStatus == storage.CollectStatusFailed {
			overview.CollectFailedServers++
		}
		if item.Stale || item.CollectStatus == storage.CollectStatusStale {
			overview.CollectStaleServers++
		}
	}
	return overview, nil
}

func (s *OverviewService) GetDashboardOverview(ctx context.Context, since time.Time) (DashboardOverview, error) {
	servers, err := s.servers.List(ctx, storage.ServerFilter{})
	if err != nil {
		return DashboardOverview{}, err
	}
	latest, err := s.statuses.ListLatest(ctx)
	if err != nil {
		return DashboardOverview{}, err
	}
	fleetHistory, err := s.statuses.ListFleetHistory(ctx, since)
	if err != nil {
		return DashboardOverview{}, err
	}

	alerts := make([]domain.AlertEvent, 0)
	if s.alerts != nil {
		alerts, err = s.alerts.ListCurrentAlerts(ctx)
		if err != nil {
			return DashboardOverview{}, err
		}
	}

	trends := buildDashboardTrendPoints(fleetHistory)
	if len(trends) == 0 {
		trends = buildDashboardFallbackTrendPoints(latest)
	}

	return DashboardOverview{
		Headline:        buildDashboardHeadline(servers, latest, alerts),
		Trends:          trends,
		TopServers:      buildDashboardTopServers(servers, latest),
		ResourceSummary: buildDashboardResourceSummary(latest),
		AlertSummary:    buildDashboardAlertSummary(alerts),
	}, nil
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
		CollectStatus:             latest.CollectStatus,
		LastCollectStartedAt:      latest.LastCollectStartedAt,
		LastCollectFinishedAt:     latest.LastCollectFinishedAt,
		LastSuccessAt:             latest.LastSuccessAt,
		LastCollectError:          latest.LastCollectError,
		CollectFailureCount:       latest.CollectFailureCount,
		CollectDurationMS:         latest.CollectDurationMS,
		Stale:                     latest.Stale,
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

func buildDashboardHeadline(servers []domain.Server, latest []storage.LatestStatus, alerts []domain.AlertEvent) DashboardHeadline {
	headline := DashboardHeadline{
		TotalServers:   len(servers),
		OfflineServers: len(servers),
		ActiveAlerts:   countAlertsByStatus(alerts, domain.AlertStatusActive),
	}
	for _, item := range latest {
		if item.Online {
			headline.OnlineServers++
			headline.OfflineServers--
		}
		if !item.SSHOK {
			headline.SSHFailures++
		}
		if item.CollectStatus == storage.CollectStatusFailed {
			headline.CollectFailedServers++
		}
		if item.Stale || item.CollectStatus == storage.CollectStatusStale {
			headline.CollectStaleServers++
		}
		if item.LastSuccessAt.After(headline.LastUpdatedAt) {
			headline.LastUpdatedAt = item.LastSuccessAt
		}
	}
	return headline
}

func buildDashboardTrendPoints(points []storage.FleetHistoryPoint) []DashboardTrendPoint {
	result := make([]DashboardTrendPoint, 0, len(points))
	for _, point := range points {
		result = append(result, DashboardTrendPoint{
			SampledAt:      point.SampledAt,
			AvgCPUUsage:    point.AvgCPUUsage,
			AvgMemoryUsage: point.AvgMemoryUsage,
			AvgDiskUsage:   point.AvgDiskUsage,
			AvgLoad1:       point.AvgLoad1,
			AvgLoad5:       point.AvgLoad5,
			AvgLoad15:      point.AvgLoad15,
			SampleCount:    point.SampleCount,
		})
	}
	return result
}

func buildDashboardFallbackTrendPoints(latest []storage.LatestStatus) []DashboardTrendPoint {
	point := DashboardTrendPoint{}
	for _, item := range latest {
		if !hasUsableResourceSnapshot(item) {
			continue
		}
		if item.LastReportAt.After(point.SampledAt) {
			point.SampledAt = item.LastReportAt
		}
		point.AvgCPUUsage += item.CPUUsage
		point.AvgMemoryUsage += item.MemoryUsage
		point.AvgDiskUsage += item.DiskUsage
		point.AvgLoad1 += item.Load1
		point.AvgLoad5 += item.Load5
		point.AvgLoad15 += item.Load15
		point.SampleCount++
	}
	if point.SampleCount == 0 {
		return []DashboardTrendPoint{}
	}

	count := float64(point.SampleCount)
	point.AvgCPUUsage /= count
	point.AvgMemoryUsage /= count
	point.AvgDiskUsage /= count
	point.AvgLoad1 /= count
	point.AvgLoad5 /= count
	point.AvgLoad15 /= count
	point.Fallback = true
	return []DashboardTrendPoint{point}
}

func buildDashboardTopServers(servers []domain.Server, latest []storage.LatestStatus) []DashboardTopServer {
	latestByServerID := make(map[int64]storage.LatestStatus, len(latest))
	for _, item := range latest {
		latestByServerID[item.ServerID] = item
	}

	type rankedServer struct {
		item  DashboardTopServer
		score float64
	}

	ranked := make([]rankedServer, 0, len(latestByServerID))
	for _, server := range servers {
		status, ok := latestByServerID[server.ID]
		if !ok {
			continue
		}
		ranked = append(ranked, rankedServer{
			item: DashboardTopServer{
				ID:           server.ID,
				Name:         server.Name,
				Hostname:     server.Hostname,
				IP:           server.IP,
				Purpose:      server.Purpose,
				Online:       status.Online,
				SSHOK:        status.SSHOK,
				CPUUsage:     status.CPUUsage,
				MemoryUsage:  status.MemoryUsage,
				DiskUsage:    status.DiskUsage,
				Load1:        status.Load1,
				LastReportAt: status.LastReportAt,
				RankReason:   rankReason(status),
			},
			score: rankScore(status),
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].item.LastReportAt.After(ranked[j].item.LastReportAt)
		}
		return ranked[i].score > ranked[j].score
	})

	if len(ranked) > dashboardTopServerCap {
		ranked = ranked[:dashboardTopServerCap]
	}
	items := make([]DashboardTopServer, 0, len(ranked))
	for _, item := range ranked {
		items = append(items, item.item)
	}
	return items
}

func buildDashboardResourceSummary(latest []storage.LatestStatus) DashboardResourceSummary {
	if len(latest) == 0 {
		return DashboardResourceSummary{}
	}
	summary := DashboardResourceSummary{}
	for _, item := range latest {
		if item.CollectStatus == storage.CollectStatusFailed {
			summary.CollectFailedServers++
		}
		if item.Stale || item.CollectStatus == storage.CollectStatusStale {
			summary.CollectStaleServers++
		}
		if !item.Online || !item.SSHOK || item.Stale || item.CPUUsage >= 85 || item.MemoryUsage >= 85 || item.DiskUsage >= 90 {
			summary.UnhealthyServers++
		}
		if !hasUsableResourceSnapshot(item) {
			continue
		}
		summary.ReportingServers++
		summary.AvgCPUUsage += item.CPUUsage
		summary.AvgMemoryUsage += item.MemoryUsage
		summary.AvgDiskUsage += item.DiskUsage
		if item.CPUUsage > summary.PeakCPUUsage {
			summary.PeakCPUUsage = item.CPUUsage
		}
		if item.MemoryUsage > summary.PeakMemoryUsage {
			summary.PeakMemoryUsage = item.MemoryUsage
		}
		if item.DiskUsage > summary.PeakDiskUsage {
			summary.PeakDiskUsage = item.DiskUsage
		}
	}
	if summary.ReportingServers == 0 {
		return summary
	}
	count := float64(summary.ReportingServers)
	summary.AvgCPUUsage /= count
	summary.AvgMemoryUsage /= count
	summary.AvgDiskUsage /= count
	return summary
}

func hasUsableResourceSnapshot(item storage.LatestStatus) bool {
	return !item.LastReportAt.IsZero() && (item.CPUUsage != 0 || item.MemoryUsage != 0 || item.DiskUsage != 0)
}

func buildDashboardAlertSummary(alerts []domain.AlertEvent) DashboardAlertSummary {
	summary := DashboardAlertSummary{Total: len(alerts)}
	for _, alert := range alerts {
		switch strings.ToLower(strings.TrimSpace(alert.Severity)) {
		case "critical":
			summary.Critical++
		case "warning":
			summary.Warning++
		}
		switch strings.ToLower(strings.TrimSpace(alert.Status)) {
		case domain.AlertStatusAcknowledged:
			summary.Acknowledged++
		case domain.AlertStatusMuted:
			summary.Muted++
		}
	}
	return summary
}

func countAlertsByStatus(alerts []domain.AlertEvent, status string) int {
	count := 0
	for _, alert := range alerts {
		if strings.EqualFold(strings.TrimSpace(alert.Status), status) {
			count++
		}
	}
	return count
}

func rankScore(item storage.LatestStatus) float64 {
	score := item.CPUUsage + item.MemoryUsage + item.DiskUsage + (item.Load1 * 20)
	if !item.SSHOK {
		score += 150
	}
	if !item.Online {
		score += 300
	}
	return score
}

func rankReason(item storage.LatestStatus) string {
	if !item.Online {
		return "节点离线"
	}
	if !item.SSHOK {
		return "SSH 异常"
	}
	if item.CPUUsage >= item.MemoryUsage && item.CPUUsage >= item.DiskUsage {
		return "CPU 压力最高"
	}
	if item.MemoryUsage >= item.DiskUsage {
		return "内存占用偏高"
	}
	return "磁盘占用偏高"
}
