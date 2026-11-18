package app

import (
	"regexp"

	"yingce/backend/internal/model"
	"yingce/backend/internal/protocol"
)

var logicalModelCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,79}$`)

type PublicLogicalModel struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	Capability  string `json:"capability"`
	SortOrder   int    `json:"sortOrder"`

	PricingMode    string         `json:"pricingMode"`
	PriceLabel     string         `json:"priceLabel"`
	LegacyModelIDs []string       `json:"legacyModelIds"`
	CapabilitySpec CapabilitySpec `json:"capabilitySpec"`
	// CapabilityProfiles 是创作端可见的匿名能力组合，不暴露其背后的供应线路关系。
	CapabilityProfiles []CapabilitySpec `json:"capabilityProfiles"`
	DefaultOptions     map[string]any   `json:"defaultOptions"`
	Available          bool             `json:"available"`
}

func availableCachedRouteSpecs(routes []cachedLogicalRoute) []CapabilitySpec {
	result := make([]CapabilitySpec, 0, len(routes))
	for _, route := range routes {
		if route.Route.Enabled && route.Route.Weight > 0 {
			result = append(result, route.CapabilitySpec)
		}
	}
	return result
}

func hasHealthyCachedRoute(s *Service, routes []cachedLogicalRoute) bool {
	for _, route := range routes {
		if route.Route.Enabled && route.Route.Weight > 0 && !s.logicalRouteBlocked(route) {
			return true
		}
	}
	return false
}

func logicalModelCapabilityCovered(product CapabilitySpec, routeSpecs []CapabilitySpec) bool {
	return len(routeSpecs) > 0 && validateProductSpecWithinRoutes(product, routeSpecs) == nil
}

func logicalModelConfigurationError(product CapabilitySpec, routeSpecs []CapabilitySpec) string {
	if len(routeSpecs) == 0 || logicalModelCapabilityCovered(product, routeSpecs) {
		return ""
	}
	return "供应线路已无法完整覆盖创作端能力，请调整线路或能力范围"
}

func normalizeLogicalDefaults(spec CapabilitySpec, defaults map[string]any) (map[string]any, error) {
	if spec.Capability == "image" && spec.ImageParameters != nil {
		parameters := append([]protocol.ImageParameter(nil), spec.ImageParameters...)
		for index := range parameters {
			parameters[index].Required = false
		}
		if _, err := resolveImageParameterValues(parameters, defaults); err != nil {
			return nil, err
		}
	}
	result := make(map[string]any, len(defaults))
	for rawName, value := range defaults {
		name := canonicalModelOptionName(spec.Capability, rawName)
		if _, exists := result[name]; exists {
			return nil, BadAuthRequest("默认参数存在重复别名：" + name)
		}
		constraint, ok := spec.Options[name]
		if !ok || !matchOptionConstraint(name, constraint, value) {
			return nil, BadAuthRequest("默认参数 " + name + " 不在前台模型能力范围内")
		}
		// `*` 只表示允许任意自定义值，不能作为创作端默认参数发送。
		if normalizedScalar(value) == "*" {
			for _, candidate := range constraint.Values {
				if normalizedScalar(candidate) != "*" {
					value = candidate
					break
				}
			}
		}
		result[name] = value
	}
	return result, nil
}

func channelModelCapabilitySpec(channelModel model.ChannelModel, registries ...*protocol.Registry) (CapabilitySpec, error) {
	config, err := DecodeModelCapabilityConfig(channelModel.CapabilityConfigJSON)
	if err != nil {
		return CapabilitySpec{}, BadAuthRequest("渠道模型能力配置无效，请先修复渠道模型")
	}
	if config != nil {
		config, err = NormalizeModelCapabilityConfigForModel(normalizeCapability(channelModel.Capability), string(channelModel.Protocol), firstNonEmpty(channelModel.ProviderModelKey, channelModel.ModelKey), config, registries...)
		if err != nil {
			return CapabilitySpec{}, err
		}
	}
	spec, err := CapabilitySpecFromModelCapabilityConfig(config, normalizeCapability(channelModel.Capability))
	if err != nil {
		return CapabilitySpec{}, err
	}
	return capabilitySpecWithVariants(spec, channelModel), nil
}

// capabilitySpecWithVariants 只让创作端选择已经启用的规格。规格组合最终仍由
// 路由时的精确规格档匹配保证，避免独立枚举无法表达“分辨率 × 时长”非笛卡尔组合的问题。
func capabilitySpecWithVariants(spec CapabilitySpec, channelModel model.ChannelModel) CapabilitySpec {
	if normalizeCapability(spec.Capability) != "video" || len(channelModel.Variants) == 0 {
		return spec
	}
	tiers := make([]model.ChannelModelVariant, 0, len(channelModel.Variants))
	for _, tier := range channelModel.Variants {
		if tier.Enabled {
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		return spec
	}
	result := spec
	result.Options = make(map[string]OptionConstraint, len(spec.Options))
	for name, option := range spec.Options {
		result.Options[name] = option
	}
	hasResolutionWildcard, hasDurationWildcard := false, false
	resolutions := make([]any, 0, len(tiers))
	durations := make([]any, 0, len(tiers))
	seenResolutions := make(map[string]bool, len(tiers))
	seenDurations := make(map[int]bool, len(tiers))
	for _, tier := range tiers {
		if normalizeChannelModelVariantResolution(tier.Resolution) == "*" {
			hasResolutionWildcard = true
		} else if value := normalizeChannelModelVariantResolution(tier.Resolution); !seenResolutions[value] {
			seenResolutions[value] = true
			resolutions = append(resolutions, value)
		}
		if tier.VideoSeconds == 0 {
			hasDurationWildcard = true
		} else if !seenDurations[tier.VideoSeconds] {
			seenDurations[tier.VideoSeconds] = true
			durations = append(durations, tier.VideoSeconds)
		}
	}
	if !hasResolutionWildcard && len(resolutions) > 0 {
		if channelModel.Protocol == "autodl-comfyui" {
			if declared, ok := spec.Options["vquality"]; ok {
				allowed := make([]any, 0, len(resolutions))
				for _, value := range resolutions {
					if matchOptionConstraint("vquality", declared, value) {
						allowed = append(allowed, value)
					}
				}
				resolutions = allowed
			}
		}
		if len(resolutions) > 0 {
			result.Options["vquality"] = OptionConstraint{Values: resolutions}
		}
	}
	if !hasDurationWildcard && len(durations) > 0 {
		result.Options["videoSeconds"] = OptionConstraint{Values: durations}
	}
	return result
}

func channelModelDefaultOptions(channelModel model.ChannelModel, spec CapabilitySpec, registries ...*protocol.Registry) (map[string]any, error) {
	config, err := DecodeModelCapabilityConfig(channelModel.CapabilityConfigJSON)
	if err != nil {
		return nil, BadAuthRequest("渠道模型能力配置无效，请先修复渠道模型")
	}
	if config != nil {
		config, err = NormalizeModelCapabilityConfigForModel(normalizeCapability(channelModel.Capability), string(channelModel.Protocol), firstNonEmpty(channelModel.ProviderModelKey, channelModel.ModelKey), config, registries...)
		if err != nil {
			return nil, err
		}
	}
	defaults := make(map[string]any)
	if spec.Capability == "image" && spec.ImageParameters != nil {
		return normalizeLogicalDefaults(spec, imageParameterDefaults(spec.ImageParameters))
	}
	if config != nil {
		switch normalizeCapability(channelModel.Capability) {
		case "image":
			if config.Image != nil {
				defaults["size"] = config.Image.Size.Default
				if config.Image.Quality.Supported {
					defaults["quality"] = config.Image.Quality.Default
				}
				if config.Image.TransparentBackground.Supported {
					defaults["transparentBackground"] = config.Image.TransparentBackground.Default
				}
			}
		case "video":
			if config.Video != nil {
				defaults["videoSeconds"] = config.Video.Duration.Default
				defaults["vquality"] = normalizeChannelModelVariantResolution(config.Video.DefaultResolution)
				if len(config.Video.Ratios) > 0 {
					defaults["size"] = config.Video.DefaultRatio
				}
				if config.Video.GenerateAudio.Supported {
					defaults["videoGenerateAudio"] = config.Video.GenerateAudio.Default
				}
				if config.Video.Watermark.Supported {
					defaults["videoWatermark"] = config.Video.Watermark.Default
				}
			}
		}
	}
	// 当渠道默认规格没有规格档时，优先选择第一个启用规格，确保初始选择可直接生成。
	if normalizeCapability(channelModel.Capability) == "video" && channelModelVariantForIntent(channelModel, ModelRequestIntent{Capability: "video", Options: defaults}) == nil {
		for _, tier := range channelModel.Variants {
			if !tier.Enabled {
				continue
			}
			if tier.Resolution != "*" {
				defaults["vquality"] = normalizeChannelModelVariantResolution(tier.Resolution)
			}
			if tier.VideoSeconds > 0 {
				defaults["videoSeconds"] = tier.VideoSeconds
			}
			break
		}
	}
	return normalizeLogicalDefaults(spec, defaults)
}
