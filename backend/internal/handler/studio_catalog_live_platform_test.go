//go:build platformpresentationlive

package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/database"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
)

// This test is launched by Platform's owned PostgreSQL fixture. It uses the
// released SDK and production Studio router; no provider response is stubbed.
func TestStudioCatalogPresentationLivePlatform(t *testing.T) {
	path := os.Getenv("STUDIO_PRESENTATION_LIVE_MANIFEST")
	if path == "" {
		t.Fatal("requires the isolated Platform live catalog runner")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("live manifest must be a private 0600 file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		URL        string `json:"url"`
		Host       string `json:"host"`
		Origin     string `json:"origin"`
		Cookie     string `json:"cookie"`
		ProductKey string `json:"product_key"`
		Evidence   string `json:"evidence"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.URL == "" || manifest.Cookie == "" {
		t.Fatal("invalid isolated provider manifest")
	}
	env := map[string]string{
		"STUDIO_PLATFORM_ORIGIN": manifest.URL, "STUDIO_PLATFORM_PRODUCT_KEY": manifest.ProductKey,
		"STUDIO_PUBLIC_ORIGIN": manifest.Origin, "STUDIO_PLATFORM_SESSION_HOSTS": manifest.Host,
		"STUDIO_PLATFORM_ALLOWED_ORIGINS": manifest.Origin,
	}
	t.Setenv("REDIS_URL", "")
	gateway, err := platformintegration.NewGatewayFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewStudio(repository.New(db), t.TempDir(), gateway)
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
	studio := httptest.NewServer(router)
	t.Cleanup(studio.Close)
	request := func(cookie string, expected int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, studio.URL+"/api/model-catalog", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = manifest.Host
		req.Header.Set("Origin", manifest.Origin)
		req.Header.Set("Cookie", cookie)
		response, err := studio.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != expected {
			t.Fatalf("Studio catalog status=%d want=%d body=%s", response.StatusCode, expected, body)
		}
		return body
	}
	body := request(manifest.Cookie, http.StatusOK)
	var host struct {
		Code int `json:"code"`
		Data struct {
			Source   string `json:"source"`
			Channels []struct {
				PlatformGroups json.RawMessage `json:"platformGroups"`
				Models         []struct {
					ModelKey  string          `json:"modelKey"`
					Available bool            `json:"available"`
					Platform  json.RawMessage `json:"platform"`
				} `json:"models"`
			} `json:"channels"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &host) != nil || host.Code != service.CodeOK || host.Data.Source != "platform" || len(host.Data.Channels) != 1 {
		t.Fatal("invalid actual Studio catalog envelope")
	}
	// Read the actual provider through the same released SDK, then compare the
	// complete public wire object; no locally reconstructed DTO is authoritative.
	provider, err := gateway.Client.Models(t.Context(), platformintegration.Session(&http.Request{Host: manifest.Host, Header: http.Header{"Cookie": {manifest.Cookie}, "Origin": {manifest.Origin}}}))
	if err != nil {
		t.Fatal(err)
	}
	var direct struct {
		Data struct {
			Items  []json.RawMessage `json:"items"`
			Groups json.RawMessage   `json:"groups"`
		} `json:"data"`
	}
	if json.Unmarshal(provider.Response.Body, &direct) != nil || len(direct.Data.Items) != 3 || len(host.Data.Channels[0].Models) != 3 {
		t.Fatal("expected one text model and two image variants from live provider")
	}
	decode := func(raw []byte) any {
		t.Helper()
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		// Collection timestamps change between two genuine HTTP reads.
		var scrub func(any)
		scrub = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				delete(x, "sampled_at")
				delete(x, "window_end")
				for _, child := range x {
					scrub(child)
				}
			case []any:
				for _, child := range x {
					scrub(child)
				}
			}
		}
		scrub(value)
		return value
	}
	if !reflect.DeepEqual(decode(direct.Data.Groups), decode(host.Data.Channels[0].PlatformGroups)) {
		t.Fatal("live provider groups changed in SDK/Studio forwarding")
	}
	want := map[string]json.RawMessage{}
	for _, item := range direct.Data.Items {
		var id struct {
			ModelName string `json:"model_name"`
		}
		if json.Unmarshal(item, &id) != nil {
			t.Fatal("invalid provider identity")
		}
		want[id.ModelName] = item
	}
	for _, item := range host.Data.Channels[0].Models {
		if !item.Available || !reflect.DeepEqual(decode(want[item.ModelKey]), decode(item.Platform)) {
			t.Fatalf("published model %s lost availability or public wire fields", item.ModelKey)
		}
	}
	if strings.Contains(string(body), "presentation-hidden") {
		t.Fatal("product-hidden model leaked")
	}
	request("", http.StatusUnauthorized)
	if manifest.Evidence != "" {
		for name, data := range map[string][]byte{"live-host-catalog.json": body, "live-provider-catalog.json": provider.Response.Body} {
			if err := os.WriteFile(filepath.Join(manifest.Evidence, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Log("real Platform HTTP -> published platformclient -> real Studio HTTP: 3 callable models, 2 image variants, exact groups/presentation/price/health, hidden model excluded, anonymous 401")
}
