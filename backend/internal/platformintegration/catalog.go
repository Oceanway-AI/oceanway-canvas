package platformintegration

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

// PublishedPrice keeps decimal strings and the provider's version intact.
// Studio never evaluates pricing expressions or converts these into a wallet.
type PublishedPrice struct {
	Status   string            `json:"status"`
	Version  string            `json:"version"`
	RateUnit string            `json:"rate_unit"`
	Rates    map[string]string `json:"rates"`
}

type NativeEndpoint struct {
	NativeRequest   *NativeRequestVariant `json:"native_request_variant,omitempty"`
	GatewayProtocol string                `json:"gateway_protocol,omitempty"`
	ID              string                `json:"id"`
	Protocol        string                `json:"protocol"`
	Operation       string                `json:"operation"`
	Method          string                `json:"method"`
	Path            string                `json:"path"`
	Defaults        map[string]any        `json:"defaults"`
	Schema          json.RawMessage       `json:"specifications_schema"`
	Bindings        map[string]string     `json:"parameter_bindings"`
	References      []NativeReference     `json:"native_reference_inputs"`
}

type NativeReference struct {
	Protocol      string   `json:"protocol"`
	Operation     string   `json:"operation"`
	Field         string   `json:"field"`
	Transport     string   `json:"transport"`
	Roles         []string `json:"roles"`
	MaxItems      int      `json:"max_items"`
	MinItems      int      `json:"min_items"`
	MaxItemBytes  int64    `json:"max_item_bytes"`
	MaxTotalBytes int64    `json:"max_total_bytes"`
	ImageFormat   string   `json:"image_format"`
	MIMETypes     []string `json:"mime_types"`
}

type MediaProfile struct {
	NativeEndpoint
	Mode            string           `json:"mode"`
	DefaultVariant  string           `json:"default_variant"`
	Variants        []NativeEndpoint `json:"variants"`
	Endpoints       []NativeEndpoint `json:"native_endpoints"`
	OutputContracts []string         `json:"output_contracts"`
	InputModes      []string         `json:"input_modes"`
}

type ModelSelection struct {
	NativeEndpoint
	Name         string
	Kind         string
	Price        PublishedPrice
	TextProtocol platformclient.TextProtocol
	MediaRoute   platformclient.MediaRoute
	PreferAsync  bool
}

// SelectModel is shared by display and admission. Alternatives are never
// unioned: the selected variant owns its own parameters and native endpoint.
func SelectModel(m platformclient.Model, variant string) (ModelSelection, error) {
	selected := ModelSelection{Name: m.ModelName, Kind: m.ModelType}
	if m.ModelName == "" || strings.TrimSpace(m.ModelName) != m.ModelName || json.Unmarshal(m.Price, &selected.Price) != nil || selected.Price.Version == "" || (selected.Price.Status != "available" && selected.Price.Status != "usage_dependent") {
		return selected, errors.New("模型价格暂不可用")
	}
	if m.ModelType == "text" {
		if m.InvocationStatus != "published_text" {
			return selected, errors.New("文本模型暂不可用")
		}
		for _, p := range m.SupportedProtocols {
			switch p {
			case "openai-chat":
				selected.TextProtocol = platformclient.ChatCompletions
			case "openai-responses":
				selected.TextProtocol = platformclient.Responses
			case "anthropic-messages":
				selected.TextProtocol = platformclient.Messages
			default:
				continue
			}
			selected.Protocol = p
			selected.Path = "/" + string(selected.TextProtocol)
			return selected, nil
		}
		return selected, errors.New("模型协议暂未接通")
	}
	if (m.ModelType != "image" && m.ModelType != "video") || m.InvocationStatus != "available" {
		return selected, errors.New("模型暂不可用")
	}
	var profile MediaProfile
	if json.Unmarshal(m.MediaGenerationProfile, &profile) != nil {
		return selected, errors.New("模型能力暂不可用")
	}
	if len(profile.Variants) > 0 {
		if variant == "" {
			variant = profile.DefaultVariant
		}
		found := false
		for _, v := range profile.Variants {
			if v.ID == variant {
				selected.NativeEndpoint = v
				found = true
				break
			}
		}
		if !found {
			return selected, errors.New("请选择有效的模型能力版本")
		}
	} else {
		// Older exact-binding contracts may have one explicit endpoint; more
		// than one requires a protocol choice, never an inferred first entry.
		if variant != "" || len(profile.Endpoints) != 1 {
			return selected, errors.New("模型能力需要明确的协议选择")
		}
		selected.NativeEndpoint = profile.Endpoints[0]
		selected.Defaults = profile.Defaults
		selected.Schema = profile.Schema
		selected.Bindings = profile.Bindings
		for _, ref := range profile.References {
			if ref.Protocol == selected.Protocol && ref.Operation == selected.Operation {
				selected.References = append(selected.References, ref)
			}
		}
	}
	if selected.Method != "POST" || len(selected.Schema) == 0 {
		return selected, errors.New("模型能力合同不完整")
	}
	if selected.NativeRequest != nil {
		err := selected.selectNativeRoute()
		return selected, err
	}
	if selected.GatewayProtocol != "" {
		return selected, errors.New("模型原生请求合同尚未接通")
	}
	switch {
	case m.ModelType == "image" && selected.Protocol == "openai-images" && selected.Operation == "generate" && selected.Path == "/images/generations":
		selected.MediaRoute = platformclient.ImageGeneration
	case m.ModelType == "image" && selected.Protocol == "openai-images" && selected.Operation == "edit" && selected.Path == "/images/edits":
		selected.MediaRoute = platformclient.ImageEdit
	case m.ModelType == "image" && selected.Protocol == "gemini" && selected.Operation == "generate" && selected.Path == "/v1beta/models/{model}:generateContent":
		selected.MediaRoute = platformclient.GeminiMedia
	case m.ModelType == "video" && selected.Protocol == "openai-video" && selected.Operation == "generate" && selected.Path == "/videos":
		selected.MediaRoute = platformclient.VideoGeneration
	case m.ModelType == "video" && (selected.Protocol == "doubao-video" || selected.Protocol == "openai-video") && selected.Operation == "generate" && selected.Path == "/doubao/api/v3/contents/generations/tasks":
		selected.MediaRoute = platformclient.DoubaoVideoGeneration
	default:
		return selected, errors.New("模型协议暂未接通")
	}
	return selected, nil
}
