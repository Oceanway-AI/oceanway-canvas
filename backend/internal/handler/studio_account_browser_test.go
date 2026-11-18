//go:build studio_account_browser

package handler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Actual Studio routes with an isolated PostgreSQL schema and free Platform
// responses. This fixture is never registered in an application build.
func TestStudioAccountBrowserFixture(t *testing.T) {
	addr := os.Getenv("STUDIO_ACCOUNT_FIXTURE_ADDR")
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatal("loopback required")
	}
	f := newStudioFixture(t)
	studioWorkspaceFixture(f, "unset")
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/platform/v1/materials/authorization" {
			_ = json.NewEncoder(w).Encode(gin.H{"authorization": "mat1.local-account-fixture", "expires_at": time.Now().Add(time.Hour).Unix()})
			return
		}
		w.WriteHeader(403)
	}
	f.account = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{
			"version": "oceanway.account-summary.v1", "product": "studio", "user": gin.H{"id": 101, "status": 1, "display_name": "创作测试账号", "username": "studio-preview"},
			"workspace":  gin.H{"id": "personal-101", "name": "个人空间", "kind": "personal", "actor_user_id": 101},
			"balance":    gin.H{"state": "ready", "kind": "personal", "available_points": "120.123456", "frozen_points": "2.5"},
			"membership": gin.H{"state": "ready", "active_count": 1, "plan_title": "创作会员"},
			"benefits":   gin.H{"state": "ready", "items": []any{}}, "capabilities": gin.H{"can_read_finance": true, "can_purchase": true},
			"applications": []gin.H{{"product": "studio", "url": "https://studio.example.test"}, {"product": "console", "url": "/console"}, {"product": "drama", "url": "https://drama.example.test/drama"}},
		}})
	}
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected execution: %s", r.URL.Path)
		w.WriteHeader(403)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "studio.example.test"
		if r.Header.Get("Origin") != "" {
			r.Header.Set("Origin", "https://studio.example.test")
		}
		r.Header.Set("Cookie", "platform_session=alice")
		f.router.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	t.Log("Studio account fixture ready at", addr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	_ = server.Close()
}
