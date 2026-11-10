package app

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

type StudioTaskOptions struct {
	VariantID                  string `json:"variantId,omitempty"`
	ExpectedCapabilityRevision string `json:"expectedCapabilityRevision,omitempty"`
	ExpectedConnectionRevision int64  `json:"expectedConnectionRevision,omitempty"`
	ExpectedPriceVersion       string `json:"expectedPriceVersion"`
	WalletMaxPoints            string `json:"walletMaxPoints,omitempty"`
	BillingPreference          string `json:"billingPreference,omitempty"`
}

var studioDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func studioValidDecimal(v string) bool { return len(v) <= 128 && studioDecimal.MatchString(v) }

func (s *Service) studioSelectTaskModel(ctx context.Context, session platformclient.SessionContext, req CreateTaskRequest, response *platformclient.Response) (platformintegration.ModelSelection, error) {
	catalog, err := s.studioPlatform.Client.Models(ctx, session)
	catalog.Response.AppendCookies(response.Header)
	if err != nil {
		return platformintegration.ModelSelection{}, platformIdentityError(err)
	}
	config, _ := req.Input["config"].(map[string]any)
	name := firstNonEmpty(stringValue(config["model"]), req.Model)
	variant := stringValue(config["nativeVariantId"])
	if req.Platform != nil {
		if variant != "" && req.Platform.VariantID != "" && variant != req.Platform.VariantID {
			return platformintegration.ModelSelection{}, BadAuthRequest("模型能力选择不一致")
		}
		variant = firstNonEmpty(req.Platform.VariantID, variant)
	}
	for _, m := range catalog.Data.Items {
		if m.ModelName == name {
			selected, err := platformintegration.SelectModel(m, variant)
			if err != nil {
				return selected, BadAuthRequest(err.Error())
			}
			if req.Type != "canvas_"+selected.Kind || strings.TrimSpace(req.Prompt) == "" {
				return selected, BadAuthRequest("任务类型或提示词无效")
			}
			return selected, nil
		}
	}
	return platformintegration.ModelSelection{}, BadAuthRequest("所选平台模型暂不可用，请刷新模型目录")
}

func (s *Service) StudioPreviewTask(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest) (*platformclient.ConsumptionPreview, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response := &platformclient.Response{Header: make(http.Header)}
	selected, err := s.studioSelectTaskModel(ctx, session, req, response)
	if err != nil {
		return nil, response, err
	}
	if selected.Kind == "text" {
		return nil, response, BadAuthRequest("文本按平台实际用量结算")
	}
	if err := s.ensureTaskProjectActive(userID, req.ProjectID); err != nil {
		return nil, response, err
	}
	body, err := s.studioMediaBody(ctx, userID, req, selected)
	if err != nil {
		return nil, response, err
	}
	quote, err := s.studioPreviewMedia(ctx, session, req, selected, body, response, false)
	return quote, response, err
}

func (s *Service) studioPreviewMedia(ctx context.Context, session platformclient.SessionContext, req CreateTaskRequest, selected platformintegration.ModelSelection, body []byte, response *platformclient.Response, requireCap bool) (*platformclient.ConsumptionPreview, error) {
	if req.Platform == nil || req.Platform.ExpectedPriceVersion != selected.Price.Version {
		return nil, NewAppError(409, "模型价格已更新，请刷新报价后再生成")
	}
	options := req.Platform
	if requireCap && (!studioSHA256.MatchString(options.ExpectedCapabilityRevision) || options.ExpectedConnectionRevision <= 0) {
		return nil, BadAuthRequest("请提供已确认的平台能力与连接版本")
	}
	if options.BillingPreference != "" && options.BillingPreference != "wallet-fallback" && options.BillingPreference != "only-benefits" {
		return nil, BadAuthRequest("资金使用方式无效")
	}
	if (requireCap || options.WalletMaxPoints != "") && !studioValidDecimal(options.WalletMaxPoints) {
		return nil, BadAuthRequest("请提供本次生成的平台积分上限")
	}
	quote, err := s.studioPlatform.Client.PreviewMedia(ctx, session, platformclient.PreviewRequest{Request: platformclient.MediaConsumption{Model: selected.Name, ModelType: selected.Kind, NativeProtocol: selected.GatewayProtocol, NativeRequest: json.RawMessage(body), ExpectedPriceVersion: options.ExpectedPriceVersion, BillingPreference: options.BillingPreference, WalletMaxPoints: options.WalletMaxPoints}})
	quote.Response.AppendCookies(response.Header)
	if err != nil {
		return nil, WrapAppError(503, "平台报价暂不可用，未提交生成请求", err)
	}
	q := quote.Data
	if q.Model != selected.Name || q.ModelType != selected.Kind || q.PriceVersion != options.ExpectedPriceVersion || !q.CombinationSupported || !q.Allowed || !studioValidDecimal(q.WalletPoints) || !studioSHA256.MatchString(q.CapabilityRevision) || q.ConnectionRevision <= 0 {
		return nil, NewAppError(409, "平台未批准当前参数或额度，请刷新报价")
	}
	if requireCap && (q.CapabilityRevision != options.ExpectedCapabilityRevision || q.ConnectionRevision != options.ExpectedConnectionRevision) {
		return nil, NewAppError(409, "平台能力或连接版本已变化，请重新确认报价")
	}
	if options.WalletMaxPoints != "" {
		cost, _ := new(big.Rat).SetString(q.WalletPoints)
		cap, _ := new(big.Rat).SetString(options.WalletMaxPoints)
		if cost.Cmp(cap) > 0 {
			return nil, NewAppError(409, "平台报价超过本次授权上限")
		}
	}
	return &q, nil
}

