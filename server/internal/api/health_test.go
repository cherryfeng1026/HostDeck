package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hostdeck/server/internal/httpx"
)

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rec := httptest.NewRecorder()

	httpx.NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
