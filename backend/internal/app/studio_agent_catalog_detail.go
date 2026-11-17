package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

func studioValidateCatalogDetail(detail platformclient.ModelDetail, query platformclient.ModelDetailQuery) error {
	if detail.Version != platformclient.ModelCatalogVersion || detail.State != "detail" || detail.CatalogRevision == "" || detail.ModelName != query.ModelName || detail.Model.ModelName != query.ModelName || query.VariantID != "" && detail.VariantID != query.VariantID {
		return NewAppError(502, "平台模型详情响应无效")
	}
	var price struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(detail.Model.Price, &price) != nil || price.Version == "" || price.Version != detail.PriceVersion {
		return NewAppError(502, "平台模型详情的价格版本不一致")
	}
	if query.ExpectedPriceVersion != "" && detail.PriceVersion != query.ExpectedPriceVersion || query.ExpectedCapabilityRevision != "" && detail.CapabilityRevision != query.ExpectedCapabilityRevision || query.ExpectedConnectionRevision != 0 && detail.ConnectionRevision != query.ExpectedConnectionRevision {
		err := NewAppError(409, "模型目录已更新，请刷新并重新确认模型和报价")
		err.Reason = "catalog_revision_conflict"
		return err
	}
	return nil
}

func studioFrozenCatalogDetail(envelope studioExecutionEnvelope, name, variant string) (platformclient.ModelDetail, bool) {
	for _, detail := range envelope.CatalogDetails {
		if detail.ModelName == name && (detail.VariantID == variant || detail.VariantID == "") {
			return detail, true
		}
	}
	return platformclient.ModelDetail{}, false
}

func (s *Service) studioAgentCatalogDetail(userID, runID, name, variant string) (platformclient.ModelDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var loaded *platformclient.ModelDetail
	for attempt := 0; attempt < 8; attempt++ {
		root, err := s.repo.StudioExecution(userID, runID)
		if err != nil {
			return platformclient.ModelDetail{}, err
		}
		if root.State != "agent" || root.Released {
			return platformclient.ModelDetail{}, NewAppError(409, "本轮执行授权已结束")
		}
		var envelope studioExecutionEnvelope
		if err := s.decryptStudioValue(root.EnvelopeEncrypted, &envelope); err != nil {
			return platformclient.ModelDetail{}, err
		}
		if detail, ok := studioFrozenCatalogDetail(envelope, name, variant); ok {
			return detail, nil
		}
		var summary *platformclient.ModelSummary
		for i := range envelope.CatalogSummaries {
			if envelope.CatalogSummaries[i].ModelName == name {
				summary = &envelope.CatalogSummaries[i]
				break
			}
		}
		if summary == nil || envelope.CatalogVersion != platformclient.ModelCatalogVersion {
			return platformclient.ModelDetail{}, BadAuthRequest("模型不在本轮平台目录中，请重新开始会话")
		}
		query := studioSummaryDetailQuery(*summary, variant)
		if loaded == nil {
			authorization, err := s.decryptSettingSecret(root.AuthorizationEncrypted)
			if err != nil {
				return platformclient.ModelDetail{}, err
			}
			result, err := s.studioPlatform.Client.ExecutionModelDetail(ctx, platformclient.ExecutionContext{Authorization: authorization}, query)
			if err != nil {
				return platformclient.ModelDetail{}, studioCatalogError(err)
			}
			loaded = &result.Data
		}
		if err := studioValidateCatalogDetail(*loaded, query); err != nil {
			return platformclient.ModelDetail{}, err
		}
		envelope.CatalogDetails = append(envelope.CatalogDetails, *loaded)
		encrypted, err := s.encryptStudioValue(envelope)
		if err != nil {
			return platformclient.ModelDetail{}, err
		}
		saved, err := s.repo.SaveStudioAgentCatalog(userID, runID, root.EnvelopeEncrypted, encrypted)
		if err != nil {
			return platformclient.ModelDetail{}, err
		}
		if saved {
			return *loaded, nil
		}
	}
	return platformclient.ModelDetail{}, NewAppError(409, "本轮模型详情正在更新，请重试读取")
}
