package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

type overviewResponse struct {
	TotalServers   int `json:"totalServers"`
	OnlineServers  int `json:"onlineServers"`
	OfflineServers int `json:"offlineServers"`
	ActiveAlerts   int `json:"activeAlerts"`
	SSHFailures    int `json:"sshFailures"`
}

type dashboardOverviewResponse struct {
	Headline struct {
		TotalServers   int    `json:"totalServers"`
		OnlineServers  int    `json:"onlineServers"`
		OfflineServers int    `json:"offlineServers"`
		ActiveAlerts   int    `json:"activeAlerts"`
		SSHFailures    int    `json:"sshFailures"`
		LastUpdatedAt  string `json:"lastUpdatedAt"`
	} `json:"headline"`
	Trends []struct {
		SampledAt      string  `json:"sampledAt"`
		AvgCPUUsage    float64 `json:"avgCpuUsage"`
		AvgMemoryUsage float64 `json:"avgMemoryUsage"`
		AvgDiskUsage   float64 `json:"avgDiskUsage"`
		AvgLoad1       float64 `json:"avgLoad1"`
		AvgLoad5       float64 `json:"avgLoad5"`
		AvgLoad15      float64 `json:"avgLoad15"`
		SampleCount    int     `json:"sampleCount"`
	} `json:"trends"`
	TopServers []struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Online      bool    `json:"online"`
		SSHOK       bool    `json:"sshOk"`
		CPUUsage    float64 `json:"cpuUsage"`
		MemoryUsage float64 `json:"memoryUsage"`
		DiskUsage   float64 `json:"diskUsage"`
		RankReason  string  `json:"rankReason"`
	} `json:"topServers"`
	ResourceSummary struct {
		ReportingServers int     `json:"reportingServers"`
		UnhealthyServers int     `json:"unhealthyServers"`
		AvgCPUUsage      float64 `json:"avgCpuUsage"`
		AvgMemoryUsage   float64 `json:"avgMemoryUsage"`
		AvgDiskUsage     float64 `json:"avgDiskUsage"`
		PeakCPUUsage     float64 `json:"peakCpuUsage"`
		PeakMemoryUsage  float64 `json:"peakMemoryUsage"`
		PeakDiskUsage    float64 `json:"peakDiskUsage"`
	} `json:"resourceSummary"`
	AlertSummary struct {
		Total        int `json:"total"`
		Critical     int `json:"critical"`
		Warning      int `json:"warning"`
		Acknowledged int `json:"acknowledged"`
		Muted        int `json:"muted"`
	} `json:"alertSummary"`
}

type serverStatusResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Enabled     bool    `json:"enabled"`
	Online      bool    `json:"online"`
	SSHOK       bool    `json:"sshOk"`
	MemoryUsage float64 `json:"memoryUsage"`
	OSVersion   string  `json:"osVersion"`
}

type metricsResponse struct {
	Points []struct {
		SampledAt   string  `json:"sampledAt"`
		CPUUsage    float64 `json:"cpuUsage"`
		MemoryUsage float64 `json:"memoryUsage"`
		DiskUsage   float64 `json:"diskUsage"`
	} `json:"points"`
}

func openOverviewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testsupport.OpenPostgresTestDB(t)
}

