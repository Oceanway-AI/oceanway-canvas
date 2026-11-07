package app

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/assets"
)

func (s *Service) StudioResourceAccess(ctx context.Context, userID string, session platformclient.SessionContext, requests []ResourceAccessRequest) ([]ResourceAccessResult, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c := s.materialCall(ctx, userID, session)
	if len(requests) < 1 || len(requests) > 100 {
		return nil, c.response, BadAuthRequest("每批资源访问请求须为 1–100 项")
	}
	results := make([]ResourceAccessResult, len(requests))
	items := []platformclient.AccessItem{}
	files := map[string]string{}
	for i, req := range requests {
		results[i].ResourceID = req.ResourceID
		results[i].Error = &ResourceAccessFailure{Code: 404, Reason: "resource_unavailable", Message: "资源不存在或不可访问"}
		res, err := s.StudioResource(ctx, userID, req.ResourceID)
		if err != nil {
			continue
		}
		ref, err := s.repo.WithContext(ctx).StudioMaterialReference(userID, req.ResourceID)
		if err != nil {
			continue
		}
		purpose, variant := string(req.Purpose), string(req.Variant)
		if purpose == "" {
			purpose = "display"
		}
		if variant == "" {
			variant = "original"
		}
		switch purpose {
		case "display":
			if variant == "original" {
				variant = "preview"
				if res.Kind == "video" || res.Kind == "audio" {
					variant = "playback"
				}
			}
			if variant != "preview" && variant != "thumbnail" && variant != "playback" && variant != "poster" {
				continue
			}
		case "copy", "download", "browser-process":
			variant = "original"
			if purpose == "copy" {
				purpose = "download"
			}
		default:
			results[i].Error = &ResourceAccessFailure{Code: 403, Reason: "resource_purpose_unavailable", Message: "该素材用途暂不可用"}
			continue
		}
		id := strconv.Itoa(i)
		items = append(items, platformclient.AccessItem{ItemID: id, AssetID: ref.AssetID, VersionID: ref.VersionID, Purpose: purpose, Variant: variant})
		files[id] = ref.FileID
	}
	if len(items) == 0 {
		return results, c.response, nil
	}
	response, err := c.m.Access(ctx, platformclient.AccessRequest{Items: items})
	c.collect(response.Response)
	if err != nil {
		return nil, c.response, studioMaterialError(err)
	}
	now := time.Now().UTC()
	for _, access := range response.Data.Results {
		i, err := strconv.Atoi(access.ItemID)
		if err != nil || i < 0 || i >= len(results) {
			return nil, c.response, NewAppError(502, "素材访问响应不匹配")
		}
		if access.Status != "ready" {
			results[i].Error = &ResourceAccessFailure{Code: 503, Reason: "material_access_unavailable", Message: "素材暂时不可访问，请稍后重试"}
			continue
		}
		u, err := url.Parse(access.URL)
		expires, expiryErr := time.Parse(time.RFC3339, access.ExpiresAt)
		refresh, refreshErr := time.Parse(time.RFC3339, access.RefreshAt)
		if access.FileID != files[access.ItemID] || err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || expiryErr != nil || refreshErr != nil || !expires.After(now) || refresh.After(expires) || access.Revision == "" {
			return nil, c.response, NewAppError(502, "素材访问凭据无效")
		}
		fallback := ""
		if access.FallbackReason != nil {
			fallback = *access.FallbackReason
		}
		results[i].Error = nil
		results[i].Access = &ResourceAccess{ResourceID: requests[i].ResourceID, RequestedVariant: assets.ResourceVariant(access.RequestedVariant), ActualVariant: assets.ResourceVariant(access.ActualVariant), URL: access.URL, Delivery: assets.DeliveryCDN, IssuedAt: now, ExpiresAt: &expires, RefreshAt: refresh, Revision: access.Revision, FallbackReason: fallback}
	}
	return results, c.response, nil
}
