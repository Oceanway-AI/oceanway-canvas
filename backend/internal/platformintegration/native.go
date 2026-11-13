package platformintegration

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

// Consumer view of the published contract. The original catalog remains raw;
// Studio never writes this view back as a Gateway capability definition.
type NativeRequestVariant struct {
	ID                 string                           `json:"id"`
	Protocol           string                           `json:"protocol"`
	Operation          string                           `json:"operation"`
	Method             string                           `json:"method"`
	Path               string                           `json:"path"`
	ContentTypes       []string                         `json:"content_types"`
	Schema             json.RawMessage                  `json:"request_schema"`
	SizePresets        []platformclient.ImageSizePreset `json:"size_presets,omitempty"`
	OutputSize         *platformclient.ImageOutputSize  `json:"output_size,omitempty"`
	SupplyAlternatives json.RawMessage                  `json:"supply_alternatives,omitempty"`
	Defaults           map[string]any                   `json:"defaults"`
	References         []NativeReferenceSlot            `json:"native_reference_inputs"`
	Modes              []string                         `json:"modes"`
	MaxOutputs         int                              `json:"max_outputs"`
	Unresolved         []string                         `json:"unresolved_conditions"`
}

type NativeReferenceSlot struct {
	ID               string   `json:"id"`
	Pointer          string   `json:"pointer"`
	Transport        string   `json:"transport"`
	Role             string   `json:"role"`
	MIMETypes        []string `json:"mime_types"`
	MinItems         *int64   `json:"min_items"`
	MaxItems         *int64   `json:"max_items"`
	MaxItemBytes     *int64   `json:"max_item_bytes"`
	MaxTotalBytes    *int64   `json:"max_total_bytes"`
	SameDimensionsAs string   `json:"same_dimensions_as"`
}

var errNativeContract = errors.New("模型原生请求合同无效或尚未接通")

func (v NativeRequestVariant) MultipartReferenceImages(slot NativeReferenceSlot) bool {
	return v.Protocol == "openai.image.edit" && slices.Contains(v.ContentTypes, "multipart/form-data") &&
		slot.Pointer == "/image" && slot.Role == "reference_image" && slot.Transport == "image-reference-array"
}

func (s *ModelSelection) selectNativeRoute() error {
	v := s.NativeRequest
	if v.ID == "" || v.Protocol != s.GatewayProtocol || v.Operation != "create" || v.Method != "POST" || len(v.Unresolved) != 0 || v.Defaults == nil || v.MaxOutputs < 1 {
		return errNativeContract
	}
	if !slices.Contains(v.ContentTypes, "application/json") && !slices.ContainsFunc(v.References, v.MultipartReferenceImages) {
		return errNativeContract
	}
	var schema map[string]any
	if json.Unmarshal(v.Schema, &schema) != nil || schema["type"] != "object" {
		return errNativeContract
	}
	path, kind := "", "image"
	switch v.Protocol {
	case "openai.image.generate":
		path, s.MediaRoute = "/v1/images/generations", platformclient.ImageGeneration
	case "openai.image.edit":
		path, s.MediaRoute = "/v1/images/edits", platformclient.ImageEdit
	case "gemini.image":
		path, s.MediaRoute = "/v1beta/models/{model}:generateContent", platformclient.GeminiMedia
	case "gemini.chat.image":
		path, s.MediaRoute = "/v1/chat/completions", platformclient.GeminiChatImage
	case "sora.video":
		path, kind, s.MediaRoute = "/v1/videos", "video", platformclient.VideoGeneration
	case "newapi.video":
		path, kind, s.MediaRoute = "/v1/video/generations", "video", platformclient.NewAPIVideoGeneration
	case "modelark.video":
		path, kind, s.MediaRoute = "/api/v3/contents/generations/tasks", "video", platformclient.ModelArkVideoGeneration
	default:
		return errNativeContract
	}
	if v.Path != path || s.Kind != kind || len(v.Modes) == 0 {
		return errNativeContract
	}
	for _, mode := range v.Modes {
		if mode != "sync" && mode != "async" {
			return errNativeContract
		}
	}
	if kind == "video" && !slices.Contains(v.Modes, "async") {
		return errNativeContract
	}
	s.PreferAsync = kind == "image" && slices.Contains(v.Modes, "async")
	for _, pointer := range v.EnvelopePointers() {
		if _, exists := NativeValue(v.Defaults, pointer); exists {
			return errNativeContract
		}
	}
	for key, pointer := range s.Bindings {
		if key != pointer || !v.ParameterAllowed(pointer) {
			return errNativeContract
		}
	}
	return nil
}

func (v NativeRequestVariant) EnvelopePointers() []string {
	p := []string{"/model", "/prompt", "/content", "/contents", "/messages"}
	for _, r := range v.References {
		p = append(p, r.Pointer)
	}
	return p
}

func (v NativeRequestVariant) ParameterAllowed(pointer string) bool {
	if _, err := NativePointerParts(pointer); err != nil {
		return false
	}
	for _, root := range v.EnvelopePointers() {
		if pointer == root || strings.HasPrefix(pointer, root+"/") || strings.HasPrefix(root, pointer+"/") {
			return false
		}
	}
	return true
}

// Object JSON Pointers only. Material arrays are assembled by the reference
// builder, so parameter bindings cannot address arbitrary prompt array entries.
func NativePointerParts(pointer string) ([]string, error) {
	if !strings.HasPrefix(pointer, "/") || len(pointer) > 512 {
		return nil, errNativeContract
	}
	parts := strings.Split(pointer[1:], "/")
	if len(parts) > 16 {
		return nil, errNativeContract
	}
	for i, p := range parts {
		if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(p, "~0", ""), "~1", ""), "~") {
			return nil, errNativeContract
		}
		p = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
		if p == "" || strings.Trim(p, "0123456789") == "" || strings.ContainsAny(p, "*[]\x00\r\n") || slices.Contains([]string{"-", "__proto__", "constructor", "prototype", "authorization", "api_key", "headers", "base_url", "endpoint"}, strings.ToLower(p)) {
			return nil, errNativeContract
		}
		parts[i] = p
	}
	return parts, nil
}

func NativeValue(body map[string]any, pointer string) (any, bool) {
	parts, err := NativePointerParts(pointer)
	if err != nil {
		return nil, false
	}
	var value any = body
	for _, part := range parts {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func SetNativeValue(body map[string]any, pointer string, value any) error {
	parts, err := NativePointerParts(pointer)
	if err != nil {
		return err
	}
	current := body
	for _, part := range parts[:len(parts)-1] {
		if old, exists := current[part]; exists {
			obj, ok := old.(map[string]any)
			if !ok {
				return errNativeContract
			}
			current = obj
		} else {
			obj := map[string]any{}
			current[part] = obj
			current = obj
		}
	}
	current[parts[len(parts)-1]] = value
	return nil
}
