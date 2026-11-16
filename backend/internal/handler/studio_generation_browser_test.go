//go:build studio_media_browser

package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

// A loopback browser exercises Studio routes and its SDK against a free
// Platform HTTP fixture. Counters exclude page identity and model catalog reads.
func TestStudioGenerationBrowserFixture(t *testing.T) {
	addr := os.Getenv("STUDIO_MEDIA_FIXTURE_ADDR")
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatal("loopback address required")
	}
	f := newStudioFixture(t)
	f.catalog = strings.Replace(studioGenerationCatalog, `"model_name":"exact-image"`, `"id":42,"model_name":"exact-image","content":{"display_name":"Fixture Image"}`, 1)
	var mu sync.Mutex
	posts, reads, preview, quotes := 0, 0, 0, 0
	ready, early := false, false
	var frame bytes.Buffer
	if err := png.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(frame.Bytes()))
	served := 0
	var binding platformclient.Binding
	f.delivery = func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Query().Get("ticket") != "fixture" || (!early && !ready) {
			w.WriteHeader(403)
			return
		}
		if r.Header.Get("X-Platform-Product-Key") != "" || r.Header.Get("X-Platform-Execution-Authorization") != "" {
			t.Error("server credential crossed browser redirect")
		}
		served++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(frame.Bytes())
	}
	var original string
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "consumption-preview") {
			quotes++
			w.WriteHeader(418)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/internal/products/v1/media-generations") {
			w.WriteHeader(404)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/preview") {
			preview++
			w.Header().Set("Location", "/api/platform/v1/media-previews/local-frame/0?ticket=fixture")
			w.WriteHeader(302)
			return
		}
		if r.Method == "POST" {
			posts++
			var c platformclient.MediaCommand
			_ = json.NewDecoder(r.Body).Decode(&c)
			original = c.RequestID
			early = false
			ready = false
			binding = platformclient.Binding{}
		} else {
			if !strings.HasSuffix(r.URL.Path, "/"+original) {
				w.WriteHeader(404)
				return
			}
			reads++
		}
		g := map[string]any{"request_id": original, "task_id": "fixture-platform-task", "revision": 1, "execution_state": "running", "funds_state": "reserved", "outputs": []any{}, "retry_after_ms": 400}
		if early {
			g["revision"] = 2
			g["execution_state"] = "succeeded"
			g["funds_state"] = "settled"
			g["outputs"] = []map[string]any{{"index": 0, "kind": "image", "material_state": "saving", "preview_state": "available", "content_state": "available", "preview_path": "/internal/products/v1/media-generations/" + original + "/outputs/0/preview"}}
		}
		if ready {
			g["revision"] = 3
			g["execution_state"] = "succeeded"
			g["funds_state"] = "settled"
			g["retry_after_ms"] = 0
			g["outputs"] = []map[string]any{{"index": 0, "kind": "image", "content_state": "available", "preview_state": "pending", "material_state": "saved", "asset_id": "asset_" + original, "version_id": "version_" + original, "file_id": "file_" + original, "bytes": frame.Len(), "mime_type": "image/png", "width": 16, "height": 16, "sha256": digest, "owner": map[string]string{"kind": "account", "id": "101"}}}
		}
		_ = json.NewEncoder(w).Encode(g)
	}
	f.account = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"user":{"id":101,"status":1,"display_name":"Platform user"},"workspace":{"kind":"personal","id":"personal-101","actor_user_id":101,"name":"Personal"},"balance":{"points":"100"},"applications":[]}}`))
	}
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/assets"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/authorization"):
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization": "mat1.browser-fixture", "expires_at": time.Now().Add(time.Hour).Unix()})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/bindings"):
			if binding.BindingID == "" {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(binding)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/bindings"):
			var in platformclient.BindingRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			binding = platformclient.Binding{BindingID: "bind_" + original, AssetID: "asset_" + original, VersionID: "version_" + original, TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "active"}
			_ = json.NewEncoder(w).Encode(binding)
		default:
			w.WriteHeader(404)
		}
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/__fixture/counts":
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"posts": posts, "gets": reads, "ticketGets": preview, "quotes": quotes, "previewReads": served, "adopted": binding.BindingID != ""})
			return
		case "/api/__fixture/previewable":
			mu.Lock()
			early = true
			mu.Unlock()
			w.WriteHeader(204)
			return
		case "/api/__fixture/ready":
			mu.Lock()
			ready = true
			mu.Unlock()
			w.WriteHeader(204)
			return
		}
		r.Host = "studio.example.test"
		r.Header.Set("Origin", "https://studio.example.test")
		if _, err := r.Cookie("platform_session"); err != nil {
			r.Header.Set("Cookie", "platform_session=alice")
		}
		f.router.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	_ = server.Close()
}
