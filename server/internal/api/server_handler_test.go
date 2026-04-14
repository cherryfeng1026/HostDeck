package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/storage"
)

type serverListItem struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	IP                 string   `json:"ip"`
	SSHPort            int      `json:"sshPort"`
	CollectorMode      string   `json:"collectorMode"`
	Tags               []string `json:"tags"`
	Enabled            bool     `json:"enabled"`
	PasswordConfigured bool     `json:"passwordConfigured"`
}

func openAPITestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:server-handler-test?mode=memory&cache=shared")
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

func TestServerRoutes_CRUD(t *testing.T) {
	db := openAPITestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(repo, nil), nil, nil, nil, nil, nil)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/servers",
		bytes.NewBufferString(`{"name":"prod-web-01","hostname":"prod-web-01","ip":"10.0.0.21","sshPort":22,"username":"root","authType":"password","password":"super-secret","collectorMode":"ssh_only","tags":["prod","web"]}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	items := requestServerList(t, router, "/api/servers")
	if len(items) != 1 {
		t.Fatalf("expected 1 server after create, got %d", len(items))
	}
	if items[0].Name != "prod-web-01" || items[0].IP != "10.0.0.21" {
		t.Fatalf("unexpected server after create: %+v", items[0])
	}
	if !items[0].Enabled {
		t.Fatalf("expected created server to be enabled by default")
	}
	if !items[0].PasswordConfigured {
		t.Fatalf("expected created server passwordConfigured to be true")
	}

	updateReq := httptest.NewRequest(
		http.MethodPut,
		"/api/servers/1",
		bytes.NewBufferString(`{"name":"prod-web-01","hostname":"prod-web-01","ip":"10.0.0.22","sshPort":2222,"username":"admin","authType":"password","collectorMode":"prefer_agent","tags":["prod"],"enabled":false}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusNoContent {
		t.Fatalf("expected update status %d, got %d", http.StatusNoContent, updateRec.Code)
	}

	items = requestServerList(t, router, "/api/servers?keyword=10.0.0.22")
	if len(items) != 1 {
		t.Fatalf("expected filtered list length 1, got %d", len(items))
	}
	if items[0].SSHPort != 2222 || items[0].CollectorMode != "ssh_only" {
		t.Fatalf("unexpected server after update: %+v", items[0])
	}
	if items[0].Enabled {
		t.Fatalf("expected updated server to be disabled")
	}
	if !items[0].PasswordConfigured {
		t.Fatalf("expected updated server to keep passwordConfigured=true")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/servers/1", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete status %d, got %d", http.StatusNoContent, deleteRec.Code)
	}

	items = requestServerList(t, router, "/api/servers")
	if len(items) != 0 {
		t.Fatalf("expected empty list after delete, got %d", len(items))
	}
}

func TestServerRoutes_CreateRejectsDuplicateIP(t *testing.T) {
	db := openAPITestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(repo, nil), nil, nil, nil, nil, nil)

	firstReq := httptest.NewRequest(
		http.MethodPost,
		"/api/servers",
		bytes.NewBufferString(`{"name":"prod-web-01","hostname":"prod-web-01","ip":"10.0.0.21","sshPort":22,"username":"root","authType":"password","password":"super-secret","collectorMode":"ssh_only"}`),
	)
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)

	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first create status %d, got %d", http.StatusCreated, firstRec.Code)
	}

	secondReq := httptest.NewRequest(
		http.MethodPost,
		"/api/servers",
		bytes.NewBufferString(`{"name":"prod-web-02","hostname":"prod-web-02","ip":"10.0.0.21","sshPort":22,"username":"root","authType":"password","password":"another-secret","collectorMode":"ssh_only"}`),
	)
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate create status %d, got %d with body %s", http.StatusConflict, secondRec.Code, secondRec.Body.String())
	}

	var payload map[string]string
	if err := json.NewDecoder(secondRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode duplicate response: %v", err)
	}
	if payload["error"] != "服务器 IP 已存在: 10.0.0.21" {
		t.Fatalf("unexpected duplicate error: %q", payload["error"])
	}
}

func TestServerRoutes_CreateRequiresPasswordWhenUsingPasswordAuth(t *testing.T) {
	db := openAPITestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(repo, nil), nil, nil, nil, nil, nil)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/servers",
		bytes.NewBufferString(`{"name":"prod-web-01","hostname":"prod-web-01","ip":"10.0.0.21","username":"root","authType":"password","collectorMode":"ssh_only"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["error"] != "SSH 密码不能为空" {
		t.Fatalf("unexpected error message: %q", payload["error"])
	}
}

func TestServerRoutes_ListDoesNotReturnPassword(t *testing.T) {
	db := openAPITestDB(t)
	repo := storage.NewServerRepository(db, "test-master-key")
	router := httpx.NewRouterWithHandlers(api.NewServerHandler(repo, nil), nil, nil, nil, nil, nil)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/servers",
		bytes.NewBufferString(`{"name":"prod-web-01","hostname":"prod-web-01","ip":"10.0.0.21","sshPort":22,"username":"root","authType":"password","password":"super-secret","collectorMode":"ssh_only","tags":["prod","web"]}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d with body %s", http.StatusCreated, createRec.Code, createRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 server, got %d", len(payload))
	}
	if payload[0]["passwordConfigured"] != true {
		t.Fatalf("expected passwordConfigured=true, got %#v", payload[0]["passwordConfigured"])
	}
	if _, ok := payload[0]["password"]; ok {
		t.Fatalf("expected list payload to hide password, got %#v", payload[0]["password"])
	}
}

func requestServerList(t *testing.T, router http.Handler, path string) []serverListItem {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, rec.Code)
	}

	var items []serverListItem
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return items
}
