package app

import (
	"encoding/json"
	"strings"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/protocol"
)

type ChannelModelRequest struct {
	ModelKey         string                  `json:"modelKey"`
	ProviderModelKey string                  `json:"providerModelKey"`
	DisplayName      string                  `json:"displayName"`
	ChannelLabel     string                  `json:"channelLabel"`
	Tags             []model.ChannelModelTag `json:"tags"`
	Description      string                  `json:"description"`
	Icon             string                  `json:"icon"`
	Capability       string                  `json:"capability"`
	Protocol         string                  `json:"protocol"`

	Enabled          *bool                        `json:"enabled"`
	CapabilityConfig *ModelCapabilityConfig       `json:"capabilityConfig"`
	Variants         []ChannelModelVariantRequest `json:"variants"`
}

// ChannelModelVariantRequest 是系统渠道内某个规格的上游 SKU。
// Resolution="*"、VideoSeconds=0 分别表示任意分辨率和任意时长。
type ChannelModelVariantRequest struct {

	// Selector 是 SKU 的规范匹配条件。支持 operation、quality、size、vquality、videoSeconds、imageCount、videoGenerateAudio；
	// operation 可区分文生/图生/视频生，避免同一分辨率下错误复用上游 SKU。
	Selector         map[string]string `json:"selector"`
	Resolution       string            `json:"resolution"`
	VideoSeconds     int               `json:"videoSeconds"`
	ProviderModelKey string            `json:"providerModelKey"`

	Enabled *bool `json:"enabled"`
}

func (s *Service) EnsureSystemChannelModels() error {
	channels, err := s.repo.SystemChannels(true)
	if err != nil {
		return err
	}
	for index := range channels {
		items, err := s.repo.ChannelModels(channels[index].ID, true)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			if err := s.syncInitialChannelModels(&channels[index], channelModelNames(channels[index])); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) SystemChannelModel(channelID string, modelKey string) (*model.ChannelModel, error) {
	return s.repo.ChannelModelByKey(channelID, strings.TrimPrefix(strings.TrimSpace(modelKey), "models/"))
}

// SystemChannelHasProtocol 用于没有携带 model 字段的轮询请求：先确认渠道确实配置了该协议，
// 再由 handler 按协议限定请求路径，避免用空模型绕过系统渠道授权。
func (s *Service) SystemChannelHasProtocol(channelID string, protocol model.ChannelInterfaceType) (bool, error) {
	items, err := s.repo.ChannelModels(channelID, false)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Protocol == protocol {
			return true, nil
		}
	}
	return false, nil
}

func normalizeChannelModelContract(channel *model.ModelChannel, req ChannelModelRequest) (string, string, string, model.ChannelInterfaceType, error) {
	return normalizeChannelModelContractWithRegistry(protocol.Builtins(), channel, req)
}

func (s *Service) normalizeChannelModelContract(channel *model.ModelChannel, req ChannelModelRequest) (string, string, string, model.ChannelInterfaceType, error) {
	return normalizeChannelModelContractWithRegistry(s.protocolRegistry(), channel, req)
}

func normalizeChannelModelContractWithRegistry(registry *protocol.Registry, channel *model.ModelChannel, req ChannelModelRequest) (string, string, string, model.ChannelInterfaceType, error) {
	modelKey := strings.TrimPrefix(strings.TrimSpace(req.ModelKey), "models/")
	if modelKey == "" {
		return "", "", "", "", BadAuthRequest("请填写模型标识")
	}
	providerModelKey := strings.TrimPrefix(strings.TrimSpace(req.ProviderModelKey), "models/")
	if providerModelKey == "" {
		providerModelKey = modelKey
	}
	capability := normalizeCapability(req.Capability)
	if capability == "" {
		return "", "", "", "", BadAuthRequest("请选择模型能力")
	}
	adapter, ok := registry.Resolve(strings.TrimSpace(req.Protocol))
	if !ok || !adapter.Metadata().Enabled || adapter.Metadata().UnavailableReason != "" {
		return "", "", "", "", BadAuthRequest("请选择有效的模型请求协议")
	}
	protocol := model.ChannelInterfaceType(adapter.Metadata().ID)
	if expected := protocolCapabilityFromMetadata(adapter.Metadata()); expected != "" && expected != capability {
		return "", "", "", "", BadAuthRequest("模型能力与请求协议不匹配")
	}
	if protocolAuthUsesSecretKey(adapter.Metadata().Auth) && (strings.TrimSpace(channel.APIKey) == "" || strings.TrimSpace(channel.SecretKey) == "") {
		return "", "", "", "", BadAuthRequest(adapter.Metadata().Name + " 协议需要先在渠道中配置 API Key 和 Secret Key")
	}
	return modelKey, providerModelKey, capability, protocol, nil
}

func (s *Service) syncInitialChannelModels(channel *model.ModelChannel, names []string) error {
	existing, err := s.repo.ChannelModels(channel.ID, true)
	if err != nil {
		return err
	}
	byKey := make(map[string]*model.ChannelModel, len(existing))
	for index := range existing {
		byKey[existing[index].ModelKey] = &existing[index]
	}
	desired := make(map[string]bool, len(names))
	retired := retiredChannelModelKeys(channel.RetiredModelsJSON)
	for _, name := range uniqueNonEmpty(names) {
		name = strings.TrimPrefix(name, "models/")
		if retired[channelModelCatalogKey(name)] {
			continue
		}
		desired[name] = true
		if item := byKey[name]; item != nil {
			continue
		}
		modelID, idErr := s.repo.NextPrefixedID("MODEL")
		if idErr != nil {
			return idErr
		}
		item := model.ChannelModel{ID: modelID, ChannelID: channel.ID, ModelKey: name, ProviderModelKey: name, DisplayName: "未分类", Enabled: false}
		if err := s.repo.SaveChannelModel(&item); err != nil {
			return err
		}
	}
	for index := range existing {
		if !desired[existing[index].ModelKey] {
			changed := existing[index].Enabled
			existing[index].Enabled = false
			if changed {
				if err := s.repo.SaveChannelModel(&existing[index]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func retiredChannelModelKeys(raw string) map[string]bool {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if key := channelModelCatalogKey(value); key != "" {
			result[key] = true
		}
	}
	return result
}

func channelModelCatalogKey(value string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "models/"))
}

func (s *Service) ensureChannelModels(channelID string, includeDisabled bool) ([]model.ChannelModel, error) {
	items, err := s.repo.ChannelModels(channelID, includeDisabled)
	if err != nil || len(items) > 0 {
		return items, err
	}
	channel, err := s.repo.AdminSystemChannel(channelID)
	if err != nil {
		return nil, err
	}
	if err := s.syncInitialChannelModels(channel, channelModelNames(*channel)); err != nil {
		return nil, err
	}
	return s.repo.ChannelModels(channelID, includeDisabled)
}

func (s *Service) syncChannelModelNames(channel *model.ModelChannel) error {
	return s.repo.SyncChannelModelNames(channel.ID, time.Now())
}

func (s *Service) capabilityForProtocol(protocol model.ChannelInterfaceType) string {
	metadata, ok := s.channelProtocolMetadata(string(protocol))
	if !ok {
		return ""
	}
	return protocolCapabilityFromMetadata(metadata)
}

func protocolCapabilityFromMetadata(metadata protocol.Metadata) string {
	if len(metadata.Categories) == 0 {
		return ""
	}
	return string(metadata.Categories[0])
}
