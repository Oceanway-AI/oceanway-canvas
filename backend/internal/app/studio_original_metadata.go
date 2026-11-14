package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func originalStudioMetadata(asset platformclient.Asset) StudioResourceMeta {
	var meta StudioResourceMeta
	if (asset.Kind == "image" || asset.Kind == "video") && asset.File.Width > 0 && asset.File.Height > 0 {
		meta.Width, meta.Height = asset.File.Width, asset.File.Height
	}
	if (asset.Kind == "video" || asset.Kind == "audio") && asset.File.DurationMs > 0 {
		meta.DurationMs = asset.File.DurationMs
	}
	return meta
}

func (m StudioResourceMeta) withOriginal(original StudioResourceMeta) StudioResourceMeta {
	if original.Width > 0 && original.Height > 0 {
		m.Width, m.Height = original.Width, original.Height
	}
	if original.DurationMs > 0 {
		m.DurationMs = original.DurationMs
	}
	return m
}

func matchesStudioOriginal(ref model.StudioMaterialReference, asset platformclient.Asset) bool {
	return asset.Owner.Kind == "account" && asset.Owner.ID == ref.OwnerID &&
		asset.AssetID == ref.AssetID && asset.Version.AssetID == ref.AssetID && asset.Version.VersionID == ref.VersionID &&
		asset.Version.FileID == ref.FileID && asset.File.FileID == ref.FileID && asset.File.State == "ready" &&
		asset.File.SHA256 == ref.SHA256 && asset.File.ByteLength == ref.ByteLength
}

func (s *Service) reconcileStudioOriginal(ctx context.Context, resource *model.Resource, asset platformclient.Asset, ref model.StudioMaterialReference) error {
	if !matchesStudioOriginal(ref, asset) || asset.File.ContentType != resource.MimeType || (asset.Kind != resource.Kind && !(asset.Kind == "document" && resource.Kind == "file")) {
		return NewAppError(502, "素材版本与原文件身份不一致")
	}
	original := originalStudioMetadata(asset)
	saved := StudioResourceMeta{Width: resource.Width, Height: resource.Height, DurationMs: resource.DurationMs}
	if saved.withOriginal(original) == saved {
		return nil
	}
	return s.repo.WithContext(ctx).UpdateStudioOriginalMetadata(resource, original.Width, original.Height, original.DurationMs)
}

// The public resource read refreshes one fixed version. Internal reference and access
// checks use the local resource, so a batched access never fans out to Platform.
func (s *Service) StudioResourceWithMetadata(ctx context.Context, userID, id string, session platformclient.SessionContext) (*model.Resource, *platformclient.Response, error) {
	resource, err := s.StudioResource(ctx, userID, id)
	if err != nil || resource.Kind == "file" {
		return resource, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ref, err := s.repo.WithContext(ctx).StudioMaterialReference(userID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return resource, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if ref.BindingID == "" {
		return resource, nil, nil
	}
	result, err := s.studioPlatform.Client.CurrentMaterials(session).Asset(ctx, ref.AssetID, ref.VersionID)
	if err != nil {
		// Removed library entries can still have an active, locally owned resource.
		var upstream *platformclient.Error
		if errors.As(err, &upstream) && upstream.StatusCode == http.StatusNotFound {
			return resource, result.Response, nil
		}
		return nil, result.Response, studioMaterialError(err)
	}
	if err := s.reconcileStudioOriginal(ctx, resource, result.Data, *ref); err != nil {
		return nil, result.Response, err
	}
	return resource, result.Response, nil
}
