package app

import (
	"encoding/json"
	"strings"
	"testing"
	"yingce/backend/internal/testdb"

	"yingce/backend/internal/model"
	"yingce/backend/internal/outbound/outboundtest"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

func TestChannelFromRequestStoresConnectionWithoutDefaultProtocol(t *testing.T) {
	channel, err := channelFromRequest(ChannelRequest{
		Name:             "混合模型渠道",
		BaseURL:          "https://8.8.8.8/v1",
		APIKey:           "access-key",
		SecretKey:        "secret-key",
		ConcurrencyLimit: intPtr(6),
		Models:           []string{"seedance-2.0"},
	}, model.ModelChannel{})
	if err != nil {
		t.Fatalf("channelFromRequest() error = %v", err)
	}
	if channel.APIFormat != "openai" {
		t.Fatalf("APIFormat = %q, want openai", channel.APIFormat)
	}
	if channel.ConcurrencyLimit != 6 {
		t.Fatalf("ConcurrencyLimit = %d, want 6", channel.ConcurrencyLimit)
	}
	if channel.APIKey != "access-key" || channel.SecretKey != "secret-key" {
		t.Fatal("channel credentials were not stored")
	}
}

func TestMergeChannelRequestSupportsEnabledOnlyPatch(t *testing.T) {
	enabled := false
	req := mergeChannelRequest(ChannelRequest{Enabled: &enabled}, model.ModelChannel{
		Name:        "Video",
		BaseURL:     "https://example.com/v1",
		APIFormat:   "openai",
		ModelsJSON:  `["custom-video"]`,
		HeadersJSON: `[{"name":"User-Agent","value":"Stored Agent"}]`,
	})
	if req.Name != "Video" || req.BaseURL != "https://example.com/v1" || len(req.Models) != 1 || len(req.Headers) != 1 {
		t.Fatalf("mergeChannelRequest() = %#v", req)
	}
}

func TestChannelFromRequestStoresAndClearsHeaders(t *testing.T) {
	// URL validation is real, but this non-network test must not depend on DNS.
	request := ChannelRequest{Name: "Headers", BaseURL: "https://93.184.216.34/v1", Headers: []OutboundHeader{{Name: "User-Agent", Value: "Custom Agent"}}}
	channel, err := channelFromRequest(request, model.ModelChannel{})
	if err != nil {
		t.Fatal(err)
	}
	if channel.HeadersJSON != `[{"name":"User-Agent","value":"Custom Agent"}]` {
		t.Fatalf("HeadersJSON = %q", channel.HeadersJSON)
	}

	request.Headers = []OutboundHeader{}
	channel, err = channelFromRequest(request, channel)
	if err != nil {
		t.Fatal(err)
	}
	if channel.HeadersJSON != `[]` {
		t.Fatalf("cleared HeadersJSON = %q", channel.HeadersJSON)
	}
}

func TestChannelFromRequestValidatesProxyURL(t *testing.T) {
	limit := 4
	base := model.ModelChannel{}
	channel, err := channelFromRequest(ChannelRequest{Name: "Proxy", BaseURL: "https://93.184.216.34/v1", ProxyURL: stringPtr("socks5://user:secret@127.0.0.1:1080"), ConcurrencyLimit: &limit}, base)
	if err != nil {
		t.Fatal(err)
	}
	if channel.ProxyURL != "socks5://user:secret@127.0.0.1:1080" {
		t.Fatalf("proxy URL = %q", channel.ProxyURL)
	}
	for _, proxyURL := range []string{"ftp://127.0.0.1:1080", "socks5://", "socks5://127.0.0.1:1080#fragment", "http://" + strings.Repeat("a", 506)} {
		_, err := channelFromRequest(ChannelRequest{Name: "Proxy", BaseURL: "https://93.184.216.34/v1", ProxyURL: stringPtr(proxyURL), ConcurrencyLimit: &limit}, base)
		if err == nil {
			t.Fatalf("invalid proxy URL %q was accepted", proxyURL)
		}
	}
}

// 复现线上问题：管理端编辑渠道时把「代理地址」删掉（前端提交 proxyUrl:""）必须真的清空；
// 未提交该字段（nil）才保持渠道现有值。此前 ProxyURL 是 string 类型，
// mergeChannelRequest 无法区分「空串」与「没传」，一律回填旧值 → 永远清不掉。
func TestMergeChannelRequestProxyURLPointerSemantics(t *testing.T) {
	stored := model.ModelChannel{
		Name:       "Proxy",
		BaseURL:    "https://93.184.216.34/v1",
		APIFormat:  "openai",
		ModelsJSON: `[]`,
		ProxyURL:   "http://127.0.0.1:10080",
	}

	cleared := mergeChannelRequest(ChannelRequest{ProxyURL: stringPtr("")}, stored)
	if cleared.ProxyURL == nil || *cleared.ProxyURL != "" {
		t.Fatalf("cleared proxy URL = %v, want pointer to empty string", cleared.ProxyURL)
	}

	disabled := false
	kept := mergeChannelRequest(ChannelRequest{Enabled: &disabled}, stored)
	if kept.ProxyURL == nil || *kept.ProxyURL != stored.ProxyURL {
		t.Fatalf("omitted proxy URL = %v, want %q kept", kept.ProxyURL, stored.ProxyURL)
	}
}

func TestChannelFromRequestClearsProxyURL(t *testing.T) {
	limit := 4
	base := model.ModelChannel{ProxyURL: "http://127.0.0.1:10080"}
	request := ChannelRequest{Name: "Proxy", BaseURL: "https://93.184.216.34/v1", ConcurrencyLimit: &limit}

	clearedReq := request
	clearedReq.ProxyURL = stringPtr("")
	cleared, err := channelFromRequest(clearedReq, base)
	if err != nil {
		t.Fatalf("channelFromRequest() clear proxy error: %v", err)
	}
	if cleared.ProxyURL != "" {
		t.Fatalf("cleared proxy URL = %q, want empty", cleared.ProxyURL)
	}

	// 未提交该字段时不应保留传入 base 上的旧值以外的东西：包级入口下 base 值会被
	// 显式赋空，实际链路依赖 mergeChannelRequest 先回填（见上一个用例）。
	omitted, err := channelFromRequest(request, model.ModelChannel{})
	if err != nil {
		t.Fatalf("channelFromRequest() omitted proxy error: %v", err)
	}
	if omitted.ProxyURL != "" {
		t.Fatalf("omitted proxy URL = %q, want empty", omitted.ProxyURL)
	}
}

func TestPublicChannelOnlyReturnsSystemHeadersAndProxyToAdmin(t *testing.T) {
	channel := model.ModelChannel{ID: "system-1", Scope: model.ChannelScopeSystem, BaseURL: "https://example.com/v1", HeadersJSON: `[{"name":"X-Gateway-Tenant","value":"tenant-a"}]`, ProxyURL: "socks5://user:secret@127.0.0.1:1080"}
	adminView := publicChannel(channel, true, nil)
	if len(adminView.Headers) != 1 || adminView.Headers[0].Name != "X-Gateway-Tenant" {
		t.Fatalf("admin headers = %#v", adminView.Headers)
	}
	if adminView.ProxyURL != channel.ProxyURL {
		t.Fatalf("admin proxy URL = %q", adminView.ProxyURL)
	}
	userView := publicChannel(channel, false, nil)
	if len(userView.Headers) != 0 {
		t.Fatalf("user headers = %#v", userView.Headers)
	}
	if userView.ProxyURL != "" {
		t.Fatalf("user proxy URL = %q", userView.ProxyURL)
	}
}

func TestChannelFromRequestRejectsInvalidConcurrencyLimit(t *testing.T) {
	for _, limit := range []int{0, 1000} {
		_, err := channelFromRequest(ChannelRequest{Name: "Bad", BaseURL: "https://example.com/v1", ConcurrencyLimit: &limit}, model.ModelChannel{})
		if err == nil {
			t.Fatalf("channelFromRequest() concurrencyLimit = %d, error = nil", limit)
		}
	}
}

func TestRuntimeConcurrencyUsesEnvironmentFallback(t *testing.T) {
	t.Setenv("CANVAS_CHANNEL_CONCURRENCY", "7")
	t.Setenv("CANVAS_WORKER_CONCURRENCY", "9")
	setting := defaultRuntimePolicy().Task
	if setting.ChannelConcurrency != 7 || setting.WorkerConcurrency != 9 {
		t.Fatalf("runtimeConcurrencyFromEnvironment() = %#v", setting)
	}

	useGlobal := true
	channel, err := channelFromRequest(ChannelRequest{Name: "Global", BaseURL: "https://93.184.216.34/v1", UseGlobalConcurrency: &useGlobal}, model.ModelChannel{ConcurrencyLimit: 4})
	if err != nil || channel.ConcurrencyLimit != 0 {
		t.Fatalf("global concurrency channel = %#v, error = %v", channel, err)
	}
}

func TestResolveProviderConfigMapsSKUToProviderModel(t *testing.T) {
	outboundtest.PublicDNS(t, "ark.cn-beijing.volces.com")
	svc, db := newChannelModelTestService(t)
	svc.dataDir = t.TempDir()
	channel := model.ModelChannel{
		ID: "channel-1", Scope: model.ChannelScopeSystem, Enabled: true, Name: "Seedance",
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3", APIKey: "test-key", APIFormat: "openai", ModelsJSON: `["seedance-2-5-480p"]`,
	}
	if err := svc.encryptSystemChannelSecrets(&channel); err != nil {
		t.Fatal(err)
	}
	item := model.ChannelModel{
		ID: "model-1", ChannelID: channel.ID, ModelKey: "seedance-2-5-480p", ProviderModelKey: "doubao-seedance-2-5",
		Capability: "video", Protocol: model.ChannelInterfaceVolcengineArkVideo, Enabled: true,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}

	config, err := svc.resolveProviderConfig(providerConfig{ChannelID: channel.ID, Model: item.ModelKey})
	if err != nil {
		t.Fatal(err)
	}
	if config.ChannelModelKey != item.ModelKey || config.Model != item.ProviderModelKey {
		t.Fatalf("resolved config = %#v", config)
	}
}

func TestValidateTaskCapabilityFixesSingleResolutionSKU(t *testing.T) {
	svc, db := newChannelModelTestService(t)
	video := DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceVolcengineArkVideo), "doubao-seedance-2-5").Video
	video.References.MaxVideos = 0
	video.Resolutions = []string{"480p"}
	video.DefaultResolution = "480p"
	encoded, err := json.Marshal(&ModelCapabilityConfig{Version: 1, Video: video})
	if err != nil {
		t.Fatal(err)
	}
	channelModel := model.ChannelModel{
		ID: "model-480p", ChannelID: "channel-1", ModelKey: "doubao-seedance-2-5-480p", ProviderModelKey: "doubao-seedance-2-5",
		Capability: "video", Protocol: model.ChannelInterfaceVolcengineArkVideo, Enabled: true, CapabilityConfigJSON: string(encoded),
	}
	if err := db.Create(&channelModel).Error; err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"mode": "video",
		"config": map[string]any{
			"channelId": "channel-1", "model": channelModel.ModelKey, "vquality": "auto", "videoSeconds": "6", "size": "16:9",
			"videoGenerateAudio": "true", "videoWatermark": "false",
		},
	}
	if err := svc.ValidateTaskCapability(input); err != nil {
		t.Fatal(err)
	}
	config := input["config"].(map[string]any)
	if got := config["vquality"]; got != "480p" {
		t.Fatalf("vquality = %#v, want 480p", got)
	}
}

func newChannelModelTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ModelChannel{}, &model.ChannelModel{}, &model.ChannelModelPriceTier{}, &model.LogicalModel{}, &model.LogicalModelRevision{}, &model.LogicalModelRoute{}, &model.Task{}, &model.IDSequence{}, &model.User{}, &model.UserGroup{}, &model.UserGroupChannel{}, &model.SystemSetting{}); err != nil {
		t.Fatal(err)
	}
	return &Service{repo: repository.New(db)}, db
}

func intPtr(value int) *int { return &value }
