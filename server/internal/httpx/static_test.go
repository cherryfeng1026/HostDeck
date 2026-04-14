package httpx_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hostdeck/server/internal/httpx"
)

func TestWithStaticFallback_ServesAPIRoutesAndIndex(t *testing.T) {
	staticDir := t.TempDir()
	err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>spa</html>"), 0o644)
	if err != nil {
		t.Fatalf("write index: %v", err)
	}

	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	handler := httpx.WithStaticFallback(apiHandler, staticDir)

	apiReq := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	apiRec := httptest.NewRecorder()
	handler.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("expected api status %d, got %d", http.StatusOK, apiRec.Code)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/", nil)
	pageRec := httptest.NewRecorder()
	handler.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("expected page status %d, got %d", http.StatusOK, pageRec.Code)
	}
	if pageRec.Body.String() != "<html>spa</html>" {
		t.Fatalf("unexpected page body: %q", pageRec.Body.String())
	}
}

func TestWithRequestLogging_LogsRequestSummary(t *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	handler := httpx.WithRequestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/servers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buffer.String()
	if !strings.Contains(logLine, "msg=\"http request\"") {
		t.Fatalf("expected request log message, got %q", logLine)
	}
	if !strings.Contains(logLine, "method=POST") || !strings.Contains(logLine, "path=/api/servers") {
		t.Fatalf("expected method and path in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "status=201") {
		t.Fatalf("expected status in log, got %q", logLine)
	}
}
