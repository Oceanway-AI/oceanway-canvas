package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

const StudioPlatformChannelID = "oceanway-platform"

func (s *Service) StudioModelCatalog(ctx context.Context, session platformclient.SessionContext) (*ModelCatalogResponse, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.Models(ctx, session)
	if err != nil {
		return nil, result.Response, platformIdentityError(err)
	}
	wireModels, groups, err := studioCatalogWire(result.Response, result.Data.Items)
	if err != nil {
		return nil, result.Response, err
	}
	channel := PublicChannelCatalog{ID: StudioPlatformChannelID, Name: "OceanWay", DisplayName: "OceanWay", Models: []PublicChannelModel{}, PlatformGroups: groups}
	for _, item := range result.Data.Items {
		selected, selectErr := platformintegration.SelectModel(item, "")
		var content struct {
			DisplayName string   `json:"display_name"`
			Tagline     string   `json:"tagline"`
			Tags        []string `json:"tags"`
		}
		_ = json.Unmarshal(item.Content, &content)
		tags := make([]model.ChannelModelTag, 0, len(content.Tags))
		for _, tag := range content.Tags {
			tags = append(tags, model.ChannelModelTag{Text: tag, Color: "blue"})
		}
		m := PublicChannelModel{ID: strconv.Itoa(item.ID), ModelKey: item.ModelName, DisplayName: firstNonEmpty(content.DisplayName, item.ModelName), ChannelLabel: item.DisplayVendor, Description: firstNonEmpty(content.Tagline, item.Description), Icon: item.DisplayIcon, Capability: item.ModelType, Tags: tags, PriceTiers: []PublicChannelModelPriceTier{}, PricingMode: "platform", PriceLabel: "以平台报价为准", Available: selectErr == nil, Platform: wireModels[item.ModelName]}
		if selectErr != nil {
			m.UnavailableReason = selectErr.Error()
		} else {
			m.Protocol = model.ChannelInterfaceType(studioOACProtocol(selected))
			m.TextWire = studioTextWire(selected.TextProtocol)
			m.DefaultOptions = selected.Defaults
			m.CapabilityConfig = studioCapabilityProjection(selected)
		}
		channel.Models = append(channel.Models, m)
	}
	return &ModelCatalogResponse{Source: "platform", Models: []PublicLogicalModel{}, Channels: []PublicChannelCatalog{channel}}, result.Response, nil
}

