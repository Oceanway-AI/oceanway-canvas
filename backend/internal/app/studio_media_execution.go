package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	ExpectedUserID             string `json:"expectedUserId,omitempty"`
	WorkspaceID                string `json:"workspaceId,omitempty"`
	PreviewUserID              string `json:"previewUserId,omitempty"`
	PreviewBinding             string `json:"previewBinding,omitempty"`
	VariantID                  string `json:"variantId,omitempty"`
	ExpectedCapabilityRevision string `json:"expectedCapabilityRevision,omitempty"`
	ExpectedConnectionRevision int64  `json:"expectedConnectionRevision,omitempty"`
	ExpectedPriceVersion       string `json:"expectedPriceVersion"`
	WalletMaxPoints            string `json:"walletMaxPoints,omitempty"`
	BillingPreference          string `json:"billingPreference,omitempty"`
}

var studioDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func studioValidDecimal(v string) bool { return len(v) <= 128 && studioDecimal.MatchString(v) }

func (o *StudioTaskOptions) UnmarshalJSON(data []byte) error {
	// Only an omitted limit selects actual-usage billing. Reject a malformed
	// explicit limit before it can be decoded as the empty string.
	type options StudioTaskOptions
	var wire struct {
		options
		WalletMaxPoints json.RawMessage `json:"walletMaxPoints"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.WalletMaxPoints != nil {
		if err := json.Unmarshal(wire.WalletMaxPoints, &wire.options.WalletMaxPoints); err != nil || !studioValidDecimal(wire.options.WalletMaxPoints) {
			return errors.New("平台积分上限无效")
		}
	}
	*o = StudioTaskOptions(wire.options)
	return nil
}

func (s *Service) studioSelectTaskModel(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest, response *platformclient.Response, scopes ...studioCatalogScope) (platformintegration.ModelSelection, error) {
	config, _ := req.Input["config"].(map[string]any)
	name := firstNonEmpty(stringValue(config["model"]), req.Model)
	variant := stringValue(config["nativeVariantId"])
	if req.Platform != nil {
		if variant != "" && req.Platform.VariantID != "" && variant != req.Platform.VariantID {
			return platformintegration.ModelSelection{}, BadAuthRequest("模型能力选择不一致")
		}
		variant = firstNonEmpty(req.Platform.VariantID, variant)
	}
	var scope studioCatalogScope
	selectModel := func(m platformclient.Model) (platformintegration.ModelSelection, error) {
		if req.Type != "canvas_"+m.ModelType || strings.TrimSpace(req.Prompt) == "" {
			return platformintegration.ModelSelection{}, BadAuthRequest("任务类型或提示词无效")
		}
		if variant == "" && (m.ModelType == "image" || m.ModelType == "video") {
			if profile, choices := studioNativeChoices(m); choices {
				input, err := studioDecodeInput(req)
				if err != nil {
					return platformintegration.ModelSelection{}, err
				}
				refs, err := s.studioNativeReferenceResources(ctx, userID, input)
				if err != nil {
					return platformintegration.ModelSelection{}, err
				}
				return studioChooseNativeVariant(m, profile, func(selected platformintegration.ModelSelection) ([]byte, error) {
					values, err := studioNativeSpecifications(input, selected)
					if err != nil {
						return nil, err
					}
					return studioNativeResolvedBody(req.Prompt, selected, values, refs)
				})
			}
		}
		selected, err := platformintegration.SelectModel(m, variant)
		if err != nil {
			return selected, BadAuthRequest(err.Error())
		}
		return selected, nil
	}
	if len(scopes) == 1 {
		scope = scopes[0]
		if req.Platform != nil {
			scope.workspace = req.Platform.WorkspaceID
		}
		// An exact variant and price identify the schema shown on this signed-in
		// page. Platform still checks publication, current price, entitlement
		// and any capability/connection constraints when it accepts the command.
		pinned := req.Platform != nil && studioSHA256.MatchString(variant) && studioSHA256.MatchString(req.Platform.ExpectedPriceVersion)
		if m, ok := s.studioMediaCatalog.lookup(scope, name, pinned); ok {
			selected, err := selectModel(m)
			var revisions struct {
				Capability string `json:"revision"`
				Connection int64  `json:"connection_revision"`
			}
			_ = json.Unmarshal(m.MediaGenerationProfile, &revisions)
			versionsMatch := req.Platform == nil || (req.Platform.ExpectedPriceVersion == "" || req.Platform.ExpectedPriceVersion == selected.Price.Version) && (req.Platform.ExpectedCapabilityRevision == "" || req.Platform.ExpectedCapabilityRevision == revisions.Capability) && (req.Platform.ExpectedConnectionRevision == 0 || req.Platform.ExpectedConnectionRevision == revisions.Connection)
			if err == nil && selected.GenerationV1 && versionsMatch {
				return selected, nil
			}
		}
	}
	query := platformclient.ModelDetailQuery{ModelName: name, VariantID: variant}
	if req.Platform != nil {
		query.WorkspaceID = req.Platform.WorkspaceID
		query.ExpectedPriceVersion = req.Platform.ExpectedPriceVersion
		query.ExpectedCapabilityRevision = req.Platform.ExpectedCapabilityRevision
		query.ExpectedConnectionRevision = req.Platform.ExpectedConnectionRevision
	}
	// Existing saved requests may predate summary revisions. Resolve only their
	// named model, then require that exact price/capability/connection for detail.
	if query.ExpectedPriceVersion == "" || req.Type != "canvas_text" && (query.ExpectedCapabilityRevision == "" || query.ExpectedConnectionRevision == 0) {
		pageQuery := platformclient.ModelSummaryQuery{WorkspaceID: query.WorkspaceID, Query: name, Limit: 50}
		found := false
		seenCursors := map[string]bool{}
		revision := ""
		for !found {
			page, err := s.studioPlatform.Client.ModelSummaries(ctx, session, pageQuery)
			page.Response.AppendCookies(response.Header)
			if err != nil {
				return platformintegration.ModelSelection{}, studioCatalogError(err)
			}
			if page.Data.Version != platformclient.ModelCatalogVersion || page.Data.State != "summary" || page.Data.CatalogRevision == "" || revision != "" && revision != page.Data.CatalogRevision {
				return platformintegration.ModelSelection{}, NewAppError(409, "模型目录已更新，请刷新后继续")
			}
			revision = page.Data.CatalogRevision
			for _, summary := range page.Data.Items {
				if summary.ModelName == name {
					resolved := studioSummaryDetailQuery(summary, variant)
					resolved.WorkspaceID = query.WorkspaceID
					if query.ExpectedPriceVersion != "" {
						resolved.ExpectedPriceVersion = query.ExpectedPriceVersion
					}
					if query.ExpectedCapabilityRevision != "" {
						resolved.ExpectedCapabilityRevision = query.ExpectedCapabilityRevision
					}
					if query.ExpectedConnectionRevision != 0 {
						resolved.ExpectedConnectionRevision = query.ExpectedConnectionRevision
					}
					query, found = resolved, true
					break
				}
			}
			if !found && (!page.Data.HasMore || page.Data.NextCursor == "" || seenCursors[page.Data.NextCursor]) {
				return platformintegration.ModelSelection{}, BadAuthRequest("所选平台模型暂不可用，请刷新模型目录")
			}
			seenCursors[page.Data.NextCursor] = true
			pageQuery.Cursor = page.Data.NextCursor
		}
	}
	detail, err := s.studioPlatform.Client.ModelDetail(ctx, session, query)
	detail.Response.AppendCookies(response.Header)
	if err != nil {
		return platformintegration.ModelSelection{}, studioCatalogError(err)
	}
	if err := studioValidateCatalogDetail(detail.Data, query); err != nil {
		return platformintegration.ModelSelection{}, err
	}
	if query.VariantID == "" {
		s.studioMediaCatalog.rememberDetail(scope, detail.Data.Model)
	}
	return selectModel(detail.Data.Model)
}

type StudioTaskPreview struct {
	*platformclient.ConsumptionPreview
	PreviewUserID  string `json:"previewUserId"`
	PreviewBinding string `json:"previewBinding"`
}

func studioPreviewAccountChanged() *AppError {
	err := NewAppError(http.StatusConflict, "请使用当前账号重新预估并确认消费")
	err.Reason = "studio_preview_account_changed"
	return err
}

// This first component can be verified before a catalog or database read.
func (s *Service) studioPreviewBinding(userID, commandID string) (string, error) {
	key, err := s.settingsEncryptionKey()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("studio-media-preview-v1\n" + userID + "\n" + commandID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Service) studioSelectionBinding(userID string, req CreateTaskRequest, selected platformintegration.ModelSelection, body []byte, price, capability string, connection int64) (string, error) {
	key, err := s.settingsEncryptionKey()
	if err != nil {
		return "", err
	}
	o := req.Platform
	nativeID := ""
	if v := selected.NativeRequest; v != nil {
		nativeID = v.ID
	}
	data, err := json.Marshal([]any{"studio-media-selection-v2", userID, req.CommandID, req.ProjectID, o.WorkspaceID,
		selected.Name, selected.ID, nativeID, selected.GatewayProtocol, selected.MediaRoute, selected.GenerationV1,
		sha256.Sum256(body), price, capability, connection, o.BillingPreference, o.WalletMaxPoints})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Service) verifyStudioPreviewAccount(userID string, req CreateTaskRequest) error {
	if req.Platform == nil || req.Platform.PreviewUserID != userID || req.Platform.PreviewBinding == "" {
		return studioPreviewAccountChanged()
	}
	expected, err := s.studioPreviewBinding(userID, req.CommandID)
	if err != nil {
		return err
	}
	account, _, _ := strings.Cut(req.Platform.PreviewBinding, ".")
	if !hmac.Equal([]byte(expected), []byte(account)) {
		return studioPreviewAccountChanged()
	}
	return nil
}

func (s *Service) verifyStudioPreviewSelection(userID string, req CreateTaskRequest, selected platformintegration.ModelSelection, body []byte) error {
	o := req.Platform
	if o == nil || o.PreviewBinding == "" {
		return nil
	}
	if !studioSHA256.MatchString(o.ExpectedCapabilityRevision) || o.ExpectedConnectionRevision <= 0 {
		return BadAuthRequest("请提供已确认的平台能力与连接版本")
	}
	expected, err := s.studioSelectionBinding(userID, req, selected, body, o.ExpectedPriceVersion, o.ExpectedCapabilityRevision, o.ExpectedConnectionRevision)
	if err != nil {
		return err
	}
	_, signature, _ := strings.Cut(o.PreviewBinding, ".")
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		e := NewAppError(http.StatusConflict, "模型、参数、素材或报价已变更，请重新预估并确认消费")
		e.Reason = "studio_preview_selection_changed"
		return e
	}
	return nil
}

func (s *Service) StudioPreviewTask(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest, verifiedSessionID ...string) (*StudioTaskPreview, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response := &platformclient.Response{Header: make(http.Header)}
	if !studioMaterialID.MatchString(req.CommandID) {
		return nil, response, BadAuthRequest("请提供稳定的任务命令标识")
	}
	selected, err := s.studioSelectTaskModel(ctx, userID, session, req, response, studioCatalogScopeFor(userID, session, verifiedSessionID...))
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
	if err != nil {
		return nil, response, err
	}
	binding, err := s.studioPreviewBinding(userID, req.CommandID)
	if err != nil {
		return nil, response, err
	}
	selection, err := s.studioSelectionBinding(userID, req, selected, body, quote.PriceVersion, quote.CapabilityRevision, quote.ConnectionRevision)
	if err != nil {
		return nil, response, err
	}
	return &StudioTaskPreview{ConsumptionPreview: quote, PreviewUserID: userID, PreviewBinding: binding + "." + selection}, response, nil
}

func (s *Service) studioPreviewMedia(ctx context.Context, session platformclient.SessionContext, req CreateTaskRequest, selected platformintegration.ModelSelection, body []byte, response *platformclient.Response, requireRevisions bool) (*platformclient.ConsumptionPreview, error) {
	if req.Platform == nil || req.Platform.ExpectedPriceVersion != selected.Price.Version {
		return nil, NewAppError(409, "模型价格已更新，请刷新报价后再生成")
	}
	options := req.Platform
	if requireRevisions && (!studioSHA256.MatchString(options.ExpectedCapabilityRevision) || options.ExpectedConnectionRevision <= 0) {
		return nil, BadAuthRequest("请提供已确认的平台能力与连接版本")
	}
	if options.BillingPreference != "" && options.BillingPreference != "wallet-fallback" && options.BillingPreference != "only-benefits" {
		return nil, BadAuthRequest("资金使用方式无效")
	}
	if options.WalletMaxPoints != "" && !studioValidDecimal(options.WalletMaxPoints) {
		return nil, BadAuthRequest("平台积分上限无效")
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
	if requireRevisions && (q.CapabilityRevision != options.ExpectedCapabilityRevision || q.ConnectionRevision != options.ExpectedConnectionRevision) {
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

// The preview owns the revisions. WalletMaxPoints comes only from the request,
// since a Platform estimate may be lower than the final usage.
func studioMediaConsumption(preview *platformclient.ConsumptionPreview, options *StudioTaskOptions) *platformclient.MediaConsumptionConstraints {
	return &platformclient.MediaConsumptionConstraints{
		ExpectedCapabilityRevision: preview.CapabilityRevision,
		ExpectedConnectionRevision: preview.ConnectionRevision,
		ExpectedPriceVersion:       preview.PriceVersion,
		WalletMaxPoints:            options.WalletMaxPoints,
		BillingPreference:          options.BillingPreference,
	}
}

func (e *studioExecutionEnvelope) validMediaConsumption() bool {
	c := e.Consumption
	if c == nil || !studioSHA256.MatchString(c.ExpectedCapabilityRevision) || c.ExpectedConnectionRevision <= 0 || c.ExpectedPriceVersion == "" {
		return false
	}
	// Older envelopes only granted a specific amount. A missing limit in one
	// of those records cannot authorize actual-usage billing during recovery.
	if e.ActualUsageBilling {
		return c.WalletMaxPoints == "" && e.AgentRunID == e.Task.AgentRunID
	}
	return studioValidDecimal(c.WalletMaxPoints)
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
	if err := studioResolveVideoFrameReferences(&input); err != nil {
		return input, err
	}
	return input, nil
}

func studioResolveVideoFrameReferences(input *canvasGenerationInput) error {
	start := metadataString(input.Metadata, "videoStartFrameNodeId")
	end := metadataString(input.Metadata, "videoEndFrameNodeId")
	if start == "" && end == "" {
		return nil
	}
	foundStart, foundEnd := start == "", end == ""
	images := make([]providerMedia, 0, len(input.ReferenceImages)+1)
	frames := map[string]providerMedia{}
	for _, ref := range input.ReferenceImages {
		if ref.ID != start && ref.ID != end {
			images = append(images, ref)
			continue
		}
		if ref.Role != "" && ref.Role != "reference" && ref.Role != "reference_image" && !(ref.ID == start && ref.Role == "first_frame") && !(ref.ID == end && ref.Role == "last_frame") {
			return BadAuthRequest("视频首尾帧选择与图片参考角色冲突")
		}
		identity := ref
		identity.Role = ""
		if previous, ok := frames[ref.ID]; ok {
			if previous != identity {
				return BadAuthRequest("同一首尾帧节点存在不同参考内容")
			}
			continue
		}
		frames[ref.ID] = identity
		if ref.ID == start {
			frame := ref
			frame.Role = "first_frame"
			images = append(images, frame)
			foundStart = true
		}
		if ref.ID == end {
			frame := ref
			frame.Role = "last_frame"
			images = append(images, frame)
			foundEnd = true
		}
	}
	if !foundStart || !foundEnd {
		return BadAuthRequest("视频首尾帧必须包含在本次图片参考中")
	}
	input.ReferenceImages = images
	return nil
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
