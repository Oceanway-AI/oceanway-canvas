package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
)

type studioFixture struct {
	router       *gin.Engine
	db           *gorm.DB
	svc          *service.Service
	admin        bool
	unavailable  bool
	revoked      bool
	rotate       bool
	requests     int
	catalog      string
	catalogCalls int
	materials    http.HandlerFunc
	execution    http.HandlerFunc
}

func newStudioFixture(t *testing.T) *studioFixture {
	t.Helper()
	t.Setenv("REDIS_URL", "")
	gin.SetMode(gin.TestMode)
	f := &studioFixture{}
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		if f.execution != nil && (strings.HasPrefix(r.URL.Path, "/internal/relay/") || strings.HasPrefix(r.URL.Path, "/api/platform/v1/execution-authorizations")) {
			f.execution(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/platform/v1/materials/") && f.materials != nil {
			if r.Header.Get("X-Platform-Product-Key") != "fixture.secret" || r.Header.Get("Cookie") == "" || r.Header.Get("X-Oceanway-Materials-Version") != "2" {
				t.Error("material request missing account/product/version authority")
			}
			f.materials(w, r)
			return
		}
		if r.URL.Path == "/api/platform/v1/products/studio/models" {
			f.catalogCalls++
			if r.Header.Get("X-Platform-Product-Key") != "fixture.secret" || r.Header.Get("Cookie") == "" {
				t.Error("catalog did not carry both product and current account authority")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Add("Set-Cookie", "catalog_nonce=next; Secure; HttpOnly")
			if f.unavailable {
				w.WriteHeader(503)
				return
			}
			_, _ = w.Write([]byte(f.catalog))
			return
		}
		if r.Header.Get("X-Platform-Product-Key") != "" {
			t.Error("identity request received product key")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/platform/v1/auth/login-return" {
			callback := r.URL.Query().Get("return_url")
			_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{"return_url": callback, "login_path": "/login?return=" + url.QueryEscape(callback)}})
			return
		}
		if f.unavailable {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"success":false,"message":"private.upstream.secret"}`))
			return
		}
		if r.URL.Path == "/api/platform/v1/logout" {
			if r.Header.Get("Origin") != "https://studio.example.test" {
				t.Error("logout origin lost")
			}
			http.SetCookie(w, &http.Cookie{Name: "platform_session", Value: "", MaxAge: -1, Path: "/"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"revoked_sid":"sid"}}`))
			return
		}
		id := 0
		switch r.Header.Get("Cookie") {
		case "platform_session=alice":
			id = 101
		case "platform_session=bob":
			id = 202
		}
		if id == 0 || f.revoked {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		roles := []string{}
		if f.admin {
			roles = append(roles, "studio:admin")
		}
		if f.rotate {
			w.Header().Add("Set-Cookie", "platform_session=rotated; Secure; HttpOnly")
			w.Header().Add("Set-Cookie", "nonce=next; Secure; HttpOnly")
		}
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{"user": gin.H{"id": id, "username": "same-name", "display_name": "Platform user", "status": 1, "role": 100}, "session": gin.H{"sid": "reference-only"}, "product_roles": roles}})
	}))
	t.Cleanup(platform.Close)
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": platform.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	gateway, err := platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	f.db = db
	f.svc, err = service.NewStudio(repository.New(db), t.TempDir(), gateway)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.svc.Close() })
	ConfigureRuntime(f.svc)
	f.router = gin.New()
	if err := RegisterStudioAPI(f.router.Group("/api"), f.svc); err != nil {
		t.Fatal(err)
	}
	f.router.NoRoute(StudioUnavailableRoute)
	return f
}