// Preview is the authority for all versions; the caller supplies only the
// confirmed wallet ceiling and billing preference. Persist this unchanged.
func studioMediaConsumption(preview *platformclient.ConsumptionPreview, options *StudioTaskOptions) *platformclient.MediaConsumptionConstraints {
	return &platformclient.MediaConsumptionConstraints{
		ExpectedCapabilityRevision: preview.CapabilityRevision,
		ExpectedConnectionRevision: preview.ConnectionRevision,
		ExpectedPriceVersion:       preview.PriceVersion,
		WalletMaxPoints:            options.WalletMaxPoints,
		BillingPreference:          options.BillingPreference,
	}
}

func studioMediaConsumptionValid(c *platformclient.MediaConsumptionConstraints) bool {
	return c != nil && studioSHA256.MatchString(c.ExpectedCapabilityRevision) && c.ExpectedConnectionRevision > 0 && c.ExpectedPriceVersion != "" && studioValidDecimal(c.WalletMaxPoints)
}

func studioDecodeInput(req CreateTaskRequest) (canvasGenerationInput, error) {
	var input canvasGenerationInput
	raw, err := json.Marshal(req.Input)
	if err != nil || json.Unmarshal(raw, &input) != nil {
		return input, BadAuthRequest("任务输入无效")
	}
	cfg := input.Config
	if cfg.ChannelID != StudioPlatformChannelID || cfg.APIKey != "" || cfg.SecretKey != "" || cfg.BaseURL != "" || len(cfg.Headers) > 0 || cfg.WorkflowID != "" || cfg.RunningHubWalletKey != "" || cfg.RunningHubUploadKey != "" || req.Provider != "" && req.Provider != "platform" || req.LogicalModelID != "" {
		return input, BadAuthRequest("Studio 仅支持已发布的平台模型")
	}
	return input, nil
}

func studioParameterName(key string) string {
	switch key {
	case "aspect_ratio":
		return "size"
	case "n", "images":
		return "count"
	case "resolution", "image_size":
		return "imageResolution"
	}
	return key
}

