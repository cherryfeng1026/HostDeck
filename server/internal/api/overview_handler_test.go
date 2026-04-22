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

	err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		CollectorMode: "ssh_only",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	second := time.Now().UTC().Add(-5 * time.Minute)
	first := second.Add(-30 * time.Minute)
	firstSnapshot := collector.Snapshot{
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
	}
	secondSnapshot := collector.Snapshot{
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
	}

	if err := statusRepo.UpsertLatest(context.Background(), 1, secondSnapshot, second); err != nil {
		t.Fatalf("upsert latest: %v", err)
	}
	if err := statusRepo.AppendHistory(context.Background(), 1, firstSnapshot, first); err != nil {
		t.Fatalf("append first history: %v", err)
	}
	if err := statusRepo.AppendHistory(context.Background(), 1, secondSnapshot, second); err != nil {
		t.Fatalf("append second history: %v", err)
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
	if overview.TotalServers != 1 || overview.OnlineServers != 1 || overview.OfflineServers != 0 || overview.SSHFailures != 0 || overview.ActiveAlerts != 0 {
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
	if overview.ActiveAlerts != 1 {
		t.Fatalf("expected activeAlerts=1, got %+v", overview)
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
	if len(liveItems) != 1 {
		t.Fatalf("expected 1 live server item, got %d", len(liveItems))
	}
	if liveItems[0]["online"] != true || liveItems[0]["memoryUsage"] != 50.0 {
		t.Fatalf("unexpected live server item: %+v", liveItems[0])
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
