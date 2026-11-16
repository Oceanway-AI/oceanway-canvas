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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
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
	const videoModel = `{"id":43,"model_name":"exact-video","model_type":"video","content":{"display_name":"Fixture Video"},"invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"video","variants":[{"id":"video","media_generation_v1":true,"protocol":"doubao-video","operation":"generate","method":"POST","path":"/doubao/api/v3/contents/generations/tasks","defaults":{"duration":2,"ratio":"16:9","resolution":"720p"},"specifications_schema":{"type":"object","properties":{"duration":{"type":"integer","enum":[2]},"ratio":{"type":"string","enum":["16:9"]},"resolution":{"type":"string","enum":["720p"]}}},"parameter_bindings":{"duration":"duration","ratio":"ratio","resolution":"resolution"}}]}}`
	f.catalog = strings.TrimSuffix(f.catalog, "]}}") + "," + videoModel + "]}}"
	var mu sync.Mutex
	posts, reads, preview, quotes := 0, 0, 0, 0
	ready, early := false, false
	var frame bytes.Buffer
	if err := png.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(frame.Bytes()))
	videoPath := filepath.Join(t.TempDir(), "fixture.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x48:r=12", "-t", "2", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-movflags", "+faststart", videoPath).CombinedOutput(); err != nil {
		t.Fatalf("fixture MP4: %s %v", out, err)
	}
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	videoDigest := fmt.Sprintf("%x", sha256.Sum256(video))
	kind := "image"
	ranged := 0
	served := 0
	bindings := map[string]platformclient.Binding{}
	boundPosts, requested := 0, 0
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
		if kind == "video" {
			if r.Header.Get("Range") != "" {
				ranged++
			}
			w.Header().Set("Content-Type", "video/mp4")
			http.ServeContent(w, r, "fixture.mp4", time.Unix(1, 0), bytes.NewReader(video))
			return
		}
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
			var native struct {
				N int `json:"n"`
			}
			_ = json.Unmarshal(c.NativeRequest, &native)
			kind = "image"
			requested = native.N
			if c.Model == "exact-video" {
				kind = "video"
				requested = 1
			}
			if requested < 1 || requested > 3 {
				w.WriteHeader(422)
				return
			}
			if requested == 3 {
				early = true
			}
		} else {
			if !strings.HasSuffix(r.URL.Path, "/"+original) {
				w.WriteHeader(404)
				return
			}
			reads++
		}
		g := map[string]any{"request_id": original, "task_id": "fixture-platform-task", "revision": 1, "execution_state": "running", "funds_state": "reserved", "outputs": []any{}, "retry_after_ms": 400}
		if early || ready {
			g["revision"] = 2
			g["execution_state"] = "succeeded"
			g["funds_state"] = "settled"
			if ready {
				g["revision"] = 3
				g["retry_after_ms"] = 0
			}
			outputs := make([]map[string]any, 0, requested)
			for i := 0; i < requested; i++ {
				suffix := original + "_" + strconv.Itoa(i)
				out := map[string]any{"index": i, "kind": "image", "content_state": "available", "preview_state": "pending", "material_state": "saved", "asset_id": "asset_" + suffix, "version_id": "version_" + suffix, "file_id": "file_" + suffix, "bytes": frame.Len(), "mime_type": "image/png", "width": 16, "height": 16, "sha256": digest, "owner": map[string]string{"kind": "account", "id": "101"}}
				out["kind"] = kind
				if kind == "video" {
					out["mime_type"] = "video/mp4"
					out["bytes"] = len(video)
					out["sha256"] = videoDigest
					out["width"] = 64
					out["height"] = 48
					out["duration_ms"] = 2000
				}
				if !ready && (requested == 1 || i == 1) {
					out = map[string]any{"index": i, "kind": kind, "material_state": "saving", "preview_state": "available", "content_state": "available", "preview_path": "/internal/products/v1/media-generations/" + original + "/outputs/" + strconv.Itoa(i) + "/preview"}
					if requested == 3 {
						out["material_state"] = "failed"
						out["error"] = map[string]any{"code": "material_archive_unavailable", "stage": "archive", "recovery": map[string]string{"action": "query_original", "availability": "available"}}
					}
				}
				outputs = append(outputs, out)
			}
			g["outputs"] = outputs
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
			binding, ok := bindings[r.URL.Query().Get("command_id")]
			if !ok {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(binding)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/bindings"):
			var in platformclient.BindingRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(r.URL.Path, "/")
			asset := parts[len(parts)-2]
			binding := platformclient.Binding{BindingID: "bind_" + asset, AssetID: asset, VersionID: in.VersionID, TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "active"}
			bindings[in.CommandID] = binding
			boundPosts++
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
			present := map[string]bool{}
			for _, binding := range bindings {
				present[binding.AssetID] = true
			}
			indexes := []int{}
			for i := 0; i < requested; i++ {
				if present["asset_"+original+"_"+strconv.Itoa(i)] {
					indexes = append(indexes, i)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"posts": posts, "gets": reads, "ticketGets": preview, "quotes": quotes, "previewReads": served, "adopted": requested > 0 && len(indexes) == requested, "requestedOutputs": requested, "boundIndexes": indexes, "bindingPosts": boundPosts, "rangeReads": ranged})
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
		if r.Header.Get("Origin") != "" {
			r.Header.Set("Origin", "https://studio.example.test")
		}
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