func TestStudioCatalogUsesPlatformAndNeverLocalChannels(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[{"id":41,"model_name":"exact-text-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","mode":"ratio","version":"price-1","rate_unit":"million_tokens","rates":{"input":"1.23456789","output":"0"}},"content":{"display_name":"模型显示名称"}},{"id":42,"model_name":"draft-image","model_type":"image","invocation_status":"price_only","price":{"status":"available","version":"price-2"}}]}}`
	w := f.request(t, "GET", "/api/model-catalog", "platform_session=alice", "", 200)
	var result struct {
		Data struct {
			Source   string `json:"source"`
			Channels []struct {
				Models []struct {
					ModelKey  string          `json:"modelKey"`
					Available bool            `json:"available"`
					Platform  json.RawMessage `json:"platform"`
				} `json:"models"`
			} `json:"channels"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Source != "platform" || len(result.Data.Channels) != 1 || len(result.Data.Channels[0].Models) != 2 {
		t.Fatalf("catalog: %s", w.Body.String())
	}
	models := result.Data.Channels[0].Models
	if models[0].ModelKey != "exact-text-model" || !models[0].Available || models[1].Available {
		t.Fatal("platform availability/model identity lost")
	}
	if !strings.Contains(string(models[0].Platform), `"1.23456789"`) {
		t.Fatal("exact published price lost")
	}
	if len(w.Header().Values("Set-Cookie")) != 1 {
		t.Fatal("catalog cookie update lost")
	}
	if f.catalogCalls != 1 {
		t.Fatal("catalog bypassed SDK")
	}
	f.request(t, "GET", "/api/model-catalog", "", "", 401)
	if f.catalogCalls != 1 {
		t.Fatal("anonymous catalog reached product API")
	}
	f.unavailable = true
	f.request(t, "GET", "/api/model-catalog", "platform_session=alice", "", 503)
}

func (f *studioFixture) request(t *testing.T, method, path, cookie, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://studio.example.test"+path, strings.NewReader(body))
	r.Header.Set("Cookie", cookie)
	r.Header.Set("Origin", "https://studio.example.test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, w.Code, status, w.Body.String())
	}
	return w
}

func TestStudioPlatformLoginCanvasPersistenceAndIsolation(t *testing.T) {
	f := newStudioFixture(t)
	f.rotate = true
	w := f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 200)
	if len(w.Header().Values("Set-Cookie")) != 2 {
		t.Fatal("lost rotated cookies")
	}
	if !strings.Contains(w.Body.String(), `"role":"user"`) {
		t.Fatal("numeric Platform role granted product admin")
	}
	f.rotate = false
	for i := 0; i < 2; i++ {
		f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 200)
	}
	var count int64
	f.db.Model(&model.User{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate users=%d", count)
	}
	body := `{"mutationId":"create-1","operations":[{"type":"create_canvas","id":"canvas-1","patch":{"title":"平台画布"}}]}`
	f.request(t, "PATCH", "/api/canvas-projects/canvas-1/operations", "platform_session=alice", body, 200)
	read := f.request(t, "GET", "/api/canvas-projects/canvas-1", "platform_session=alice", "", 200)
	if !strings.Contains(read.Body.String(), "平台画布") {
		t.Fatal("canvas not persisted")
	}
	f.request(t, "GET", "/api/canvas-projects/canvas-1", "platform_session=bob", "", 404)
	f.request(t, "GET", "/api/canvas-projects/canvas-1", "", "", 401)
	f.request(t, "POST", "/api/canvas-projects/canvas-1/share", "", `{}`, 401)
	share := f.request(t, "POST", "/api/canvas-projects/canvas-1/share", "platform_session=alice", `{"expiresDays":1}`, 200)
	var shared struct {
		Data struct {
			Share struct {
				Token string `json:"token"`
			} `json:"share"`
		} `json:"data"`
	}
	if err := json.Unmarshal(share.Body.Bytes(), &shared); err != nil || shared.Data.Share.Token == "" {
		t.Fatal("share token missing")
	}
	f.request(t, "GET", "/api/canvas-projects/canvas-1/share", "platform_session=bob", "", 404)
	publicURL := "/api/public/canvas-shares/" + shared.Data.Share.Token
	before := f.requests
	public := f.request(t, "GET", publicURL, "", "", 200)
	if f.requests != before || !strings.Contains(public.Body.String(), "平台画布") {
		t.Fatal("public share unexpectedly required a Platform session or lost its canvas")
	}
	f.request(t, "DELETE", "/api/canvas-projects/canvas-1/share", "platform_session=alice", "", 200)
	f.request(t, "GET", publicURL, "", "", 404)
	for _, table := range []any{&model.AuthSession{}, &model.CreditAccount{}, &model.BillingOrder{}} {
		if err := f.db.Model(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("unexpected local platform writes %T count=%d", table, count)
		}
	}
	f.request(t, "POST", "/api/auth/logout", "platform_session=alice", "{}", 200)
}