func seedOverviewData(t *testing.T, serverRepo *storage.ServerRepository, statusRepo *storage.StatusRepository) {
	t.Helper()

	servers := []domain.Server{
		{
			Name:          "prod-web-01",
			Hostname:      "prod-web-01",
			IP:            "10.0.0.21",
			SSHPort:       22,
			Username:      "root",
			CollectorMode: "ssh_only",
			Purpose:       "gateway",
			Enabled:       true,
		},
		{
			Name:          "prod-db-01",
			Hostname:      "prod-db-01",
			IP:            "10.0.0.31",
			SSHPort:       22,
			Username:      "postgres",
			CollectorMode: "ssh_only",
			Purpose:       "database",
			Enabled:       true,
		},
	}
	for _, server := range servers {
		if err := serverRepo.Create(context.Background(), server); err != nil {
			t.Fatalf("create server: %v", err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	first := now.Add(-30 * time.Minute)
	second := now.Add(-5 * time.Minute)

	firstSnapshots := []collector.Snapshot{
		{
			Online:        true,
			SSHOK:         true,
			CPUUsage:      20,
			MemoryUsage:   45,
			DiskUsage:     55,
			OSVersion:     "Ubuntu 24.04 LTS",
			KernelVersion: "6.8.0-31-generic",
			UptimeSeconds: 12345,
			Load1:         0.11,
			Load5:         0.21,
			Load15:        0.31,
			Source:        "ssh",
		},
		{
			Online:        true,
			SSHOK:         true,
			CPUUsage:      72,
			MemoryUsage:   64,
			DiskUsage:     78,
			OSVersion:     "Debian 12",
			KernelVersion: "6.1.0-26-amd64",
			UptimeSeconds: 22345,
			Load1:         0.91,
			Load5:         0.81,
			Load15:        0.71,
			Source:        "ssh",
		},
	}
	secondSnapshots := []collector.Snapshot{
		{
			Online:        true,
			SSHOK:         true,
			CPUUsage:      28,
			MemoryUsage:   50,
			DiskUsage:     57,
			OSVersion:     "Ubuntu 24.04 LTS",
			KernelVersion: "6.8.0-31-generic",
			UptimeSeconds: 23456,
			Load1:         0.19,
			Load5:         0.25,
			Load15:        0.35,
			Source:        "ssh",
		},
		{
			Online:        false,
			SSHOK:         false,
			CPUUsage:      84,
			MemoryUsage:   91,
			DiskUsage:     88,
			OSVersion:     "Debian 12",
			KernelVersion: "6.1.0-26-amd64",
			UptimeSeconds: 24567,
			Load1:         1.42,
			Load5:         1.23,
			Load15:        0.97,
			Source:        "ssh",
		},
	}

	for serverID, snapshot := range firstSnapshots {
		if err := statusRepo.AppendHistory(context.Background(), int64(serverID+1), snapshot, first); err != nil {
			t.Fatalf("append first history: %v", err)
		}
	}
	for serverID, snapshot := range secondSnapshots {
		id := int64(serverID + 1)
		if err := statusRepo.UpsertLatest(context.Background(), id, snapshot, second); err != nil {
			t.Fatalf("upsert latest: %v", err)
		}
		if err := statusRepo.AppendHistory(context.Background(), id, snapshot, second); err != nil {
			t.Fatalf("append second history: %v", err)
		}
	}
}

func TestOverviewRoutes_ReturnOverviewStatusAndMetrics(t *testing.T) {
	db := openOverviewTestDB(t)
	serverRepo := storage.NewServerRepository(db)
	statusRepo := storage.NewStatusRepository(db)
	seedOverviewData(t, serverRepo, statusRepo)

	alertRepo := storage.NewAlertRepository(db)
	alertService := service.NewAlertService(alertRepo, alertRepo, serverRepo, alertRepo)
	svc := service.NewOverviewService(serverRepo, statusRepo, alertService)
	serverViewService := service.NewServerViewService(serverRepo, statusRepo)
	router := httpx.NewRouterWithHandlers(
		api.NewServerHandler(serverRepo, serverViewService),
		nil,
		api.NewOverviewHandler(svc),
		api.NewServerDetailHandler(svc),
		nil,
		api.NewAlertHandler(alertService),
	)

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	overviewRec := httptest.NewRecorder()
	router.ServeHTTP(overviewRec, overviewReq)
	if overviewRec.Code != http.StatusOK {
		t.Fatalf("expected overview status %d, got %d", http.StatusOK, overviewRec.Code)
	}

	var overview overviewResponse
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.TotalServers != 2 || overview.OnlineServers != 1 || overview.OfflineServers != 1 || overview.SSHFailures != 1 || overview.ActiveAlerts != 0 {
		t.Fatalf("unexpected overview response: %+v", overview)
	}

	createAlertRuleReq := httptest.NewRequest(
		http.MethodPost,
		"/api/alert-rules",
		strings.NewReader(`{"metric":"memory_usage","operator":"gte","threshold":40,"durationSeconds":60,"enabled":true}`),
	)
	createAlertRuleReq.Header.Set("Content-Type", "application/json")
	createAlertRuleRec := httptest.NewRecorder()
	router.ServeHTTP(createAlertRuleRec, createAlertRuleReq)
	if createAlertRuleRec.Code != http.StatusCreated {
		t.Fatalf("expected alert rule create status %d, got %d", http.StatusCreated, createAlertRuleRec.Code)
	}

	overviewReq = httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	overviewRec = httptest.NewRecorder()
	router.ServeHTTP(overviewRec, overviewReq)
	if overviewRec.Code != http.StatusOK {
		t.Fatalf("expected overview status %d after alert rule, got %d", http.StatusOK, overviewRec.Code)
	}
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview after alert rule: %v", err)
	}
	if overview.ActiveAlerts != 2 {
		t.Fatalf("expected activeAlerts=2, got %+v", overview)
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/api/overview/dashboard", nil)
	dashboardRec := httptest.NewRecorder()
	router.ServeHTTP(dashboardRec, dashboardReq)
	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard overview status %d, got %d", http.StatusOK, dashboardRec.Code)
	}

	var dashboard dashboardOverviewResponse
	if err := json.NewDecoder(dashboardRec.Body).Decode(&dashboard); err != nil {
		t.Fatalf("decode dashboard overview: %v", err)
	}
	if dashboard.Headline.TotalServers != 2 || dashboard.Headline.OnlineServers != 1 || dashboard.Headline.OfflineServers != 1 {
		t.Fatalf("unexpected dashboard headline: %+v", dashboard.Headline)
	}
	if dashboard.Headline.ActiveAlerts != 2 || dashboard.Headline.SSHFailures != 1 || dashboard.Headline.LastUpdatedAt == "" {
		t.Fatalf("unexpected dashboard headline detail: %+v", dashboard.Headline)
	}
	if len(dashboard.Trends) != 2 {
		t.Fatalf("expected 2 dashboard trend points, got %d", len(dashboard.Trends))
	}
	if dashboard.Trends[1].SampleCount != 2 || dashboard.Trends[1].AvgMemoryUsage <= 0 {
		t.Fatalf("unexpected dashboard trend point: %+v", dashboard.Trends[1])
	}
	if len(dashboard.TopServers) == 0 || dashboard.TopServers[0].ID != 2 || dashboard.TopServers[0].RankReason == "" {
		t.Fatalf("unexpected top server list: %+v", dashboard.TopServers)
	}
	if dashboard.ResourceSummary.ReportingServers != 2 || dashboard.ResourceSummary.UnhealthyServers != 1 {
		t.Fatalf("unexpected resource summary: %+v", dashboard.ResourceSummary)
	}
	if dashboard.AlertSummary.Total != 2 || dashboard.AlertSummary.Warning != 2 {
		t.Fatalf("unexpected alert summary: %+v", dashboard.AlertSummary)
	}

	dashboardRangeReq := httptest.NewRequest(http.MethodGet, "/api/overview/dashboard?range=1h", nil)
	dashboardRangeRec := httptest.NewRecorder()
	router.ServeHTTP(dashboardRangeRec, dashboardRangeReq)
	if dashboardRangeRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard range status %d, got %d", http.StatusOK, dashboardRangeRec.Code)
	}

	var rangedDashboard dashboardOverviewResponse
	if err := json.NewDecoder(dashboardRangeRec.Body).Decode(&rangedDashboard); err != nil {
		t.Fatalf("decode ranged dashboard overview: %v", err)
	}
	if len(rangedDashboard.Trends) != 2 {
		t.Fatalf("expected 2 ranged dashboard trend points, got %d", len(rangedDashboard.Trends))
	}

	dashboardWeekReq := httptest.NewRequest(http.MethodGet, "/api/overview/dashboard?range=7d", nil)
	dashboardWeekRec := httptest.NewRecorder()
	router.ServeHTTP(dashboardWeekRec, dashboardWeekReq)
	if dashboardWeekRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard week range status %d, got %d", http.StatusOK, dashboardWeekRec.Code)
	}

	dashboardBadRangeReq := httptest.NewRequest(http.MethodGet, "/api/overview/dashboard?range=bad", nil)
	dashboardBadRangeRec := httptest.NewRecorder()
	router.ServeHTTP(dashboardBadRangeRec, dashboardBadRangeReq)
	if dashboardBadRangeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected dashboard bad range status %d, got %d", http.StatusBadRequest, dashboardBadRangeRec.Code)
	}

	liveReq := httptest.NewRequest(http.MethodGet, "/api/servers?includeStatus=1", nil)
	liveRec := httptest.NewRecorder()
	router.ServeHTTP(liveRec, liveReq)
	if liveRec.Code != http.StatusOK {
		t.Fatalf("expected live server list status %d, got %d", http.StatusOK, liveRec.Code)
	}
	var liveItems []map[string]any
	if err := json.NewDecoder(liveRec.Body).Decode(&liveItems); err != nil {
		t.Fatalf("decode live server list: %v", err)
	}
	if len(liveItems) != 2 {
		t.Fatalf("expected 2 live server items, got %d", len(liveItems))
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/servers/1/status", nil)
	statusRec := httptest.NewRecorder()
	router.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status endpoint code %d, got %d", http.StatusOK, statusRec.Code)
	}

	var status serverStatusResponse
	if err := json.NewDecoder(statusRec.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.ID != 1 || status.Name != "prod-web-01" || !status.Enabled || !status.Online || status.MemoryUsage != 50 {
		t.Fatalf("unexpected status response: %+v", status)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/api/servers/1/metrics?range=24h", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics endpoint code %d, got %d", http.StatusOK, metricsRec.Code)
	}

	var metrics metricsResponse
	if err := json.NewDecoder(metricsRec.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if len(metrics.Points) != 2 {
		t.Fatalf("expected 2 metric points, got %d", len(metrics.Points))
	}
	if metrics.Points[1].MemoryUsage != 50 {
		t.Fatalf("unexpected metric response: %+v", metrics.Points[1])
	}
}
