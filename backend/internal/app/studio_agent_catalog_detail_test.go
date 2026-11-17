package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioAgentSummaryDetailsFreezeAndMerge(t *testing.T) {
	testStudioAgentSummaryDetailsFreezeAndMerge(t, false)
}

func TestStudioAgentSummaryDetailsFreezeAndMergePostgres(t *testing.T) {
	testStudioAgentSummaryDetailsFreezeAndMerge(t, true)
}

func testStudioAgentSummaryDetailsFreezeAndMerge(t *testing.T, usePostgres bool) {
	t.Helper()
	var s *Service
	var db *gorm.DB
	if usePostgres {
		_, db = newProjectNameConflictPostgresService(t)
		s, db, _, _ = creationTestServiceOnDatabase(t, db)
	} else {
		s, db, _, _ = creationTestService(t)
	}
	if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	base := studioNativeFixtureModel(t, "modelark.video")
	var calls atomic.Int32
	var unavailable atomic.Bool
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/internal/relay/model-catalog/detail" || r.Header.Get("X-Platform-Execution-Authorization") != "exa_catalog" || r.Header.Get("Cookie") != "" {
			t.Errorf("detail used unexpected path or authority: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		if unavailable.Load() {
			w.WriteHeader(503)
			return
		}
		entered <- struct{}{}
		<-release
		if r.URL.Query().Get("expected_price_version") != "price-1" || r.URL.Query().Get("expected_capability_revision") != strings.Repeat("a", 64) || r.URL.Query().Get("expected_connection_revision") != "7" {
			t.Error("detail lost the frozen versions")
		}
		item := base
		item.ModelName = r.URL.Query().Get("model_name")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.ModelDetail{Version: platformclient.ModelCatalogVersion, State: "detail", CatalogRevision: "revision", ModelName: item.ModelName, VariantID: "choice", PriceVersion: "price-1", CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, Model: item}})
	}))
	defer server.Close()
	client, err := platformclient.NewClient(platformclient.Config{Origin: server.URL, Product: "studio", ProductKey: "fixture.secret", AllowedSessionHosts: []string{"studio.example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	s.studioPlatform = &platformintegration.Gateway{Client: client}
	summaries := []platformclient.ModelSummary{}
	for _, name := range []string{"video-a", "video-b"} {
		item := base
		item.ModelName, item.MediaGenerationProfile = name, nil
		summaries = append(summaries, platformclient.ModelSummary{Model: item, State: "summary", PriceVersion: "price-1", CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, VariantsComplete: true, Variants: []platformclient.ModelVariantSummary{{ID: "choice", ReferencesComplete: true}}})
	}
	frozen, err := s.encryptStudioValue(studioExecutionEnvelope{CatalogVersion: platformclient.ModelCatalogVersion, CatalogRevision: "revision", CatalogSummaries: summaries})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := s.encryptSettingSecret("exa_catalog")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioExecution{TaskID: "catalog-run", UserID: "user", State: "agent", EnvelopeEncrypted: frozen, AuthorizationEncrypted: auth}).Error; err != nil {
		t.Fatal(err)
	}
	call := cloudAgentCall{}
	call.Function.Arguments = `{"mode":"video"}`
	result, err := s.studioAgentModelList("user", &cloudAgentRuntime{RuntimeRunID: "catalog-run"}, call)
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["totalMatches"] != 2 || calls.Load() != 0 {
		t.Fatal("summary enumeration requested full contracts")
	}
	var wg sync.WaitGroup
	for _, name := range []string{"video-a", "video-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selected, err := s.studioAgentMediaSelection("user", "catalog-run", name, "choice")
			if err != nil || selected.Name != name {
				t.Errorf("selection %s failed: %v", name, err)
			}
		}()
	}
	<-entered
	<-entered
	close(release)
	wg.Wait()
	root, _ := s.repo.StudioExecution("user", "catalog-run")
	var envelope studioExecutionEnvelope
	if err := s.decryptStudioValue(root.EnvelopeEncrypted, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.CatalogDetails) != 2 || len(envelope.Catalog) != 0 {
		t.Fatal("concurrent detail writes lost frozen data")
	}
	unavailable.Store(true)
	for _, name := range []string{"video-a", "video-b"} {
		if _, err := s.studioAgentMediaSelection("user", "catalog-run", name, "choice"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("recovery reloaded a frozen contract")
	}
	if _, err := s.studioAgentCatalogDetail("other", "catalog-run", "video-a", "choice"); err == nil {
		t.Fatal("cross-user detail allowed")
	}
	if err := db.Model(&model.StudioExecution{}).Where("task_id = ?", "catalog-run").Update("released", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.studioAgentCatalogDetail("user", "catalog-run", "video-a", "choice"); err == nil {
		t.Fatal("released authorization accepted")
	}
}

func TestStudioAgentNativeAliasesPreserveExplicitIntent(t *testing.T) {
	selected, err := platformintegration.SelectModel(studioNativeFixtureModel(t, "modelark.video"), "choice")
	if err != nil {
		t.Fatal(err)
	}
	audio := false
	args := cloudAgentMediaArgs{Mode: "video", Duration: 8, Quality: "1080p", Size: "9:16", VideoGenerateAudio: &audio}
	values, err := studioAgentNativeParameters(args, selected, map[string]any{"/seed": 0})
	if err != nil {
		t.Fatal(err)
	}
	if values["/duration"] != 8 || values["/resolution"] != "1080p" || values["/ratio"] != "9:16" || values["/generate_audio"] != false || values["/seed"] != 0 {
		t.Fatalf("native aliases changed: %#v", values)
	}
	if _, err := studioAgentNativeParameters(args, selected, map[string]any{"/duration": 4}); err == nil {
		t.Fatal("conflicting native duration silently won")
	}
	if _, err := studioAgentNativeParameters(cloudAgentMediaArgs{Mode: "video", ImageResolution: "2K"}, selected, nil); err == nil {
		// 2K exists as an alias, but the selected schema rejects it next.
		bad, _ := studioAgentNativeParameters(cloudAgentMediaArgs{Mode: "video", ImageResolution: "2K"}, selected, nil)
		if _, err := studioNativeSpecifications(canvasGenerationInput{Config: providerConfig{NativeVariantID: selected.ID, NativeParameters: bad}}, selected); err == nil {
			t.Fatal("invalid alias bypassed schema")
		}
	}
}

func TestStudioFrameRolesRejectConflictAndRemainIdempotent(t *testing.T) {
	input := canvasGenerationInput{ReferenceImages: []providerMedia{{ID: "frame", StorageKey: "resource:r", Role: "first_frame"}, {ID: "frame", StorageKey: "resource:r", Role: "last_frame"}}, Metadata: map[string]any{"videoStartFrameNodeId": "frame", "videoEndFrameNodeId": "frame"}}
	for i := 0; i < 2; i++ {
		if err := studioResolveVideoFrameReferences(&input); err != nil {
			t.Fatal(err)
		}
		if len(input.ReferenceImages) != 2 {
			t.Fatal("repeated decoding duplicated frames")
		}
	}
	input.ReferenceImages[0].Role = "mask"
	if err := studioResolveVideoFrameReferences(&input); err == nil {
		t.Fatal("explicit conflicting role was overwritten")
	}
}
