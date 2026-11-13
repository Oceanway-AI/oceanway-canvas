package app

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func studioNativeSpecifications(input canvasGenerationInput, selected platformintegration.ModelSelection) (map[string]any, error) {
	v := selected.NativeRequest
	if input.Config.NativeVariantID != "" && input.Config.NativeVariantID != selected.ID {
		return nil, BadAuthRequest("模型能力选择不一致")
	}
	if len(input.Config.ImageParameters) > 0 {
		return nil, BadAuthRequest("请使用所选模型的原生参数表单")
	}
	var projected struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(selected.Schema, &projected) != nil {
		return nil, BadAuthRequest("模型参数合同无效")
	}
	values := map[string]any{}
	for k, val := range selected.Defaults {
		values[k] = val
	}
	for k, raw := range projected.Properties {
		var field map[string]any
		if json.Unmarshal(raw, &field) != nil {
			return nil, BadAuthRequest("模型参数合同无效")
		}
		if _, exists := values[k]; !exists {
			if value, ok := field["const"]; ok {
				values[k] = value
			}
		}
	}
	for k, val := range input.Config.NativeParameters {
		values[k] = val
	}
	for pointer := range values {
		if _, ok := projected.Properties[pointer]; !ok || selected.Bindings[pointer] != pointer || !v.ParameterAllowed(pointer) {
			return nil, BadAuthRequest("参数不属于所选模型的发布规格")
		}
	}
	if err := platformintegration.ValidateSpecifications(selected.Schema, values); err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	return values, nil
}

func (s *Service) studioNativeMediaBody(ctx context.Context, userID string, req CreateTaskRequest, input canvasGenerationInput, selected platformintegration.ModelSelection) ([]byte, error) {
	v := selected.NativeRequest
	values, err := studioNativeSpecifications(input, selected)
	if err != nil {
		return nil, err
	}
	// Clone before overlaying: neither a preview nor an Agent edit may mutate the
	// frozen catalog. Keep nested defaults and explicit false/zero/null intact.
	raw, err := json.Marshal(v.Defaults)
	if err != nil {
		return nil, BadAuthRequest("模型默认参数无效")
	}
	body := map[string]any{}
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return nil, BadAuthRequest("模型默认参数无效")
	}
	for _, pointer := range v.EnvelopePointers() {
		if _, exists := platformintegration.NativeValue(body, pointer); exists {
			return nil, BadAuthRequest("模型默认参数包含调用方内容")
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, pointer := range keys {
		value := values[pointer]
		if object, ok := value.(map[string]any); ok {
			previous, _ := platformintegration.NativeValue(body, pointer)
			base, _ := previous.(map[string]any)
			value = studioMergeNativeObjects(base, object)
		}
		if err := platformintegration.SetNativeValue(body, pointer, value); err != nil {
			return nil, BadAuthRequest(err.Error())
		}
	}
	if body["stream"] == true || body["async"] == true {
		return nil, BadAuthRequest("生成模式必须使用已发布的非流式协议")
	}
	switch v.Protocol {
	case "gemini.image":
		body["contents"] = []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": req.Prompt}}}}
	case "gemini.chat.image":
		body["model"] = selected.Name
		body["messages"] = []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": req.Prompt}}}}
	case "modelark.video":
		body["model"] = selected.Name
		body["content"] = []any{map[string]any{"type": "text", "text": req.Prompt}}
	default:
		body["model"], body["prompt"] = selected.Name, req.Prompt
	}
	if err := s.studioNativeMediaReferences(ctx, userID, v, input, body); err != nil {
		return nil, err
	}
	if err := platformintegration.ValidateSpecifications(v.Schema, body); err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	prepared, err := json.Marshal(body)
	if err != nil {
		return nil, BadAuthRequest("模型请求参数无效")
	}
	variantJSON, err := json.Marshal(v)
	if err != nil {
		return nil, BadAuthRequest("模型能力合同无效")
	}
	if err := platformclient.ValidateNativeImageSize(variantJSON, prepared); err != nil {
		return nil, BadAuthRequest("图片尺寸不符合所选型号的限制")
	}
	return prepared, nil
}

func studioMergeNativeObjects(base, override map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if object, ok := v.(map[string]any); ok {
			previous, _ := out[k].(map[string]any)
			out[k] = studioMergeNativeObjects(previous, object)
		} else {
			out[k] = v
		}
	}
	return out
}

