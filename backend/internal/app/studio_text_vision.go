package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

// Resolve fixed, owned Resources through the original material authority. Only
// verified pixels enter the encrypted execution body and its stable fingerprint.
func (s *Service) studioTextBodyWithMaterials(ctx context.Context, userID string, materials *platformclient.Materials, req CreateTaskRequest, selected platformintegration.ModelSelection) ([]byte, error) {
	raw, err := json.Marshal(req.Input)
	if err != nil {
		return nil, BadAuthRequest("文本输入无效")
	}
	var input canvasGenerationInput
	if json.Unmarshal(raw, &input) != nil {
		return nil, BadAuthRequest("文本输入无效")
	}
	if len(input.ReferenceImages) == 0 {
		return studioTextBody(req, selected)
	}
	capability := selected.TextCapability
	if capability == nil || capability.MaxImages <= 0 || capability.MaxImageBytes <= 0 || len(input.ReferenceImages) > capability.MaxImages {
		return nil, BadAuthRequest("参考图片超过平台模型声明的能力")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	images := make(map[string]string, len(input.ReferenceImages))
	totalBytes := 0
	for _, reference := range input.ReferenceImages {
		id, ok := strings.CutPrefix(reference.StorageKey, "resource:")
		if !ok || id == "" || reference.URL != "" || reference.DataURL != "" {
			return nil, BadAuthRequest("参考图片必须使用已保存的账号资源")
		}
		if images[reference.StorageKey] != "" {
			continue
		}
		resource, err := s.StudioResource(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(resource.MimeType, "image/") || resource.Size <= 0 || resource.Size > textModelImageCompressionMaxInput {
			return nil, BadAuthRequest("图片资源类型或大小无效")
		}
		pin, err := s.repo.WithContext(ctx).StudioMaterialReference(userID, id)
		if err != nil || pin.BindingID == "" {
			return nil, NotFound("图片资源没有可用的固定素材引用")
		}
		// Download the bound original for verified inline pixels. The signed
		// download ticket stays on this server and is never sent to the model.
		access, err := materials.Access(ctx, platformclient.AccessRequest{Items: []platformclient.AccessItem{{ItemID: id, AssetID: pin.AssetID, VersionID: pin.VersionID, BindingID: pin.BindingID, Purpose: "download", Variant: "original"}}})
		if err != nil {
			return nil, studioMaterialError(err)
		}
		if len(access.Data.Results) != 1 {
			return nil, NewAppError(502, "图片访问响应不完整")
		}
		item := access.Data.Results[0]
		expires, expiryErr := time.Parse(time.RFC3339, item.ExpiresAt)
		u, urlErr := url.Parse(item.URL)
		if item.Status != "ready" || item.ItemID != id || item.AssetID != pin.AssetID || item.VersionID != pin.VersionID || item.FileID != pin.FileID || item.ActualVariant != "original" || item.Revision == "" || expiryErr != nil || !expires.After(time.Now()) || urlErr != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
			return nil, NewAppError(502, "图片访问凭据无效或已失效")
		}
		if _, err := ValidateOutboundURL(item.URL); err != nil {
			return nil, NewAppError(502, "图片访问地址不可用")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
		if err != nil {
			return nil, NewAppError(502, "图片访问地址无效")
		}
		response, err := OutboundHTTPClient(30 * time.Second).Do(request)
		if err != nil {
			return nil, NewAppError(502, "读取图片失败，请稍后重试")
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, textModelImageCompressionMaxInput+1))
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || int64(len(data)) != resource.Size || len(data) == 0 || int64(len(data)) > textModelImageCompressionMaxInput || pin.SHA256 != "" && fmt.Sprintf("%x", sha256.Sum256(data)) != pin.SHA256 {
			return nil, NewAppError(502, "图片文件与固定素材版本不匹配")
		}
		mime := http.DetectContentType(data)
		switch mime {
		case "image/png", "image/jpeg", "image/webp", "image/gif":
		default:
			return nil, BadAuthRequest("图片编码暂不支持文本模型输入")
		}
		compressed := compressImageForTextModel(data, mime, capability.MaxImageBytes)
		if compressed.Compressed {
			data, mime = compressed.Data, compressed.MIME
		}
		if int64(len(data)) > capability.MaxImageBytes {
			return nil, BadAuthRequest("图片超过平台模型单图大小限制")
		}
		pixels := dataURL(mime, data)
		totalBytes += len(pixels)
		if totalBytes > 32<<20 {
			return nil, BadAuthRequest("图片内容超过本次模型请求大小限制")
		}
		images[reference.StorageKey] = pixels
	}
	body, err := studioTextBodyWithImages(req, selected, images)
	if len(body) > 32<<20 {
		return nil, BadAuthRequest("模型请求超过 32 MiB")
	}
	return body, err
}
