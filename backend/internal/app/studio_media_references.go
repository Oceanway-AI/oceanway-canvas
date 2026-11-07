package app

import (
	"context"
	"slices"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

func (s *Service) studioMediaReferences(ctx context.Context, userID string, selected platformintegration.ModelSelection, input canvasGenerationInput, body map[string]any) error {
	refs := append(append(append([]providerMedia{}, input.ReferenceImages...), input.ReferenceVideos...), input.ReferenceAudios...)
	if input.Mask != nil {
		mask := *input.Mask
		mask.Role = "mask"
		refs = append(refs, mask)
	}
	counts := map[string]int{}
	totals := map[string]int64{}
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
		if role != "mask" && role != "first_frame" && role != "last_frame" && role != "reference_"+resource.Kind {
			return BadAuthRequest("素材角色与文件类型不一致")
		}
		if (role == "mask" || role == "first_frame" || role == "last_frame") && resource.Kind != "image" {
			return BadAuthRequest("参考帧必须是图片")
		}
		var field *platformintegration.NativeReference
		for _, candidate := range selected.References {
			if !slices.Contains(candidate.Roles, role) || candidate.Transport == "inline-data-array" {
				continue
			}
			if candidate.MaxItems > 0 && counts[candidate.Field] >= candidate.MaxItems {
				continue
			}
			if field == nil || candidate.MaxItems > field.MaxItems {
				copy := candidate
				field = &copy
			}
		}
		if field == nil {
			return BadAuthRequest("所选模型不支持当前参考素材角色或数量")
		}
		counts[field.Field]++
		totals[field.Field] += resource.Size
		if field.MaxItemBytes > 0 && resource.Size > field.MaxItemBytes || field.MaxTotalBytes > 0 && totals[field.Field] > field.MaxTotalBytes || len(field.MIMETypes) > 0 && !slices.Contains(field.MIMETypes, resource.MimeType) {
			return BadAuthRequest("参考素材超出所选模型的文件限制")
		}
		uri := "material://" + pin.AssetID + "/" + pin.VersionID
		switch {
		case selected.MediaRoute == platformclient.GeminiMedia && field.Field == "contents.parts.fileData":
			content := body["contents"].([]any)[0].(map[string]any)
			content["parts"] = append(content["parts"].([]any), map[string]any{"fileData": map[string]any{"fileUri": uri, "mimeType": resource.MimeType}})
		case selected.MediaRoute == platformclient.DoubaoVideoGeneration && strings.HasPrefix(field.Field, "content"):
			kind := resource.Kind + "_url"
			body["content"] = append(body["content"].([]any), map[string]any{"type": kind, kind: map[string]any{"url": uri}, "role": role})
		case !strings.Contains(field.Field, ".") && field.Transport == "url-array":
			values, _ := body[field.Field].([]any)
			body[field.Field] = append(values, uri)
		case !strings.Contains(field.Field, ".") && (field.Transport == "image-reference" || field.Transport == "image-reference-array"):
			key := "image_url"
			if field.ImageFormat == "xai" {
				key = "url"
			}
			value := map[string]any{key: uri}
			if field.Transport == "image-reference-array" {
				values, _ := body[field.Field].([]any)
				body[field.Field] = append(values, value)
			} else {
				if body[field.Field] != nil {
					return BadAuthRequest("参考素材字段冲突")
				}
				body[field.Field] = value
			}
		default:
			return NewAppError(501, "此参考素材协议尚未接通")
		}
	}
	for _, field := range selected.References {
		if field.MinItems > counts[field.Field] {
			return BadAuthRequest("缺少所选模型要求的参考素材")
		}
	}
	return nil
}
