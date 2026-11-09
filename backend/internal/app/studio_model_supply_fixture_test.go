package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

// The input files are exported from Platform's real isolated HTTP publication
// and PostgreSQL preview test. They are synthetic contracts, not vendor proof.
func TestStudioPlatformPublishedFixtureConsumption(t *testing.T) {
	const folder = "testdata/platform-model-supply-20261109"
	projections := []PublicChannelModel{}
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			var fixture struct {
				Model         platformclient.Model              `json:"model"`
				NativeRequest map[string]any                    `json:"native_request"`
				Preview       platformclient.ConsumptionPreview `json:"preview"`
			}
			raw, err := os.ReadFile(filepath.Join(folder, kind+"-published.json"))
			if err != nil || json.Unmarshal(raw, &fixture) != nil {
				t.Fatalf("cannot read provider fixture: %v", err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/models"):
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []platformclient.Model{fixture.Model}}})
				case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
					var input platformclient.PreviewRequest
					_ = json.NewDecoder(r.Body).Decode(&input)
					var native map[string]any
					_ = json.Unmarshal(input.Request.NativeRequest, &native)
					if !reflect.DeepEqual(native, fixture.NativeRequest) {
						t.Errorf("preview body differs from provider: %s", input.Request.NativeRequest)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": fixture.Preview})
				default:
					t.Errorf("unexpected execution or request: %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
			gateway, err := platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
			if err != nil {
				t.Fatal(err)
			}
			s := &Service{studioPlatform: gateway}
			session := platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "fixture=session"}
			catalog, _, err := s.StudioModelCatalog(t.Context(), session)
			if err != nil || len(catalog.Channels) != 1 || len(catalog.Channels[0].Models) != 1 {
				t.Fatalf("SDK catalog projection failed: %+v %v", catalog, err)
			}
			projected := catalog.Channels[0].Models[0]
			if !projected.Available {
				t.Fatal(projected.UnavailableReason)
			}
			projections = append(projections, projected)
			selected, err := platformintegration.SelectModel(fixture.Model, "")
			if err != nil {
				t.Fatal(err)
			}
			prompt := "free fixture"
			if kind == "video" {
				prompt = "free frozen contract fixture"
			}
			req := CreateTaskRequest{Prompt: prompt, Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID}}, Platform: &StudioTaskOptions{ExpectedPriceVersion: selected.Price.Version}}
			body, err := s.studioMediaBody(context.Background(), "user", req, selected)
			if err != nil {
				t.Fatal(err)
			}
			quote, err := s.studioPreviewMedia(t.Context(), session, req, selected, body, &platformclient.Response{Header: make(http.Header)}, false)
			if kind == "image" {
				if fixture.Preview.CombinationSupported || err == nil {
					t.Fatal("HTTP 200 with output_mapping_unverified must not authorize image generation")
				}
			} else if err != nil || quote.CapabilityRevision != fixture.Preview.CapabilityRevision || quote.ConnectionRevision != fixture.Preview.ConnectionRevision || quote.PriceVersion != selected.Price.Version {
				t.Fatalf("published preview versions lost: %+v %v", quote, err)
			}
			// Alternative variants remain separate, including when the default
			// rejects a value accepted by the other variant.
			var profile platformintegration.MediaProfile
			_ = json.Unmarshal(fixture.Model.MediaGenerationProfile, &profile)
			alternative, err := platformintegration.SelectModel(fixture.Model, profile.Variants[1].ID)
			if err != nil || selected.ID == alternative.ID {
				t.Fatal("alternative variant identity lost")
			}
			input := canvasGenerationInput{}
			if kind == "image" {
				input.Config.ImageParameters = map[string]any{"size": "2048x2048"}
			} else {
				input.Config.VideoSeconds, input.Config.VQuality = "8", "1080p"
			}
			if _, err := studioMediaSpecifications(input, selected); err == nil {
				t.Fatal("default variant incorrectly admitted alternative parameters")
			}
			if _, err := studioMediaSpecifications(input, alternative); err != nil {
				t.Fatalf("explicit alternative rejected: %v", err)
			}
		})
	}
	if t.Failed() {
		return
	}
	actual, _ := json.MarshalIndent(projections, "", "  ")
	actual = append(actual, '\n')
	path := filepath.Join(folder, "studio-projection.json")
	if os.Getenv("STUDIO_UPDATE_MODEL_FIXTURE") == "1" {
		if err := os.WriteFile(path, actual, 0600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil || string(want) != string(actual) {
		t.Fatal("Studio public projection changed; review and regenerate the cross-language consumer fixture")
	}
}
