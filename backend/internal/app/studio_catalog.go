package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

const StudioPlatformChannelID = "oceanway-platform"

func (s *Service) StudioModelCatalog(ctx context.Context, session platformclient.SessionContext, owner ...*model.User) (*ModelCatalogResponse, *platformclient.Response, error) {
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
	if len(owner) == 1 && owner[0] != nil && owner[0].PlatformAuthenticated {
		s.studioMediaCatalog.remember(studioCatalogScopeFor(owner[0].ID, session, owner[0].PlatformSessionID), result.Data.Items)
	}
	channel := PublicChannelCatalog{ID: StudioPlatformChannelID, Name: "OceanWay", DisplayName: "OceanWay", Models: []PublicChannelModel{}, PlatformGroups: groups}
	for _, item := range result.Data.Items {
		channel.Models = append(channel.Models, studioCatalogPublicModel(item, wireModels[item.ModelName]))
	}
	return &ModelCatalogResponse{Source: "platform", Models: []PublicLogicalModel{}, Channels: []PublicChannelCatalog{channel}}, result.Response, nil
}

func studioCatalogPublicModel(item platformclient.Model, wire json.RawMessage) PublicChannelModel {
	return studioCatalogModelVariant(item, wire, "")
}

func studioCatalogModelVariant(item platformclient.Model, wire json.RawMessage, variantID string) PublicChannelModel {
	selected, selectErr := platformintegration.SelectModel(item, variantID)
	var candidates []platformintegration.ModelSelection
	if variantID == "" && (item.ModelType == "image" || item.ModelType == "video") {
		var profile platformintegration.MediaProfile
		if json.Unmarshal(item.MediaGenerationProfile, &profile) == nil {
			for _, variant := range profile.Variants {
				candidate, err := platformintegration.SelectModelFromProfile(item, variant.ID, profile)
				if err == nil {
					candidates = append(candidates, candidate)
				}
			}
			if selectErr != nil && len(candidates) > 0 {
				selected, selectErr = candidates[0], nil
			}
		}
	}
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
	m := PublicChannelModel{ID: strconv.Itoa(item.ID), ModelKey: item.ModelName, DisplayName: firstNonEmpty(content.DisplayName, item.ModelName), ChannelLabel: item.DisplayVendor, Description: firstNonEmpty(content.Tagline, item.Description), Icon: item.DisplayIcon, Capability: item.ModelType, Tags: tags, PricingMode: "platform", PriceLabel: "以平台报价为准", Available: selectErr == nil, Platform: wire}
	if selectErr != nil {
		m.UnavailableReason = selectErr.Error()
	} else {
		m.Protocol = model.ChannelInterfaceType(studioOACProtocol(selected))
		m.TextWire = studioTextWire(selected.TextProtocol)
		m.DefaultOptions = selected.Defaults
		m.CapabilityConfig = studioCapabilityProjection(selected)
		if selected.NativeRequest != nil {
			if len(candidates) == 0 {
				candidates = []platformintegration.ModelSelection{selected}
			}
			media := m.CapabilityConfig[selected.Kind].(map[string]any)
			studioProjectReferenceChoices(media, candidates)
			if variantID == "" {
				media["referenceScope"] = "model"
			} else {
				media["referenceScope"] = "variant"
			}
		}
	}
	return m
}

