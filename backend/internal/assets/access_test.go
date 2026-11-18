package assets

import (
	"strings"
	"testing"
	"time"
	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

func testReadyResource(provider string) *model.Resource {
	return &model.Resource{
		ID: "resource-1", UserID: "user-1", Provider: provider, ObjectKey: "users/user-1/image/test.png",
		Status: model.ResourceStatusReady, MimeType: "image/png", ETag: "etag-1",
	}
}

func testPlatformURL(variants *[]ResourceVariant) func(ResourceVariant, time.Time) (string, error) {
	return func(variant ResourceVariant, _ time.Time) (string, error) {
		*variants = append(*variants, variant)
		return "https://canvas.example/api/resources/resource-1/file", nil
	}
}

func TestResolveAccessVariantAndExpiryContract(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	resource := testReadyResource("local")
	resource.Kind = "video"
	resource.PlaybackStatus = model.PlaybackStatusReady
	resource.PlaybackObjectKey = "playback/resource-1.mp4"
	var variants []ResourceVariant
	access, err := ResolveAccess(resource, AccessPolicy{}, AccessOptions{Purpose: PurposeProcess, Variant: VariantPlayback, ExpiresAt: now.Add(90 * time.Second)}, now, testPlatformURL(&variants))
	if err != nil {
		t.Fatal(err)
	}
	if access.ActualVariant != VariantPlayback || access.ExpiresAt == nil || !access.ExpiresAt.Equal(now.Add(90*time.Second)) {
		t.Fatalf("playback access = %#v", access)
	}
	if len(variants) != 1 || variants[0] != VariantPlayback {
		t.Fatalf("platform URL variants = %#v, want playback", variants)
	}

	provider, err := ResolveAccess(resource, AccessPolicy{}, AccessOptions{Purpose: PurposeProvider}, now, testPlatformURL(&variants))
	if err != nil {
		t.Fatal(err)
	}
	if provider.ExpiresAt == nil || !provider.ExpiresAt.Equal(now.Add(4*time.Hour)) {
		t.Fatalf("provider access expiry = %v, want %v", provider.ExpiresAt, now.Add(4*time.Hour))
	}
}

func TestResolveAccessRejectsPendingResource(t *testing.T) {
	resource := testReadyResource("local")
	resource.Status = model.ResourceStatusPending
	_, err := ResolveAccess(resource, AccessPolicy{}, AccessOptions{Purpose: PurposeDisplay}, time.Now(), func(ResourceVariant, time.Time) (string, error) {
		return "", nil
	})
	if err == nil || !strings.Contains(string(errorReason(err)), "resource_not_ready") {
		t.Fatalf("ResolveAccess() error = %v, want resource_not_ready", err)
	}
}

func errorReason(err error) string {
	if appErr, ok := err.(*kernel.AppError); ok {
		return string(appErr.Reason)
	}
	return err.Error()
}

func TestResolveAccessUsesRuntimePolicyTTL(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	resource := testReadyResource("local")
	setting := AccessPolicy{AccessURLTTL: 2 * time.Hour, ProviderAccessURLTTL: 6 * time.Hour}
	var variants []ResourceVariant
	// 展示地址按 ttl/4（30 分钟）对齐签名窗口：03:04:05 落在 03:00 窗口，过期于 05:00；模型输入仍按当前时间签发。
	for purpose, want := range map[AccessPurpose]time.Time{PurposeDisplay: time.Date(2026, time.January, 2, 5, 0, 0, 0, time.UTC), PurposeProvider: now.Add(6 * time.Hour)} {
		access, err := ResolveAccess(resource, setting, AccessOptions{Purpose: purpose}, now, testPlatformURL(&variants))
		if err != nil {
			t.Fatal(err)
		}
		if access.ExpiresAt == nil || !access.ExpiresAt.Equal(want) {
			t.Fatalf("%s access expiry = %v, want %v", purpose, access.ExpiresAt, want)
		}
	}
}

func TestResolveAccessAcceptsThumbnailVariantWithOriginalFallback(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	var variants []ResourceVariant
	access, err := ResolveAccess(testReadyResource("local"), AccessPolicy{}, AccessOptions{Purpose: PurposeDisplay, Variant: VariantThumbnail}, now, testPlatformURL(&variants))
	if err != nil {
		t.Fatalf("thumbnail variant must not fail (the canvas would fall back to the redirecting platform URL): %v", err)
	}
	if access.RequestedVariant != VariantThumbnail || access.ActualVariant != VariantOriginal || access.FallbackReason != "thumbnail_not_ready" {
		t.Fatalf("thumbnail access = %#v", access)
	}
	if len(variants) != 1 || variants[0] != VariantOriginal {
		t.Fatalf("platform URL variants = %#v, want original", variants)
	}
}