// Forward the authorized public wire objects intact. Re-encoding a narrow SDK
// model discards presentation fields and native capability extensions.
func studioCatalogWire(response *platformclient.Response, items []platformclient.Model) (map[string]json.RawMessage, json.RawMessage, error) {
	invalid := func() (map[string]json.RawMessage, json.RawMessage, error) {
		return nil, nil, fmt.Errorf("invalid Platform model catalog response")
	}
	if response == nil {
		return invalid()
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Items  []json.RawMessage `json:"items"`
			Groups json.RawMessage   `json:"groups"`
		} `json:"data"`
	}
	if json.Unmarshal(response.Body, &envelope) != nil || !envelope.Success || len(envelope.Data.Items) != len(items) {
		return invalid()
	}
	models := make(map[string]json.RawMessage, len(items))
	for _, raw := range envelope.Data.Items {
		var identity struct {
			ModelName string `json:"model_name"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ModelName == "" || models[identity.ModelName] != nil {
			return invalid()
		}
		models[identity.ModelName] = raw
	}
	for _, item := range items {
		if models[item.ModelName] == nil {
			return invalid()
		}
	}
	if len(envelope.Data.Groups) != 0 {
		var groups []struct {
			ModelNames []string `json:"model_names"`
		}
		if json.Unmarshal(envelope.Data.Groups, &groups) != nil {
			return invalid()
		}
		for _, group := range groups {
			for _, name := range group.ModelNames {
				if models[name] == nil {
					return invalid()
				}
			}
		}
	}
	return models, envelope.Data.Groups, nil
}

func studioTextWire(p platformclient.TextProtocol) string {
	switch p {
	case platformclient.ChatCompletions:
		return "chat-completion"
	case platformclient.Responses:
		return "responses"
	case platformclient.Messages:
		return "claude-api"
	}
	return ""
}

func studioOACProtocol(s platformintegration.ModelSelection) string {
	switch s.TextProtocol {
	case platformclient.ChatCompletions:
		return "chat-completion"
	case platformclient.Responses:
		return "openai-response"
	case platformclient.Messages:
		return "claude-api"
	}
	switch s.MediaRoute {
	case platformclient.GeminiMedia:
		return "gemini-image"
	case platformclient.VideoGeneration, platformclient.NewAPIVideoGeneration:
		return "newapi"
	case platformclient.DoubaoVideoGeneration, platformclient.ModelArkVideoGeneration:
		return "volcengine-ark-video"
	default:
		return "openai-image"
	}
}

func studioPublishedValues(field map[string]any) []any {
	if value, ok := field["const"]; ok {
		return []any{value}
	}
	if values, ok := field["enum"].([]any); ok {
		return values
	}
	return []any{}
}

func studioPublishedDefault(s platformintegration.ModelSelection, key string, field map[string]any) any {
	if value, ok := field["const"]; ok {
		return value
	}
	if value, ok := s.Defaults[key]; ok {
		return value
	}
	return field["default"]
}

// The raw published schema remains in Platform. This projection serves OAC's
// existing controls. It grants no submission authority or schema validation.
func studioCapabilityProjection(s platformintegration.ModelSelection) map[string]any {
	if s.Kind == "text" {
		return map[string]any{"version": 1, "text": map[string]any{"streaming": false, "contextWindowTokens": 0, "maxOutputTokens": 0, "references": map[string]any{"promptMaxChars": 0, "maxImages": 0, "maxImageBytes": 0, "maxVideos": 0, "maxVideoBytes": 0}}}
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	_ = json.Unmarshal(s.Schema, &schema)
	values := func(name string) []any {
		return studioPublishedValues(schema.Properties[name])
	}
	defaultValue := func(name string) any {
		return studioPublishedDefault(s, name, schema.Properties[name])
	}
	stringDefault := func(name string) string { v, _ := defaultValue(name).(string); return v }
	boolDefault := func(name string) bool { v, _ := defaultValue(name).(bool); return v }
	numberDefault := func(name string) float64 { v, _ := defaultValue(name).(float64); return v }
	refs := map[string]any{"promptMaxChars": 0, "maxImages": 0, "maxImageBytes": 0, "maskSupported": false, "minImages": 0, "maxVideos": 0, "maxVideoBytes": 0, "maxVideoDurationSeconds": 0, "maxAudios": 0, "maxAudioBytes": 0, "maxAudioDurationSeconds": 0}
	for _, r := range s.References {
		for _, role := range r.Roles {
			switch role {
			case "reference_image", "first_frame", "last_frame":
				refs["maxImages"] = max(refs["maxImages"].(int), r.MaxItems)
				refs["maxImageBytes"] = r.MaxItemBytes
			case "mask":
				refs["maskSupported"] = true
			case "reference_video":
				refs["maxVideos"] = r.MaxItems
				refs["maxVideoBytes"] = r.MaxItemBytes
			case "reference_audio":
				refs["maxAudios"] = r.MaxItems
				refs["maxAudioBytes"] = r.MaxItemBytes
			}
		}
	}
	if s.Kind == "image" {
		parameters := []map[string]any{}
		keys := make([]string, 0, len(schema.Properties))
		for key := range schema.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field := schema.Properties[key]
			typ, _ := field["type"].(string)
			if typ != "string" && typ != "boolean" && typ != "number" && typ != "integer" {
				continue
			}
			name := studioParameterName(key)
			param := map[string]any{"name": name, "label": key, "type": typ, "binding": s.Bindings[key], "enabled": true}
			for _, name := range schema.Required {
				if name == key {
					param["required"] = true
				}
			}
			if choices := values(key); len(choices) > 0 {
				param["values"] = choices
			}
			for from, to := range map[string]string{"minimum": "min", "maximum": "max", "multipleOf": "step", "maxLength": "maxLength", "description": "description"} {
				if v, ok := field[from]; ok {
					param[to] = v
				}
			}
			if v, ok := field["multipleOf"]; ok {
				param["multipleOf"] = v
			}
			if v := defaultValue(key); v != nil {
				param["default"] = v
			}
			parameters = append(parameters, param)
		}
		sizeKey := "size"
		if schema.Properties[sizeKey] == nil && schema.Properties["aspect_ratio"] != nil {
			sizeKey = "aspect_ratio"
		}
		sizeParameter := "none"
		if schema.Properties[sizeKey] != nil {
			sizeParameter = sizeKey
		}
		maxOutputs := 1
		if n, ok := schema.Properties["n"]["maximum"].(float64); ok && n > 0 {
			maxOutputs = int(n)
		}
		return map[string]any{"version": 1, "image": map[string]any{"parameters": parameters, "references": refs, "size": map[string]any{"parameter": sizeParameter, "values": values(sizeKey), "default": stringDefault(sizeKey), "allowCustom": schema.Properties[sizeKey] != nil && len(values(sizeKey)) == 0}, "resolution": map[string]any{"supported": schema.Properties["resolution"] != nil, "values": values("resolution"), "default": stringDefault("resolution")}, "quality": map[string]any{"supported": schema.Properties["quality"] != nil, "values": values("quality"), "default": stringDefault("quality")}, "transparentBackground": map[string]any{"supported": false, "default": false}, "responseFormat": map[string]any{"supported": false}, "outputFormat": map[string]any{"supported": false}, "maxOutputs": maxOutputs}}
	}
	durationKey := "duration"
	if schema.Properties[durationKey] == nil && schema.Properties["seconds"] != nil {
		durationKey = "seconds"
	}
	duration := map[string]any{"selection": "range", "default": numberDefault(durationKey)}
	for from, to := range map[string]string{"minimum": "min", "maximum": "max", "multipleOf": "step"} {
		if v, ok := schema.Properties[durationKey][from]; ok {
			duration[to] = v
		}
	}
	if v, ok := schema.Properties[durationKey]["multipleOf"]; ok {
		duration["multipleOf"] = v
	}
	if len(values(durationKey)) > 0 {
		duration["selection"] = "enum"
		duration["values"] = values(durationKey)
	}
	operations := []string{"text_to_video"}
	for _, ref := range s.References {
		for _, role := range ref.Roles {
			if role == "first_frame" {
				operations = append(operations, "image_to_video")
			}
			if role == "reference_image" || role == "reference_video" {
				operations = append(operations, "reference_to_video")
			}
		}
	}
	ratioKey := "aspect_ratio"
	if schema.Properties[ratioKey] == nil && schema.Properties["ratio"] != nil {
		ratioKey = "ratio"
	}
	audioKey := "generate_audio"
	if schema.Properties[audioKey] == nil && schema.Properties["audio"] != nil {
		audioKey = "audio"
	}
	return map[string]any{"version": 1, "video": map[string]any{"references": refs, "duration": duration, "durationSupported": schema.Properties[durationKey] != nil, "ratios": values(ratioKey), "defaultRatio": stringDefault(ratioKey), "resolutions": values("resolution"), "defaultResolution": stringDefault("resolution"), "generateAudio": map[string]any{"supported": schema.Properties[audioKey] != nil, "default": boolDefault(audioKey)}, "watermark": map[string]any{"supported": schema.Properties["watermark"] != nil, "default": boolDefault("watermark")}, "operations": operations, "defaultOperation": operations[0]}}
}