// Each displayed maximum is the largest on any one complete supply branch.
func studioProjectReferenceChoices(media map[string]any, candidates []platformintegration.ModelSelection) {
	refs := media["references"].(map[string]any)
	fields := map[string]string{"reference_image": "maxImages", "first_frame": "maxFirstFrames", "last_frame": "maxLastFrames", "reference_video": "maxVideos", "reference_audio": "maxAudios"}
	maxima, unknown := map[string]int64{}, map[string]bool{}
	operations := []string{}
	mask, complete := false, false
	for _, candidate := range candidates {
		if candidate.NativeRequest == nil {
			continue
		}
		v := candidate.NativeRequest
		options, err := v.Alternatives()
		if err != nil {
			continue
		}
		if len(options) == 0 {
			options = []platformintegration.NativeSupplyAlternative{{References: v.References, Materials: v.Materials}}
		}
		for _, option := range options {
			complete = true
			branch := []string{}
			empty := true
			var required int64
			for _, slot := range option.References {
				if slot.MinItems != nil {
					required += *slot.MinItems
				}
			}
			for _, slot := range option.References {
				if slot.MinItems != nil && *slot.MinItems > 0 {
					empty = false
				}
				limit := slot.MaxItems
				if c := option.Materials; c != nil && c.MaxItems != nil {
					remaining := *c.MaxItems - required
					if slot.MinItems != nil {
						remaining += *slot.MinItems
					}
					remaining = max(0, remaining)
					if limit == nil || remaining < *limit {
						limit = &remaining
					}
				}
				if limit != nil && *limit == 0 {
					continue
				}
				if slot.Role == "mask" {
					mask = true
				}
				if field, ok := fields[slot.Role]; ok {
					if limit == nil {
						unknown[field] = true
					} else {
						maxima[field] = max(maxima[field], *limit)
					}
				}
				switch slot.Role {
				case "first_frame", "last_frame":
					branch = append(branch, "image_to_video")
				case "reference_image", "reference_video", "reference_audio":
					branch = append(branch, "reference_to_video")
				}
			}
			if empty {
				branch = append([]string{"text_to_video"}, branch...)
			}
			for _, op := range branch {
				if !slices.Contains(operations, op) {
					operations = append(operations, op)
				}
			}
		}
	}
	for _, field := range fields {
		if !complete || unknown[field] {
			delete(refs, field)
		} else {
			refs[field] = maxima[field]
		}
	}
	if complete {
		refs["maskSupported"] = mask
	} else {
		delete(refs, "maskSupported")
	}
	if _, video := media["operations"]; video {
		media["operations"] = operations
		delete(media, "defaultOperation")
		if len(operations) == 1 {
			media["defaultOperation"] = operations[0]
		}
	}
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
		if capability := s.TextCapability; capability != nil {
			return map[string]any{"version": 1, "text": map[string]any{"streaming": false, "contextWindowTokens": capability.ContextWindowTokens, "maxOutputTokens": capability.MaxOutputTokens, "references": map[string]any{"promptMaxChars": 0, "maxImages": capability.MaxImages, "maxImageBytes": capability.MaxImageBytes, "maxVideos": 0, "maxVideoBytes": 0}}}
		}
		return map[string]any{"version": 1, "text": map[string]any{"streaming": false, "contextWindowTokens": 0, "maxOutputTokens": 0, "references": map[string]any{"promptMaxChars": 0, "maxImages": 0, "maxImageBytes": 0, "maxVideos": 0, "maxVideoBytes": 0}}}
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	_ = json.Unmarshal(s.Schema, &schema)
	fieldKey := func(names ...string) string {
		for _, name := range names {
			if _, exists := schema.Properties[name]; exists {
				return name
			}
		}
		return ""
	}
	resolutionKey := fieldKey("/resolution", "/metadata/resolution", "/generationConfig/imageConfig/imageSize", "/image_size", "resolution", "image_size")
	ratioKey := fieldKey("/aspect_ratio", "/metadata/aspect_ratio", "/ratio", "/generationConfig/imageConfig/aspectRatio", "aspect_ratio", "ratio")
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
			case "reference_image":
				refs["maxImages"] = max(refs["maxImages"].(int), r.MaxItems)
				refs["maxImageBytes"] = r.MaxItemBytes
			case "first_frame":
				refs["maxFirstFrames"] = r.MaxItems
			case "last_frame":
				refs["maxLastFrames"] = r.MaxItems
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
		sizeKey, sizeParameter := fieldKey("/size", "size"), "size"
		if sizeKey == "" {
			sizeKey, sizeParameter = ratioKey, "aspect_ratio"
		}
		if sizeKey == "" {
			sizeParameter = "none"
		}
		qualityKey := fieldKey("/quality", "quality")
		background := fieldKey("/background", "background")
		transparent := fieldKey("/transparent_background", "transparent_background")
		transparentAllowed := slices.Contains(values(background), any("transparent"))
		if transparent != "" {
			options := values(transparent)
			transparentAllowed = len(options) == 0 && schema.Properties[transparent]["type"] == "boolean" || slices.Contains(options, any(true))
		}
		size := map[string]any{"parameter": sizeParameter, "values": values(sizeKey), "default": stringDefault(sizeKey), "allowCustom": sizeKey != "" && len(values(sizeKey)) == 0}
		resolution := map[string]any{"supported": resolutionKey != "", "values": values(resolutionKey), "default": stringDefault(resolutionKey)}
		if v := s.NativeRequest; v != nil {
			if v.OutputSize != nil {
				size["allowCustom"] = v.OutputSize.AllowCustom
			}
			var presets []ImageSizePreset
			var tiers, publishedSizes []any
			for _, preset := range v.SizePresets {
				wired, _ := platformintegration.NativeValue(preset.Values, sizeKey)
				value, _ := wired.(string)
				if value == "" || preset.Resolution == "" || preset.AspectRatio == "" {
					continue
				}
				presets = append(presets, ImageSizePreset{Tier: preset.Resolution, Ratio: preset.AspectRatio, Size: value, Width: int(preset.Width), Height: int(preset.Height)})
				if !slices.Contains(tiers, any(preset.Resolution)) {
					tiers = append(tiers, preset.Resolution)
				}
				if !slices.Contains(publishedSizes, any(value)) {
					publishedSizes = append(publishedSizes, value)
				}
			}
			if len(presets) > 0 {
				size["presets"] = presets
				if len(values(sizeKey)) == 0 {
					size["values"] = publishedSizes
				}
				if resolutionKey == "" {
					resolution["supported"], resolution["values"] = true, tiers
				}
			}
		}
		maxOutputs := 1
		if n, ok := schema.Properties[fieldKey("/n", "n")]["maximum"].(float64); ok && n > 0 {
			maxOutputs = int(n)
		}
		return map[string]any{"version": 1, "image": map[string]any{
			"parameters": parameters, "references": refs, "size": size, "resolution": resolution, "maxOutputs": maxOutputs,
			"quality":               map[string]any{"supported": qualityKey != "", "values": values(qualityKey), "default": stringDefault(qualityKey)},
			"transparentBackground": map[string]any{"supported": transparentAllowed, "default": stringDefault(background) == "transparent" || boolDefault(transparent)},
			"responseFormat":        map[string]any{"supported": fieldKey("/response_format", "response_format") != ""},
			"outputFormat":          map[string]any{"supported": fieldKey("/output_format", "output_format") != ""},
		}}
	}
	durationKey := fieldKey("/duration", "/seconds", "duration", "seconds")
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
	operations := []string{}
	allowsEmpty := true
	for _, ref := range s.References {
		if ref.MinItems > 0 {
			allowsEmpty = false
		}
		for _, role := range ref.Roles {
			if (role == "first_frame" || role == "last_frame") && !slices.Contains(operations, "image_to_video") {
				operations = append(operations, "image_to_video")
			}
			if (role == "reference_image" || role == "reference_video" || role == "reference_audio") && !slices.Contains(operations, "reference_to_video") {
				operations = append(operations, "reference_to_video")
			}
		}
	}
	if allowsEmpty {
		operations = append([]string{"text_to_video"}, operations...)
	}
	audioKey := fieldKey("/generate_audio", "/metadata/generate_audio", "/audio", "generate_audio", "audio")
	watermarkKey := fieldKey("/watermark", "/metadata/watermark", "watermark")
	video := map[string]any{
		"references": refs, "duration": duration, "durationSupported": durationKey != "", "operations": operations,
		"ratios": values(ratioKey), "defaultRatio": stringDefault(ratioKey), "resolutions": values(resolutionKey), "defaultResolution": stringDefault(resolutionKey),
		"generateAudio": map[string]any{"supported": audioKey != "", "default": boolDefault(audioKey)},
		"watermark":     map[string]any{"supported": watermarkKey != "", "default": boolDefault(watermarkKey)},
	}
	if len(operations) == 1 {
		video["defaultOperation"] = operations[0]
	}
	if s.NativeRequest != nil {
		studioProjectReferenceChoices(video, []platformintegration.ModelSelection{s})
	}
	return map[string]any{"version": 1, "video": video}
}
