package app

import (
	"context"
	"encoding/json"
	"maps"
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
	var projected studioNativeSchemaNode
	if json.Unmarshal(selected.Schema, &projected) != nil {
		return nil, BadAuthRequest("模型参数合同无效")
	}
	values := map[string]any{}
	for k, val := range input.Config.NativeParameters {
		values[k] = val
	}
	spec := selected.Schema
	if _, chosen := values["/size"]; !chosen && v.OutputSize != nil && slices.Contains(projected.Required, "/size") {
		// The matching supply provides the required size after materials and
		// explicit settings have been assembled.
		projected.Required = slices.DeleteFunc(projected.Required, func(p string) bool { return p == "/size" })
		var schema map[string]json.RawMessage
		if err := json.Unmarshal(spec, &schema); err != nil {
			return nil, BadAuthRequest("模型参数合同无效")
		}
		schema["required"], _ = json.Marshal(projected.Required)
		spec, _ = json.Marshal(schema)
	}
	studioFillNativeRequired(projected, values, selected.Defaults)
	for pointer := range values {
		if _, ok := projected.Properties[pointer]; !ok || selected.Bindings[pointer] != pointer || !v.ParameterAllowed(pointer) {
			return nil, BadAuthRequest("参数不属于所选模型的发布规格")
		}
	}
	if err := platformintegration.ValidateSpecifications(spec, values); err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	return values, nil
}

type studioNativeSchemaNode struct {
	Properties map[string]studioNativeSchemaNode `json:"properties"`
	Required   []string                          `json:"required"`
	Const      json.RawMessage                   `json:"const"`
	Enum       []json.RawMessage                 `json:"enum"`
	Default    json.RawMessage                   `json:"default"`
}

func studioFillNativeRequired(schema studioNativeSchemaNode, body, defaults map[string]any) {
	keys := make([]string, 0, len(schema.Properties)+len(schema.Required))
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	for _, key := range schema.Required {
		if _, declared := schema.Properties[key]; !declared {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		child := schema.Properties[key]
		value, supplied := body[key]
		if !supplied && !slices.Contains(schema.Required, key) {
			continue
		}
		if !supplied {
			raw := child.Const
			if len(raw) == 0 && len(child.Enum) == 1 {
				raw = child.Enum[0]
			}
			if len(raw) == 0 {
				raw = child.Default
			}
			if len(raw) > 0 {
				supplied = json.Unmarshal(raw, &value) == nil
			} else {
				value, supplied = defaults[key]
			}
			if !supplied && len(child.Properties) == 0 {
				continue
			}
			// A required object inherits its required children, not every
			// optional publishing default in the same protocol container.
			if _, object := value.(map[string]any); object && len(child.Properties) > 0 && len(child.Const) == 0 || !supplied {
				value = map[string]any{}
			}
		}
		if object, ok := value.(map[string]any); ok && len(child.Properties) > 0 {
			copied := maps.Clone(object)
			nestedDefaults, _ := defaults[key].(map[string]any)
			studioFillNativeRequired(child, copied, nestedDefaults)
			value = copied
		}
		body[key] = value
	}
}

func (s *Service) studioNativeMediaBody(ctx context.Context, userID string, req CreateTaskRequest, input canvasGenerationInput, selected platformintegration.ModelSelection) ([]byte, error) {
	values, err := studioNativeSpecifications(input, selected)
	if err != nil {
		return nil, err
	}
	refs, err := s.studioNativeReferenceResources(ctx, userID, input)
	if err != nil {
		return nil, err
	}
	return studioNativeResolvedBody(req.Prompt, selected, values, refs)
}

func studioNativeResolvedBody(prompt string, selected platformintegration.ModelSelection, values map[string]any, refs []studioNativeReferenceResource) ([]byte, error) {
	v := selected.NativeRequest
	body := map[string]any{}
	for _, pointer := range v.EnvelopePointers() {
		if _, exists := platformintegration.NativeValue(v.Defaults, pointer); exists {
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
			value = studioMergeNativeObjects(nil, object)
		}
		if err := platformintegration.SetNativeValue(body, pointer, value); err != nil {
			return nil, BadAuthRequest(err.Error())
		}
	}
	if body["stream"] == true || body["async"] == true {
		return nil, BadAuthRequest("生成模式必须使用已发布的非流式协议")
	}
	if err := studioWriteNativeEnvelope(body, prompt, selected, refs); err != nil {
		return nil, err
	}
	return studioPrepareNativeCombination(v, refs, body)
}

func studioWriteNativeEnvelope(body map[string]any, prompt any, selected platformintegration.ModelSelection, refs []studioNativeReferenceResource) error {
	v := selected.NativeRequest
	switch v.Protocol {
	case "gemini.image":
		body["contents"] = []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt}}}}
	case "gemini.chat.image":
		body["model"] = selected.Name
		body["messages"] = []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": prompt}}}}
	case "modelark.video":
		body["model"] = selected.Name
		body["content"] = []any{map[string]any{"type": "text", "text": prompt}}
	default:
		body["model"], body["prompt"] = selected.Name, prompt
	}
	return studioWriteNativeReferences(v, refs, body)
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

