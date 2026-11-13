package app

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/assets"
	"yingce/backend/internal/model"
)

// Browsing the shared library is an account read, not Resource adoption.
func (s *Service) StudioMaterialBrowseAccess(ctx context.Context, session platformclient.SessionContext, request platformclient.AccessRequest) (platformclient.AccessResponse, *platformclient.Response, error) {
	empty := platformclient.AccessResponse{}
	if len(request.Items) < 1 || len(request.Items) > 100 {
		return empty, nil, BadAuthRequest("每批素材访问须为 1–100 项")
	}
	items := map[string]platformclient.AccessItem{}
	for _, item := range request.Items {
		if item.ItemID == "" || items[item.ItemID].ItemID != "" || !studioMaterialID.MatchString(item.AssetID) || !studioMaterialID.MatchString(item.VersionID) || item.BindingID != "" {
			return empty, nil, BadAuthRequest("素材访问身份无效")
		}
		if !((item.Purpose == "display" && (item.Variant == "thumbnail" || item.Variant == "preview" || item.Variant == "poster" || item.Variant == "playback")) || (item.Purpose == "download" && item.Variant == "original")) {
			return empty, nil, BadAuthRequest("素材浏览用途无效")
		}
		items[item.ItemID] = item
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.CurrentMaterials(session).Access(ctx, request)
	if err != nil {
		return empty, result.Response, studioMaterialError(err)
	}
	if len(result.Data.Results) != len(items) {
		return empty, result.Response, NewAppError(502, "素材访问响应不完整")
	}
	for _, access := range result.Data.Results {
		item, exists := items[access.ItemID]
		if !exists || access.AssetID != item.AssetID || access.VersionID != item.VersionID || access.RequestedVariant != item.Variant {
			return empty, result.Response, NewAppError(502, "素材访问响应不匹配")
		}
		delete(items, access.ItemID)
		if access.Status != "ready" {
			continue
		}
		u, urlErr := url.Parse(access.URL)
		expires, expErr := time.Parse(time.RFC3339, access.ExpiresAt)
		refresh, refreshErr := time.Parse(time.RFC3339, access.RefreshAt)
		if urlErr != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || expErr != nil || refreshErr != nil || !expires.After(time.Now()) || refresh.After(expires) || access.Revision == "" || access.FileID == "" || access.ActualVariant != item.Variant {
			return empty, result.Response, NewAppError(502, "素材访问凭据无效")
		}
	}
	return result.Data, result.Response, nil
}

func (s *Service) StudioResourceAccess(ctx context.Context, user *model.User, session platformclient.SessionContext, requests []ResourceAccessRequest) ([]ResourceAccessResult, *platformclient.Response, error) {
	if user == nil || !user.PlatformAuthenticated || user.PlatformSessionID == "" {
		return nil, nil, Unauthorized("登录状态已失效")
	}
	userID := user.ID
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
		if err != nil || ref.BindingID == "" {
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
		items = append(items, platformclient.AccessItem{ItemID: id, AssetID: ref.AssetID, VersionID: ref.VersionID, BindingID: ref.BindingID, Purpose: purpose, Variant: variant})
		files[id] = ref.FileID
	}
	if len(items) == 0 {
		return results, c.response, nil
	}
	if session.Origin == "" {
		m, err := s.studioMaterialReadAuthority(ctx, user, session)
		if err != nil {
			return nil, c.response, err
		}
		c.m = m
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

// Browsing the account library only needs read access to a fixed asset version.
// A resource binding is created separately when the user inserts it in Studio.
func (s *Service) StudioLibraryFile(ctx context.Context, user *model.User, session platformclient.SessionContext, asset, version, variant string) (string, *platformclient.Response, error) {
	if !studioMaterialID.MatchString(asset) || !studioMaterialID.MatchString(version) {
		return "", nil, BadAuthRequest("素材版本无效")
	}
	purpose := "display"
	switch variant {
	case "thumbnail", "poster", "preview", "playback":
	case "original":
		purpose = "download"
	default:
		return "", nil, BadAuthRequest("素材文件类型无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	materials := s.studioPlatform.Client.CurrentMaterials(session)
	if session.Origin == "" {
		var err error
		materials, err = s.studioMaterialReadAuthority(ctx, user, session)
		if err != nil {
			return "", nil, err
		}
	}
	result, err := materials.Access(ctx, platformclient.AccessRequest{Items: []platformclient.AccessItem{{ItemID: "file", AssetID: asset, VersionID: version, Variant: variant, Purpose: purpose}}})
	if err != nil {
		return "", result.Response, studioMaterialError(err)
	}
	if len(result.Data.Results) != 1 || result.Data.Results[0].Status != "ready" {
		return "", result.Response, NotFound("素材文件尚未就绪或不可访问")
	}
	item := result.Data.Results[0]
	u, err := url.Parse(item.URL)
	expires, expiryErr := time.Parse(time.RFC3339, item.ExpiresAt)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || expiryErr != nil || !expires.After(time.Now()) {
		return "", result.Response, NewAppError(502, "素材访问凭据无效")
	}
	return item.URL, result.Response, nil
}