func TestStudioPlatformFailureAndLegacyRoutesStayClosed(t *testing.T) {
	f := newStudioFixture(t)
	f.unavailable = true
	w := f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 503)
	if strings.Contains(w.Body.String(), "private.upstream") {
		t.Fatal("upstream details leaked")
	}
	f.unavailable = false
	for _, path := range []string{"/api/auth/register", "/api/auth/login", "/api/auth/password-reset", "/api/ai/custom", "/api/tasks/legacy/retry", "/api/payments", "/v1/chat/completions"} {
		f.request(t, "POST", path, "platform_session=alice", "{}", 501)
	}
	request := httptest.NewRequest("PATCH", "https://studio.example.test/api/canvas-projects/c/operations", strings.NewReader(`{}`))
	request.Header.Set("Cookie", "platform_session=alice")
	before := f.requests
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, request)
	if w.Code != 403 || f.requests != before {
		t.Fatal("missing Origin reached Platform")
	}
	f.request(t, "GET", "/api/auth/login-url", "", "", 200)
}

func TestStudioRoleRevocationAndRealtimeRevalidation(t *testing.T) {
	f := newStudioFixture(t)
	req := httptest.NewRequest("GET", "https://studio.example.test/api/canvas-projects/c/realtime", nil)
	req.Header.Set("Cookie", "platform_session=alice")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req
	f.admin = true
	user, err := currentUser(ctx, f.svc)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RequireAdmin(user); err != nil {
		t.Fatal(err)
	}
	var stored model.User
	f.db.First(&stored, "id = ?", user.ID)
	if stored.Role != model.UserRoleUser || f.svc.RequireAdmin(&stored) == nil {
		t.Fatal("durable user inherited administrator authority")
	}
	f.admin = false
	current, err := currentUser(ctx, f.svc)
	if err != nil {
		t.Fatal(err)
	}
	if f.svc.RequireAdmin(current) == nil {
		t.Fatal("role revocation ignored")
	}
	if err := revalidateCanvasSession(context.Background(), ctx, f.svc, user.ID); err != nil {
		t.Fatal(err)
	}
	f.rotate = true
	if err := revalidateCanvasSession(context.Background(), ctx, f.svc, user.ID); err == nil {
		t.Fatal("rotated session did not require HTTP refresh")
	}
	f.rotate = false
	ctx.Request.Header.Set("Cookie", "platform_session=bob")
	if err := revalidateCanvasSession(context.Background(), ctx, f.svc, user.ID); err == nil {
		t.Fatal("changed account kept realtime access")
	}
	ctx.Request.Header.Set("Cookie", "platform_session=alice")
	f.revoked = true
	if err := revalidateCanvasSession(context.Background(), ctx, f.svc, user.ID); err == nil {
		t.Fatal("revoked session kept realtime access")
	}
}

func TestStudioIdentityPreservesCancellation(t *testing.T) {
	f := newStudioFixture(t)
	request := httptest.NewRequest("GET", "https://studio.example.test/api/auth/session", nil)
	request.Header.Set("Cookie", "platform_session=alice")
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		want := context.Canceled
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			cancel()
		}
		_, _, err := f.svc.StudioCurrentUser(ctx, platformintegration.Session(request))
		if !errors.Is(err, want) {
			t.Fatalf("identity cancellation lost: %v", err)
		}
		if _, err := f.svc.StudioLoginURL(ctx); !errors.Is(err, want) {
			t.Fatalf("login cancellation lost: %v", err)
		}
		cancel()
	}
	if f.requests != 0 {
		t.Fatal("cancelled request reached Platform")
	}
}
