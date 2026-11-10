package handler

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/database"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
)

// Explicit local acceptance only: the identity transport uses a disposable
// Platform session, while Studio routes, published SDK and workers are real.
func TestStudioNativeGatewayBrowser(t *testing.T) {
	manifest := os.Getenv("STUDIO_JOINT_PLATFORM_MANIFEST")
	dir := os.Getenv("STUDIO_JOINT_OUTPUT_DIR")
	if manifest == "" || dir == "" {
		t.Skip("explicit free Platform/Gateway browser fixture only")
	}
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		URL, Host, Origin, Cookie string
		ProductKey                string `json:"product_key"`
		EdgeURL                   string `json:"edge_url"`
		EdgeCertificate           string `json:"edge_certificate"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.URL == "" || m.Host != "studio.oceanway.tech" || m.Origin != "https://studio.oceanway.tech" || m.Cookie == "" || m.ProductKey == "" {
		t.Fatal("invalid isolated fixture")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDIS_URL", "")
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": m.URL, "STUDIO_PLATFORM_PRODUCT_KEY": m.ProductKey, "STUDIO_PUBLIC_ORIGIN": m.Origin, "STUDIO_PLATFORM_SESSION_HOSTS": m.Host, "STUDIO_PLATFORM_ALLOWED_ORIGINS": m.Origin}
	gateway, err := platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "studio.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewStudio(repository.New(db), dir, gateway)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ConfigureRuntime(svc)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := RegisterStudioAPI(router.Group("/api"), svc); err != nil {
		t.Fatal(err)
	}
	router.NoRoute(StudioUnavailableRoute)
	svc.StartStudioWorker()
	certs := x509.NewCertPool()
	if !certs.AppendCertsFromPEM([]byte(m.EdgeCertificate)) {
		t.Fatal("missing fixture edge certificate")
	}
	edgeClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certs}}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	proxyEdge := func(w http.ResponseWriter, r *http.Request, location string) {
		req, err := http.NewRequestWithContext(r.Context(), "GET", location, nil)
		if err != nil {
			http.Error(w, "invalid fixture ticket", 502)
			return
		}
		req.Header.Set("Range", r.Header.Get("Range"))
		response, err := edgeClient.Do(req)
		if err != nil {
			http.Error(w, "fixture edge unavailable", 502)
			return
		}
		defer response.Body.Close()
		for key, values := range response.Header {
			w.Header()[key] = values
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const edgePrefix = "/api/__joint/edge"
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, edgePrefix+"/") {
			proxyEdge(w, r, m.EdgeURL+strings.TrimPrefix(r.URL.RequestURI(), edgePrefix))
			return
		}
		r.Host = m.Host
		r.Header.Set("Origin", m.Origin)
		r.Header.Set("Cookie", m.Cookie)
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/resources/") && strings.HasSuffix(r.URL.Path, "/file") {
			// Only the test edge's pinned certificate is trusted. Browser receives
			// bytes from the real signed display route; no TLS exception is installed.
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, r)
			location := recorder.Header().Get("Location")
			if recorder.Code == http.StatusTemporaryRedirect && strings.HasPrefix(location, m.EdgeURL+"/") {
				proxyEdge(w, r, location)
				return
			}
			for key, values := range recorder.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/resources/access" {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, r)
			for key, values := range recorder.Header() {
				w.Header()[key] = values
			}
			// Preserve real tickets and variant selection; bridge only the test
			// edge origin so Chrome need not trust a disposable TLS certificate.
			w.Header().Del("Content-Length")
			w.WriteHeader(recorder.Code)
			_, _ = io.WriteString(w, strings.ReplaceAll(recorder.Body.String(), m.EdgeURL+"/", edgePrefix+"/"))
			return
		}
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(bridge.Close)
	ready, _ := json.Marshal(map[string]any{"url": bridge.URL, "scope": "real Studio API and published SDK; disposable authenticated loopback transport"})
	if err := os.WriteFile(filepath.Join(dir, "ready.json"), ready, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(40 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("Studio browser fixture expired without explicit completion")
}
