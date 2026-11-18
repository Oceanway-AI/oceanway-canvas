package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"yingce/backend/internal/testdb"

	"github.com/gin-gonic/gin"
)

func TestSystemReadinessTracksStartupAndDrain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testdb.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	svc := newStudioTestService(t, db)
	status := newSystemStatus(db, svc)
	router := gin.New()
	registerSystemStatusRoutes(router.Group("/api"), status)

	assertStatus(t, router, "/api/health/live", http.StatusOK)
	assertStatus(t, router, "/api/health/ready", http.StatusServiceUnavailable)
	status.markStarted()
	assertStatus(t, router, "/api/health/ready", http.StatusOK)
	status.beginDrain()
	assertStatus(t, router, "/api/health/ready", http.StatusServiceUnavailable)
	assertStatus(t, router, "/api/health/live", http.StatusOK)
}

func TestSystemReadinessRejectsMissingSchemaVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testdb.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	svc := newStudioTestService(t, db)
	if err := db.Exec("DROP TABLE schema_migrations").Error; err != nil {
		t.Fatal(err)
	}
	status := newSystemStatus(db, svc)
	status.markStarted()
	router := gin.New()
	registerSystemStatusRoutes(router.Group("/api"), status)
	assertStatus(t, router, "/api/health/ready", http.StatusServiceUnavailable)
}

func assertStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("GET %s status = %d, want %d; body=%s", path, recorder.Code, want, recorder.Body.String())
	}
}
