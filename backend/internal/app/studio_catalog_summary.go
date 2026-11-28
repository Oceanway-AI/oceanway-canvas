package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func studioCatalogError(err error) error {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return err
	}
	var upstream *platformclient.Error
	if errors.As(err, &upstream) && upstream.StatusCode == 409 {
		mapped := WrapAppError(409, "模型目录已更新，请刷新并重新确认选择", upstream)
		mapped.Reason = "catalog_revision_conflict"
		return mapped
	}
	if upstream != nil && (upstream.StatusCode == 400 || upstream.StatusCode == 404) {
		return WrapAppError(upstream.StatusCode, "模型选择或版本无效，请刷新目录", upstream)
	}
	if upstream != nil && (upstream.StatusCode == 401 || upstream.StatusCode == 403 || upstream.StatusCode == 429) {
		return platformIdentityError(err)
	}
	var body struct {
		Code string `json:"code"`
	}
	if upstream != nil && upstream.StatusCode == 503 && json.Unmarshal(upstream.RawBody(), &body) == nil && body.Code == "catalog_projection_pending" {
		mapped := WrapAppError(503, "平台正在更新模型目录，请稍后重新读取", err)
		mapped.Reason = "catalog_projection_pending"
		return mapped
	}
	mapped := WrapAppError(503, "模型目录服务暂不可用，请稍后重新读取", err)
	mapped.Reason = "catalog_unavailable"
	return mapped
}

func studioReadCatalogSummaries(query platformclient.ModelSummaryQuery, read func(platformclient.ModelSummaryQuery) (platformclient.Result[platformclient.ModelSummaryPage], error), match func(platformclient.ModelSummary) bool) ([]platformclient.ModelSummary, string, error) {
	items := []platformclient.ModelSummary{}
	revision := ""
	seen := map[string]bool{}
	for {
		page, err := read(query)
		if err != nil {
			return nil, "", err
		}
		if page.Data.Version != platformclient.ModelCatalogVersion || page.Data.State != "summary" || page.Data.CatalogRevision == "" || revision != "" && revision != page.Data.CatalogRevision {
			return nil, "", NewAppError(409, "模型目录已更新，请刷新后继续")
		}
		revision = page.Data.CatalogRevision
		for _, item := range page.Data.Items {
			if item.ModelType != query.ModelType {
				continue
			}
			if match == nil {
				items = append(items, item)
			} else if match(item) {
				return []platformclient.ModelSummary{item}, revision, nil
			}
		}
		if !page.Data.HasMore {
			return items, revision, nil
		}
		if page.Data.NextCursor == "" || seen[page.Data.NextCursor] {
			return nil, "", NewAppError(502, "平台目录分页响应无效")
		}
		seen[page.Data.NextCursor] = true
		query.Cursor = page.Data.NextCursor
	}
}

func (s *Service) StudioModelCatalogPage(ctx context.Context, session platformclient.SessionContext, query platformclient.ModelSummaryQuery) (*ModelCatalogResponse, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.ModelSummaries(ctx, session, query)
	if err != nil {
		return nil, result.Response, studioCatalogError(err)
	}
	if result.Data.Version != platformclient.ModelCatalogVersion || result.Data.State != "summary" || result.Data.CatalogRevision == "" {
		return nil, result.Response, NewAppError(502, "平台模型摘要响应无效")
	}
	items := make([]platformclient.Model, len(result.Data.Items))
	for i, item := range result.Data.Items {
		items[i] = item.Model
	}
	wire, _, err := studioCatalogWire(result.Response, items)
	if err != nil {
		return nil, result.Response, err
	}
	channel := PublicChannelCatalog{ID: StudioPlatformChannelID, Name: "OceanWay", DisplayName: "OceanWay", Models: []PublicChannelModel{}}
	for _, summary := range result.Data.Items {
		item := studioCatalogPublicModel(summary.Model, wire[summary.ModelName])
		if summary.ModelType != "text" {
			item.Available = summary.InvocationStatus == "available"
			item.UnavailableReason = ""
			if summary.ModelType == "video" {
				item.Protocol = model.ChannelInterfaceType("newapi")
			} else {
				item.Protocol = model.ChannelInterfaceType("openai-image")
			}
		}
		channel.Models = append(channel.Models, item)
	}
	return &ModelCatalogResponse{Source: "platform", Models: []PublicLogicalModel{}, Channels: []PublicChannelCatalog{channel}, CatalogRevision: result.Data.CatalogRevision, NextCursor: result.Data.NextCursor, HasMore: result.Data.HasMore}, result.Response, nil
}

func (s *Service) StudioModelCatalogDetail(ctx context.Context, session platformclient.SessionContext, query platformclient.ModelDetailQuery, owner *model.User) (*PublicChannelModel, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.ModelDetail(ctx, session, query)
	if err != nil {
		return nil, result.Response, studioCatalogError(err)
	}
	if err := studioValidateCatalogDetail(result.Data, query); err != nil {
		return nil, result.Response, err
	}
	var wire struct {
		Data struct {
			Model json.RawMessage `json:"model"`
		} `json:"data"`
	}
	if result.Response == nil || json.Unmarshal(result.Response.Body, &wire) != nil || len(wire.Data.Model) == 0 {
		return nil, result.Response, NewAppError(502, "平台模型详情响应无效")
	}
	var public map[string]json.RawMessage
	if json.Unmarshal(wire.Data.Model, &public) != nil || public == nil {
		return nil, result.Response, NewAppError(502, "平台模型详情响应无效")
	}
	public["state"] = json.RawMessage(`"detail"`)
	public["price_version"], _ = json.Marshal(result.Data.PriceVersion)
	public["capability_revision"], _ = json.Marshal(result.Data.CapabilityRevision)
	public["connection_revision"], _ = json.Marshal(result.Data.ConnectionRevision)
	wire.Data.Model, err = json.Marshal(public)
	if err != nil {
		return nil, result.Response, err
	}
	if owner != nil && owner.PlatformAuthenticated && query.VariantID == "" {
		scope := studioCatalogScopeFor(owner.ID, session, owner.PlatformSessionID)
		scope.workspace = query.WorkspaceID
		s.studioMediaCatalog.rememberDetail(scope, result.Data.Model)
	}
	item := studioCatalogModelVariant(result.Data.Model, wire.Data.Model, query.VariantID)
	return &item, result.Response, nil
}

func studioSummaryDetailQuery(summary platformclient.ModelSummary, variant string) platformclient.ModelDetailQuery {
	return platformclient.ModelDetailQuery{ModelName: summary.ModelName, VariantID: variant, ExpectedPriceVersion: summary.PriceVersion, ExpectedCapabilityRevision: summary.CapabilityRevision, ExpectedConnectionRevision: summary.ConnectionRevision}
}
