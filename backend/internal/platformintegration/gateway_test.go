package platformintegration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

func testGateway(t *testing.T, origin string) *Gateway {
	t.Helper()
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": origin, "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	g, err := NewGatewayFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestStudioLoginPreparationAndInboundAuthority(t *testing.T) {
	callback := "https://studio.example.test/platform/login/complete"
	malicious := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/platform/v1/auth/login-return" || r.URL.Query().Get("return_url") != callback {
			t.Error("wrong login contract")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("X-Platform-Product-Key") != "" {
			t.Error("login preparation leaked credentials")
		}
		path := "/login?return=" + url.QueryEscape(callback)
		if malicious {
			path = "//evil.test/login?return=" + url.QueryEscape(callback)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]string{"return_url": callback, "login_path": path}})
	}))
	defer server.Close()
	g := testGateway(t, server.URL)
	if target, err := g.LoginURL(context.Background()); err != nil || target != server.URL+"/login?return="+url.QueryEscape(callback) {
		t.Fatalf("login URL: %s %v", target, err)
	}
	malicious = true
	if _, err := g.LoginURL(context.Background()); err == nil {
		t.Fatal("accepted off-origin redirect")
	}
	req := httptest.NewRequest("PATCH", "https://studio.example.test/api/canvas-projects/1", nil)
	if g.ValidateInbound(req) == nil {
		t.Fatal("accepted mutation without Origin")
	}
	req.Header.Set("Origin", "https://studio.example.test")
	if err := g.ValidateInbound(req); err != nil {
		t.Fatal(err)
	}
	req.Host = "attacker.test"
	req.Header.Set("X-Forwarded-Host", "studio.example.test")
	if g.ValidateInbound(req) == nil {
		t.Fatal("forwarded host bypass")
	}
}

func TestStudioWorkerCredentialIsolationAndCancellation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "" {
			t.Error("worker sent browser credentials")
		}
		if r.Header.Get("X-Platform-Product-Key") != "fixture.secret" {
			t.Error("missing product credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"version":"oceanway.text-request.v1","request_id":"request1","status":"pending"}}`))
	}))
	defer server.Close()
	g := testGateway(t, server.URL)
	if _, err := g.Client.RecoverText(context.Background(), platformclient.ExecutionContext{Authorization: "exa1.fixture"}, "request1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Client.RecoverText(ctx, platformclient.ExecutionContext{Authorization: "exa1.fixture"}, "request1"); err == nil {
		t.Fatal("cancelled request succeeded")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