type studioNativeReferenceResource struct {
	Role     string
	Resource *model.Resource
	URI      string
}

func (s *Service) studioNativeReferenceResources(ctx context.Context, userID string, input canvasGenerationInput) ([]studioNativeReferenceResource, error) {
	refs := append(append(append([]providerMedia{}, input.ReferenceImages...), input.ReferenceVideos...), input.ReferenceAudios...)
	if input.Mask != nil {
		mask := *input.Mask
		mask.Role = "mask"
		refs = append(refs, mask)
	}
	resolved := make([]studioNativeReferenceResource, 0, len(refs))
	for _, ref := range refs {
		if !strings.HasPrefix(ref.StorageKey, "resource:") || ref.DataURL != "" {
			return nil, BadAuthRequest("参考素材必须先保存到平台素材库")
		}
		id := strings.TrimPrefix(ref.StorageKey, "resource:")
		resource, err := s.StudioResource(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		pin, err := s.repo.StudioMaterialReference(userID, id)
		if err != nil {
			return nil, NotFound("素材引用不存在或不可访问")
		}
		role := ref.Role
		if role == "" || role == "reference" {
			role = "reference_" + resource.Kind
		}
		if role != "reference_"+resource.Kind && !(resource.Kind == "image" && slices.Contains([]string{"mask", "first_frame", "last_frame"}, role)) {
			return nil, BadAuthRequest("素材角色与文件类型不一致")
		}
		uris, err := platformclient.MediaReferenceURIs([]platformclient.MediaReference{{AssetID: pin.AssetID, VersionID: pin.VersionID}})
		if err != nil {
			return nil, BadAuthRequest("参考素材版本无效")
		}
		resolved = append(resolved, studioNativeReferenceResource{Role: role, Resource: resource, URI: uris[0]})
	}
	return resolved, nil
}

func (s *Service) studioNativeMediaReferences(ctx context.Context, userID string, v *platformintegration.NativeRequestVariant, input canvasGenerationInput, body map[string]any) error {
	refs, err := s.studioNativeReferenceResources(ctx, userID, input)
	if err != nil {
		return err
	}
	return studioWriteNativeReferences(v, refs, body)
}

func studioWriteNativeReferences(v *platformintegration.NativeRequestVariant, refs []studioNativeReferenceResource, body map[string]any) error {
	if err := studioValidateNativeReferences(v.References, refs); err != nil {
		return err
	}
	for _, ref := range refs {
		for _, slot := range v.References {
			if slot.Role == ref.Role {
				if err := studioAppendNativeReference(body, v, slot, ref.URI, ref.Resource.MimeType); err != nil {
					return err
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