func (s *Service) studioNativeMediaReferences(ctx context.Context, userID string, v *platformintegration.NativeRequestVariant, input canvasGenerationInput, body map[string]any) error {
	refs := append(append(append([]providerMedia{}, input.ReferenceImages...), input.ReferenceVideos...), input.ReferenceAudios...)
	if input.Mask != nil {
		mask := *input.Mask
		mask.Role = "mask"
		refs = append(refs, mask)
	}
	counts, totals := map[string]int64{}, map[string]int64{}
	resources := map[string][]*model.Resource{}
	for _, ref := range refs {
		if !strings.HasPrefix(ref.StorageKey, "resource:") || ref.DataURL != "" {
			return BadAuthRequest("参考素材必须先保存到平台素材库")
		}
		id := strings.TrimPrefix(ref.StorageKey, "resource:")
		resource, err := s.StudioResource(ctx, userID, id)
		if err != nil {
			return err
		}
		pin, err := s.repo.StudioMaterialReference(userID, id)
		if err != nil {
			return NotFound("素材引用不存在或不可访问")
		}
		role := ref.Role
		if role == "" || role == "reference" {
			role = "reference_" + resource.Kind
		}
		if role != "reference_"+resource.Kind && !(resource.Kind == "image" && slices.Contains([]string{"mask", "first_frame", "last_frame"}, role)) {
			return BadAuthRequest("素材角色与文件类型不一致")
		}
		var slot *platformintegration.NativeReferenceSlot
		for i := range v.References {
			candidate := &v.References[i]
			if candidate.Role != role {
				continue
			}
			if slot != nil {
				return BadAuthRequest("模型参考角色声明不明确")
			}
			slot = candidate
		}
		if slot == nil {
			return BadAuthRequest("所选模型不支持当前参考素材角色")
		}
		counts[slot.ID]++
		totals[slot.ID] += resource.Size
		if slot.MaxItems != nil && counts[slot.ID] > *slot.MaxItems || slot.MaxItemBytes != nil && resource.Size > *slot.MaxItemBytes || slot.MaxTotalBytes != nil && totals[slot.ID] > *slot.MaxTotalBytes || len(slot.MIMETypes) > 0 && !slices.Contains(slot.MIMETypes, resource.MimeType) {
			return BadAuthRequest("参考素材超出所选模型的文件限制")
		}
		resources[slot.Pointer] = append(resources[slot.Pointer], resource)
		uri := "material://" + pin.AssetID + "/" + pin.VersionID
		if err := studioAppendNativeReference(body, v, *slot, uri, resource.MimeType); err != nil {
			return err
		}
	}
	for _, slot := range v.References {
		if slot.MinItems != nil && counts[slot.ID] < *slot.MinItems {
			return BadAuthRequest("缺少所选模型要求的参考素材")
		}
		if slot.SameDimensionsAs != "" && counts[slot.ID] > 0 {
			targets := resources[slot.SameDimensionsAs]
			if len(targets) == 0 {
				return BadAuthRequest("参考素材尺寸约束缺少明确的目标")
			}
			for _, resource := range resources[slot.Pointer] {
				for _, target := range targets {
					if resource.Width <= 0 || resource.Height <= 0 || resource.Width != target.Width || resource.Height != target.Height {
						return BadAuthRequest("参考素材尺寸不一致或未完成尺寸校验")
					}
				}
			}
		}
	}
	return nil
}

func studioAppendNativeReference(body map[string]any, variant *platformintegration.NativeRequestVariant, slot platformintegration.NativeReferenceSlot, uri, mime string) error {
	var value any
	array := false
	switch slot.Transport {
	case "url":
		value = uri
	case "url-array":
		value, array = uri, true
	case "image-reference":
		value = map[string]any{"image_url": uri}
	case "image-reference-array":
		array = true
		if variant.MultipartReferenceImages(slot) {
			value = uri
		} else {
			value = map[string]any{"image_url": uri}
		}
	case "content-array":
		switch {
		case variant.Protocol == "modelark.video" && slot.Pointer == "/content":
			kind := "image"
			if slot.Role == "reference_video" {
				kind = "video"
			} else if slot.Role == "reference_audio" {
				kind = "audio"
			}
			value, array = map[string]any{"type": kind + "_url", kind + "_url": map[string]any{"url": uri}, "role": slot.Role}, true
		case variant.Protocol == "gemini.image" && slot.Pointer == "/contents":
			content := body["contents"].([]any)[0].(map[string]any)
			content["parts"] = append(content["parts"].([]any), map[string]any{"fileData": map[string]any{"fileUri": uri, "mimeType": mime}})
			return nil
		case variant.Protocol == "gemini.chat.image" && slot.Pointer == "/messages":
			message := body["messages"].([]any)[0].(map[string]any)
			message["content"] = append(message["content"].([]any), map[string]any{"type": "image_url", "image_url": map[string]any{"url": uri}})
			return nil
		default:
			return NewAppError(501, "此参考素材协议尚未接通")
		}
	default:
		return NewAppError(501, "此参考素材协议尚未接通")
	}
	old, exists := platformintegration.NativeValue(body, slot.Pointer)
	if array {
		items, ok := old.([]any)
		if exists && !ok {
			return BadAuthRequest("参考素材字段冲突")
		}
		value = append(items, value)
	} else if exists {
		return BadAuthRequest("参考素材字段冲突")
	}
	if err := platformintegration.SetNativeValue(body, slot.Pointer, value); err != nil {
		return BadAuthRequest(err.Error())
	}
	return nil
}
