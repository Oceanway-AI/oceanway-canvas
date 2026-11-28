package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/catalogtest"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioCatalogHTTPFailureReasons(t *testing.T) {
	for _, tc := range []struct {
		status       int
		body, reason string
	}{
		{503, `{"success":false,"code":"catalog_projection_pending"}`, "catalog_projection_pending"},
		{503, `{"success":false}`, "catalog_unavailable"}, {409, `{}`, "catalog_revision_conflict"},
		{401, `{}`, "unauthorized"}, {403, `{}`, "forbidden"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
		client, err := platformclient.NewClient(platformclient.Config{Origin: server.URL, Product: "studio", ProductKey: "fixture.secret", AllowedSessionHosts: []string{"studio.example.test"}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.ModelSummaries(t.Context(), platformclient.SessionContext{Host: "studio.example.test", Cookie: "platform_session=alice"}, platformclient.ModelSummaryQuery{})
		server.Close()
		var mapped *AppError
		if !errors.As(studioCatalogError(err), &mapped) || mapped.Code != tc.status || string(mapped.Reason) != tc.reason {
			t.Fatalf("status=%d mapped=%+v", tc.status, mapped)
		}
	}
}

func TestStudioTextAdmissionDuringMediaCatalogOutage(t *testing.T) {
	for _, tc := range []struct {
		name, key, version string
		reads              int32
	}{
		{"selected", "pi-model", "p1", 1}, {"missing_version", "pi-model", "", 2}, {"default", "", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db, _, _ := creationTestService(t)
			if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
				t.Fatal(err)
			}
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogtest.Route(r.URL.Path) {
					reads.Add(1)
					time.Sleep(3 * time.Millisecond)
					if r.Header.Get("Cookie") != "platform_session=alice" {
						t.Error("catalog lost session authority")
					}
					if strings.HasSuffix(r.URL.Path, "/summaries") {
						q := r.URL.Query()
						switch q.Get("model_type") {
						case "":
							i, _ := strconv.Atoi(q.Get("cursor"))
							if i < 6 {
								json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.ModelSummaryPage{Version: platformclient.ModelCatalogVersion, State: "summary", CatalogRevision: "one", NextCursor: strconv.Itoa(i + 1), HasMore: true}})
								return
							}
						case "text":
							if tc.key == "" && q.Get("defaults_only") != "true" {
								t.Error("default lookup not scoped to defaults")
							}
							catalogtest.Write(t, w, r, []byte(strings.Replace(studioAgentMediaFixtureCatalog, `"model_type":"text"`, `"model_type":"text","is_default":true`, 1)))
							return
						}
						w.Header().Set("Retry-After", "1")
						w.WriteHeader(503)
						fmt.Fprint(w, `{"success":false,"code":"catalog_projection_pending"}`)
						return
					}
					catalogtest.Write(t, w, r, []byte(studioAgentMediaFixtureCatalog))
					return
				}
				if strings.HasSuffix(r.URL.Path, "/execution-authorizations") {
					var body map[string]string
					json.NewDecoder(r.Body).Decode(&body)
					json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_fixture", "task_reference": body["task_reference"], "product": "studio", "expires_at": time.Now().Add(time.Hour).Unix()})
					return
				}
				t.Errorf("unexpected request %s", r.URL.Path)
				w.WriteHeader(500)
			}))
			defer server.Close()
			client, err := platformclient.NewClient(platformclient.Config{Origin: server.URL, Product: "studio", ProductKey: "fixture.secret", AllowedSessionHosts: []string{"studio.example.test"}, AllowedOrigins: []string{"https://studio.example.test"}})
			if err != nil {
				t.Fatal(err)
			}
			s.studioPlatform = &platformintegration.Gateway{Client: client}
			req := CloudAgentRequest{Prompt: "hello", ChannelID: StudioPlatformChannelID, ChannelModelKey: tc.key, IdempotencyKey: "fixture-" + tc.name, PermissionMode: "read_only"}
			wire, _ := json.Marshal(map[string]string{"expectedPriceVersion": tc.version})
			if err := json.Unmarshal(wire, &req); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			run, _, err := s.CreateStudioCloudAgentRun(t.Context(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}, req, "")
			t.Logf("fixture catalog GETs=%d admission=%s", reads.Load(), time.Since(start))
			if err != nil || run == nil || reads.Load() != tc.reads {
				t.Fatalf("text admission: %v, reads=%d", err, reads.Load())
			}
		})
	}
}
