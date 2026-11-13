package app

import (
	"context"
	"encoding/json"
	"strings"
)

// Native Asset metadata is local business data; every file is already adopted
// through Platform. Never persist an external/signed URL as the file identity.
func (s *Service) studioAssetPayload(userID string, raw json.RawMessage) (json.RawMessage, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return nil, BadAuthRequest("素材数据格式无效")
	}
	kind, _ := payload["kind"].(string)
	if kind != "text" && kind != "image" && kind != "video" && kind != "audio" {
		return nil, BadAuthRequest("该创作资产类型尚未接通")
	}
	if kind != "text" {
		data, _ := payload["data"].(map[string]any)
		key, _ := data["storageKey"].(string)
		if !strings.HasPrefix(key, "resource:") || len(key) <= len("resource:") {
			return nil, BadAuthRequest("请先上传或采用平台文件")
		}
		id := strings.TrimPrefix(key, "resource:")
		resource, err := s.StudioResource(context.Background(), userID, id)
		if err != nil {
			return nil, err
		}
		ref, err := s.repo.StudioMaterialReference(userID, id)
		if err != nil || ref.BindingID == "" || ref.AssetID == "" || ref.VersionID == "" || ref.FileID == "" || resource.Kind != kind {
			return nil, BadAuthRequest("素材类型或引用不匹配")
		}
		url := "/api/resources/" + id + "/file"
		if kind == "image" {
			data["dataUrl"] = url
		} else {
			data["url"] = url
		}
		data["bytes"], data["mimeType"] = resource.Size, resource.MimeType
		// Covers for managed media follow the same fixed file. Separate cover
		// adoption is not enabled until its own reference lifecycle is connected.
		payload["coverUrl"] = url
	} else {
		payload["coverUrl"] = ""
	}
	return json.Marshal(payload)
}