func studioMediaSpecifications(input canvasGenerationInput, selected platformintegration.ModelSelection) (map[string]any, error) {
	if selected.NativeRequest != nil {
		return studioNativeSpecifications(input, selected)
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(selected.Schema, &schema) != nil {
		return nil, NewAppError(502, "模型参数合同无效")
	}
	spec := map[string]any{}
	for k, v := range selected.Defaults {
		spec[k] = v
	}
	known := map[string]bool{}
	for key, field := range schema.Properties {
		if _, supplied := spec[key]; !supplied {
			if value := studioPublishedDefault(selected, key, field); value != nil {
				spec[key] = value
			}
		}
		name := studioParameterName(key)
		known[name] = true
		if value, ok := input.Config.ImageParameters[name]; ok && selected.Kind == "image" {
			spec[key] = value
			continue
		}
		var value string
		if selected.Kind == "video" {
			switch key {
			case "duration", "seconds":
				value = input.Config.VideoSeconds
			case "aspect_ratio", "ratio":
				value = input.Config.Size
			case "resolution":
				value = input.Config.VQuality
			case "generate_audio", "audio":
				value = input.Config.VideoGenerateAudio
			case "watermark":
				value = input.Config.VideoWatermark
			}
		}
		if value == "" {
			continue
		}
		switch field["type"] {
		case "integer":
			v, e := strconv.ParseInt(value, 10, 64)
			if e != nil {
				return nil, BadAuthRequest("视频参数无效")
			}
			spec[key] = v
		case "number":
			v, e := strconv.ParseFloat(value, 64)
			if e != nil {
				return nil, BadAuthRequest("视频参数无效")
			}
			spec[key] = v
		case "boolean":
			if value != "true" && value != "false" {
				return nil, BadAuthRequest("视频参数无效")
			}
			spec[key] = value == "true"
		default:
			spec[key] = value
		}
	}
	for key := range input.Config.ImageParameters {
		if !known[key] {
			return nil, BadAuthRequest("图片参数不属于所选发布规格")
		}
	}
	if err := platformintegration.ValidateSpecifications(selected.Schema, spec); err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	if spec["stream"] == true || spec["async"] == true {
		return nil, BadAuthRequest("当前仅支持可恢复的非流式生成")
	}
	return spec, nil
}

func (s *Service) studioMediaBody(ctx context.Context, userID string, req CreateTaskRequest, selected platformintegration.ModelSelection) ([]byte, error) {
	input, err := studioDecodeInput(req)
	if err != nil {
		return nil, err
	}
	if selected.NativeRequest != nil {
		return s.studioNativeMediaBody(ctx, userID, req, input, selected)
	}
	if len(input.Config.NativeParameters) > 0 || input.Config.NativeVariantID != "" {
		return nil, BadAuthRequest("所选模型未声明原生参数合同")
	}
	spec, err := studioMediaSpecifications(input, selected)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"model": selected.Name, "prompt": req.Prompt}
	switch selected.MediaRoute {
	case platformclient.GeminiMedia:
		body = map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": req.Prompt}}}}}
	case platformclient.DoubaoVideoGeneration:
		body = map[string]any{"model": selected.Name, "content": []any{map[string]any{"type": "text", "text": req.Prompt}}}
	}
	keys := make([]string, 0, len(spec))
	for k := range spec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := selected.Bindings[key]
		if path == "" {
			return nil, NewAppError(501, "模型参数缺少原生字段映射")
		}
		if key == "cached_content" || key == "tools" || key == "tool_config" {
			return nil, NewAppError(501, "此模型扩展能力尚未接通")
		}
		if err := studioSetNativeField(body, path, spec[key]); err != nil {
			return nil, err
		}
	}
	if err := s.studioMediaReferences(ctx, userID, selected, input, body); err != nil {
		return nil, err
	}
	return json.Marshal(body)
}

func studioSetNativeField(body map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	pointer := strings.HasPrefix(path, "/")
	if pointer {
		parts = strings.Split(path[1:], "/")
	}
	if len(path) > 512 || len(parts) > 6 {
		return NewAppError(501, "原生参数路径尚未接通")
	}
	// Validate the entire binding before touching the request. Parameter
	// bindings address objects; reference arrays belong to the material builder.
	for i, key := range parts {
		if pointer {
			for n := 0; n < len(key); n++ {
				if key[n] == '~' {
					if n+1 >= len(key) || key[n+1] != '0' && key[n+1] != '1' {
						return NewAppError(501, "原生参数路径尚未接通")
					}
					n++
				}
			}
			key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
		} else if !studioMaterialID.MatchString(key) {
			return NewAppError(501, "原生参数路径尚未接通")
		}
		if key == "" || key == "-" || strings.Trim(key, "0123456789") == "" || strings.ContainsAny(key, "\x00\r\n") {
			return NewAppError(501, "原生参数路径尚未接通")
		}
		parts[i] = key
	}
	// Caller content and managed references cannot be supplied as parameters.
	switch parts[0] {
	case "model", "prompt", "contents", "content", "messages", "image", "images", "mask", "input_reference", "reference_images", "first_frame_image", "last_frame_image":
		return NewAppError(501, "原生参数路径尚未接通")
	}
	for i, key := range parts {
		if i == len(parts)-1 {
			if _, exists := body[key]; exists {
				return NewAppError(501, "原生参数映射冲突")
			}
			body[key] = value
			return nil
		}
		existing, exists := body[key]
		next, ok := existing.(map[string]any)
		if !ok || next == nil {
			if exists {
				return NewAppError(501, "原生参数映射冲突")
			}
			next = map[string]any{}
			body[key] = next
		}
		body = next
	}
	return nil
}
