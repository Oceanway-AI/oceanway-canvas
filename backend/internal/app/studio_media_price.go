package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

type StudioMediaPriceEstimate struct {
	State        string  `json:"state"`
	Points       *string `json:"points,omitempty"`
	PriceVersion string  `json:"priceVersion"`
	VariantID    string  `json:"variantId"`
}

func (s *Service) StudioEstimateMediaPrice(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest, sessionID string) (*StudioMediaPriceEstimate, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response := &platformclient.Response{Header: make(http.Header)}
	o := req.Platform
	if o == nil || o.VariantID == "" || o.ExpectedPriceVersion == "" {
		return nil, response, BadAuthRequest("请先选择已发布的模型与参数")
	}
	if o.WorkspaceID != "" || o.ExpectedUserID != "" && o.ExpectedUserID != userID {
		return nil, response, Forbidden("当前账号无权估算此素材空间")
	}
	if err := s.ensureTaskProjectActive(userID, req.ProjectID); err != nil {
		return nil, response, err
	}
	selected, err := s.studioSelectTaskModel(ctx, userID, session, req, response, studioCatalogScopeFor(userID, session, sessionID))
	if err != nil {
		return nil, response, err
	}
	if !selected.GenerationV1 || selected.ID != o.VariantID || selected.Price.Version != o.ExpectedPriceVersion {
		return nil, response, NewAppError(http.StatusConflict, "模型或价格已更新，请刷新目录")
	}
	body, err := s.studioMediaBody(ctx, userID, req, selected)
	if err != nil {
		return nil, response, err
	}
	result, err := s.studioPlatform.Client.EstimateMediaPrice(ctx, session, platformclient.MediaPriceEstimateRequest{
		Model: selected.Name, VariantID: selected.ID, NativeRequest: body, ExpectedPriceVersion: selected.Price.Version,
		ExpectedCapabilityRevision: o.ExpectedCapabilityRevision, ExpectedConnectionRevision: o.ExpectedConnectionRevision,
	})
	result.Response.AppendCookies(response.Header)
	if err != nil {
		var upstream *platformclient.Error
		if errors.As(err, &upstream) {
			switch upstream.StatusCode {
			case http.StatusConflict:
				return nil, response, WrapAppError(http.StatusConflict, "模型或价格已更新，请刷新目录", err)
			case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
				return nil, response, platformIdentityError(err)
			case http.StatusBadRequest, http.StatusUnprocessableEntity:
				return nil, response, WrapAppError(upstream.StatusCode, "当前参数无法估价", err)
			}
		}
		return nil, response, WrapAppError(http.StatusServiceUnavailable, "暂时无法估价", err)
	}
	quote := result.Data
	if quote.Model != selected.Name || quote.ModelType != selected.Kind || quote.VariantID != selected.ID || quote.PriceVersion != selected.Price.Version {
		return nil, response, NewAppError(http.StatusBadGateway, "平台估价与当前参数不一致")
	}
	return &StudioMediaPriceEstimate{State: quote.State, Points: quote.Points, PriceVersion: quote.PriceVersion, VariantID: quote.VariantID}, response, nil
}
