package app

import (
	"encoding/json"
	"strings"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

type AdminListQuery struct {
	Keyword string
	Status  string
	Type    string
	Group   string
	Page    int
	Limit   int
}

type ChannelRequest struct {
	MediaTransport       *ReferenceMediaTransport `json:"mediaTransport"`
	Name                 string                   `json:"name"`
	SortOrder            *int                     `json:"sortOrder"`
	BaseURL              string                   `json:"baseUrl"`
	APIKey               string                   `json:"apiKey"`
	SecretKey            string                   `json:"secretKey"`
	ConcurrencyLimit     *int                     `json:"concurrencyLimit"`
	UseGlobalConcurrency *bool                    `json:"useGlobalConcurrency"`
	Models               []string                 `json:"models"`
	Headers              []OutboundHeader         `json:"headers"`
	Enabled              *bool                    `json:"enabled"`
	// ProxyURL 指针语义：nil = 请求未提交该字段（更新时保持渠道现有值），
	// 指向空串 = 显式清空渠道代理。此前用 string 类型导致「删掉代理保存」
	// 被 mergeChannelRequest 回填旧值，永远清不掉。
	ProxyURL *string `json:"proxyUrl"`
}

type AdminChannelPage struct {
	Channels []PublicModelChannel `json:"channels"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	Limit    int                  `json:"pageSize"`
}

type PublicModelChannel struct {
	MediaTransport   *ReferenceMediaTransport `json:"mediaTransport,omitempty"`
	ID               string                   `json:"id"`
	UserID           string                   `json:"userId"`
	Scope            model.ChannelScope       `json:"scope"`
	Enabled          bool                     `json:"enabled"`
	Name             string                   `json:"name"`
	SortOrder        int                      `json:"sortOrder"`
	BaseURL          string                   `json:"baseUrl"`
	APIKey           string                   `json:"apiKey"`
	APIFormat        string                   `json:"apiFormat"`
	ConcurrencyLimit int                      `json:"concurrencyLimit"`
	ProxyURL         string                   `json:"proxyUrl"`
	Models           []string                 `json:"models"`
	Headers          []OutboundHeader         `json:"headers,omitempty"`
	HasAPIKey        bool                     `json:"hasApiKey"`
	HasSecretKey     bool                     `json:"hasSecretKey"`
	CreatedAt        time.Time                `json:"createdAt"`
	UpdatedAt        time.Time                `json:"updatedAt"`
}

func channelFromRequest(req ChannelRequest, channel model.ModelChannel) (model.ModelChannel, error) {
	return (&Service{}).channelFromRequest(req, channel)
}

func (s *Service) channelFromRequest(req ChannelRequest, channel model.ModelChannel) (model.ModelChannel, error) {
	if _, err := normalizeReferenceMediaTransport(req.MediaTransport); err != nil {
		return channel, err
	}
	name := strings.TrimSpace(req.Name)
	baseURL := strings.TrimSpace(req.BaseURL)
	if name == "" {
		return channel, BadAuthRequest("请填写渠道名称")
	}
	if baseURL == "" {
		return channel, BadAuthRequest("请填写 Base URL")
	}
	// 启用/停用或只修改价格、模型等配置时，不应要求上游域名当前可解析。
	// 只有 Base URL 实际变化时才做出站地址校验。
	connectionChanged := strings.TrimRight(baseURL, "/") != strings.TrimRight(channel.BaseURL, "/")
	if connectionChanged {
		if _, err := ValidateOutboundURL(baseURL); err != nil {
			return channel, err
		}
	}
	models := uniqueNonEmpty(req.Models)
	modelsJSON, _ := json.Marshal(models)
	headersJSON, err := EncodeOutboundHeadersJSON(req.Headers)
	if err != nil {
		return channel, err
	}
	channel.Name = name
	if req.SortOrder != nil {
		if err := validateChannelSortOrder(*req.SortOrder); err != nil {
			return channel, err
		}
		channel.SortOrder = *req.SortOrder
	}
	channel.BaseURL = strings.TrimRight(baseURL, "/")
	if req.APIKey != "" {
		channel.APIKey = req.APIKey
	}
	if req.SecretKey != "" {
		channel.SecretKey = req.SecretKey
	}
	// 系统渠道只保存地址与凭证；实际协议和鉴权方式由所选模型决定。
	channel.APIFormat = "openai"
	if req.UseGlobalConcurrency != nil && *req.UseGlobalConcurrency {
		channel.ConcurrencyLimit = 0
	} else if req.ConcurrencyLimit != nil {
		if *req.ConcurrencyLimit < minChannelConcurrencyLimit || *req.ConcurrencyLimit > maxChannelConcurrencyLimit {
			return channel, BadAuthRequest("最大并发数必须是 1-999 的整数")
		}
		channel.ConcurrencyLimit = *req.ConcurrencyLimit
	} else if req.UseGlobalConcurrency != nil {
		return channel, BadAuthRequest("请填写渠道最大并发数")
	}
	channel.ModelsJSON = string(modelsJSON)
	channel.HeadersJSON = headersJSON
	proxyURL := ""
	if req.ProxyURL != nil {
		proxyURL = strings.TrimSpace(*req.ProxyURL)
	}
	if _, err := ValidateProxyURL(proxyURL); err != nil {
		return channel, err
	}
	channel.ProxyURL = proxyURL
	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}
	return channel, nil
}

func mergeChannelRequest(req ChannelRequest, channel model.ModelChannel) ChannelRequest {
	if strings.TrimSpace(req.Name) == "" {
		req.Name = channel.Name
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		req.BaseURL = channel.BaseURL
	}
	if req.Models == nil {
		req.Models = channelModelNames(channel)
	}
	if req.Headers == nil {
		req.Headers, _ = ParseOutboundHeadersJSON(channel.HeadersJSON)
	}
	if req.ProxyURL == nil {
		req.ProxyURL = &channel.ProxyURL
	}
	return req
}

func publicChannel(channel model.ModelChannel, admin bool, channelModels []model.ChannelModel) PublicModelChannel {
	models := make([]string, 0, len(channelModels))
	for _, item := range channelModels {
		if !item.Enabled {
			continue
		}
		models = append(models, item.ModelKey)

	}
	if len(models) == 0 {
		_ = json.Unmarshal([]byte(channel.ModelsJSON), &models)
	}
	apiKey := ""
	baseURL := channel.BaseURL
	proxyURL := ""
	var headers []OutboundHeader
	if channel.Scope == model.ChannelScopeSystem {
		if !admin {
			apiKey = "system"
			baseURL = "/api/ai/system/" + channel.ID
		}
		if admin {
			headers, _ = ParseOutboundHeadersJSON(channel.HeadersJSON)
		}
	} else if admin {
		apiKey = channel.APIKey
	}
	if admin {
		proxyURL = channel.ProxyURL
	}
	return PublicModelChannel{
		ID:               channel.ID,
		UserID:           channel.UserID,
		Scope:            channel.Scope,
		Enabled:          channel.Enabled,
		Name:             channel.Name,
		SortOrder:        channel.SortOrder,
		BaseURL:          baseURL,
		APIKey:           apiKey,
		APIFormat:        channel.APIFormat,
		ConcurrencyLimit: channel.ConcurrencyLimit,
		ProxyURL:         proxyURL,
		Models:           models,
		Headers:          headers,
		HasAPIKey:        strings.TrimSpace(channel.APIKey) != "",
		HasSecretKey:     strings.TrimSpace(channel.SecretKey) != "",
		CreatedAt:        channel.CreatedAt,
		UpdatedAt:        channel.UpdatedAt,
	}
}

func uniqueNonEmpty(values []string) []string {
	return kernel.UniqueNonEmpty(values)
}
