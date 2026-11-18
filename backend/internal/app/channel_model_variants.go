// 渠道模型规格档：选择器规范化、上游 SKU 与能力校验。

package app

import (
	"fmt"
	"strconv"
	"strings"

	protocolcontract "yingce/backend/internal/protocol"
)

func normalizeChannelModelVariantSelector(capability string, input ChannelModelVariantRequest, parameters ...[]protocolcontract.ImageParameter) (map[string]string, string, int, error) {
	if capability == "image" && len(parameters) > 0 && parameters[0] != nil {
		if input.VideoSeconds != 0 || input.Resolution != "" && input.Resolution != "*" {
			return nil, "", 0, BadAuthRequest("图片规格不接受视频规格")
		}
		selector := make(map[string]string, len(input.Selector))
		for name, value := range input.Selector {
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if name == "" || value == "" || value == "*" {
				continue
			}
			if _, exists := selector[name]; exists {
				return nil, "", 0, BadAuthRequest("规格档存在重复规格条件：" + name)
			}
			selector[name] = value
		}
		if err := validateImageParameterPriceSelector(parameters[0], selector); err != nil {
			return nil, "", 0, err
		}
		return selector, "*", 0, nil
	}
	selector := make(map[string]string, len(input.Selector)+3)
	for rawKey, rawValue := range input.Selector {
		key := strings.TrimSpace(rawKey)
		value := strings.TrimSpace(rawValue)
		if key == "" || value == "" {
			continue
		}
		switch key {
		case "operation":
			value = strings.ToLower(value)
		case "quality", "imageResolution", "size":
			value = strings.ToLower(value)
			if value == "auto" || value == "any" {
				value = "*"
			}
		case "resoluton":
			// Normalize the historical misspelling at the input boundary.
			key = "imageResolution"
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "auto" || value == "any" {
				value = "*"
			}
		case "vquality":
			value = normalizeChannelModelVariantResolution(value)
		case "resolution":
			// 兼容图片规格档的标准字段，以及历史接口误拼写 resoluton。
			key = "resolution"
			value = strings.ToLower(strings.TrimSpace(value))
		case "videoSeconds":
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds < 0 {
				return nil, "", 0, BadAuthRequest("视频规格档时长必须是非负整数")
			}
			if seconds == 0 {
				continue
			}
			value = strconv.Itoa(seconds)
		case "imageCount":
			count, err := strconv.Atoi(value)
			if err != nil || count < 0 {
				return nil, "", 0, BadAuthRequest("参考图片数量必须是非负整数")
			}
			if count == 0 {
				continue
			}
			value = strconv.Itoa(count)
		case "videoGenerateAudio":
			if capability != "video" {
				return nil, "", 0, BadAuthRequest("只有视频模型可以按是否生成音频配置规格档")
			}
			value = strings.ToLower(value)
			if value == "*" {
				continue
			}
			if value != "true" && value != "false" {
				return nil, "", 0, BadAuthRequest("视频规格档音频条件必须为 true、false 或 *")
			}
		case "fpsBand":
			if capability != "video" {
				return nil, "", 0, BadAuthRequest("只有视频模型可以按帧率档配置规格档")
			}
			value = strings.ToLower(value)
			if value == "*" {
				continue
			}
			if value != "le30" && value != "le60" && value != "le120" {
				return nil, "", 0, BadAuthRequest("视频规格档帧率条件必须为 le30、le60、le120 或 *")
			}
		default:
			if capability != "image" {
				return nil, "", 0, BadAuthRequest("规格档不支持规格字段：" + key)
			}
		}
		if existing, exists := selector[key]; exists && existing != value {
			return nil, "", 0, BadAuthRequest("规格档存在重复规格条件：" + key)
		}
		selector[key] = value
	}
	if capability == "video" {
		if _, exists := selector["vquality"]; !exists {
			if resolution := normalizeChannelModelVariantResolution(input.Resolution); resolution != "*" {
				selector["vquality"] = resolution
			}
		}
		if _, exists := selector["videoSeconds"]; !exists && input.VideoSeconds > 0 {
			selector["videoSeconds"] = strconv.Itoa(input.VideoSeconds)
		}
	} else if input.Resolution != "" && normalizeChannelModelVariantResolution(input.Resolution) != "*" {
		return nil, "", 0, BadAuthRequest("非视频模型不能使用视频分辨率规格档")
	} else if input.VideoSeconds != 0 {
		return nil, "", 0, BadAuthRequest("非视频模型不能使用视频时长规格档")
	}
	for _, key := range []string{"quality", "size"} {
		if _, exists := selector[key]; exists && capability != "image" {
			return nil, "", 0, BadAuthRequest("只有图片模型可以按 " + key + " 配置规格档")
		}
	}
	if _, exists := selector["vquality"]; exists && capability != "video" {
		return nil, "", 0, BadAuthRequest("只有视频模型可以按分辨率配置规格档")
	}
	if _, exists := selector["resolution"]; exists && capability != "image" {
		return nil, "", 0, BadAuthRequest("只有图片模型可以按分辨率配置规格档")
	}
	if _, exists := selector["videoSeconds"]; exists && capability != "video" {
		return nil, "", 0, BadAuthRequest("只有视频模型可以按时长配置规格档")
	}
	if _, exists := selector["imageCount"]; exists && capability != "video" {
		return nil, "", 0, BadAuthRequest("只有视频模型可以按参考图片数量配置规格档")
	}
	resolution := "*"
	if value := selector["vquality"]; value != "" {
		resolution = value
	}
	videoSeconds := 0
	if value := selector["videoSeconds"]; value != "" {
		videoSeconds, _ = strconv.Atoi(value)
	}
	return selector, resolution, videoSeconds, nil
}

func normalizeChannelModelVariantResolution(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value == "*" || strings.EqualFold(value, "any") {
		return "*"
	}
	normalized := normalizeModelRequestOption("vquality", value)
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(normalized)))
}
