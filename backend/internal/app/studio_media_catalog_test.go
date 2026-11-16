package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

func TestStudioOpenTabUsesOnlyAnExactPublishedSchemaAfterCatalogExpiry(t *testing.T) {
	const host = "studio.example.test"
	variant, price := strings.Repeat("a", 64), strings.Repeat("b", 64)
	published := platformclient.Model{
		ModelName: "shown", ModelType: "image", InvocationStatus: "available",
		Price:                  json.RawMessage(fmt.Sprintf(`{"status":"available","version":%q}`, price)),
		MediaGenerationProfile: json.RawMessage(fmt.Sprintf(`{"default_variant":%q,"variants":[{"id":%q,"media_generation_v1":true,"protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","specifications_schema":{"type":"object"}}]}`, variant, variant)),
	}
	for _, tc := range []struct {
		name, chosen, expectedPrice string
		useDisplayed                bool
	}{
		{"original displayed revision", variant, price, true},
		{"implicit variant", "", price, false},
		{"changed price", variant, strings.Repeat("c", 64), false},
		{"no expected price", variant, "", false},
		{"changed variant", strings.Repeat("d", 64), price, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalogGets := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				catalogGets++
				if r.Method != "GET" || r.URL.Path != "/api/platform/v1/products/studio/models" {
					t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"data":{"items":[]}}`))
			}))
			defer server.Close()
			client, err := platformclient.NewClient(platformclient.Config{Origin: server.URL, Product: "studio", ProductKey: "fixture.secret", AllowedSessionHosts: []string{host}, AllowedOrigins: []string{"https://" + host}})
			if err != nil {
				t.Fatal(err)
			}
			s := &Service{studioPlatform: &platformintegration.Gateway{Client: client}}
			session := platformclient.SessionContext{Host: host, Origin: "https://" + host, Cookie: "test_session=value"}
			scope := studioCatalogScopeFor("user", session, "actual-authenticated-session")
			s.studioMediaCatalog.remember(scope, []platformclient.Model{published})
			for i := range s.studioMediaCatalog.snapshots {
				s.studioMediaCatalog.snapshots[i].expires = time.Now().Add(-time.Minute)
			}
			req := CreateTaskRequest{Type: "canvas_image", Model: "shown", Prompt: "test", Platform: &StudioTaskOptions{VariantID: tc.chosen, ExpectedPriceVersion: tc.expectedPrice}}
			selected, err := s.studioSelectTaskModel(context.Background(), session, req, &platformclient.Response{Header: make(http.Header)}, scope)
			if tc.useDisplayed {
				if err != nil || selected.ID != variant || !selected.GenerationV1 || catalogGets != 0 {
					t.Fatalf("exact displayed schema: model=%s reads=%d err=%v", selected.ID, catalogGets, err)
				}
			} else if err == nil || catalogGets != 1 {
				t.Fatalf("expired or mismatched selection: reads=%d err=%v", catalogGets, err)
			}
		})
	}
}
