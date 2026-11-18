package app

import (
	"bytes"
	"errors"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/testdb"
)

func TestNormalizeSingleByteRange(t *testing.T) {
	tests := map[string]string{
		"bytes=0-1023":       "bytes=0-1023",
		"bytes=1024-":        "bytes=1024-",
		"bytes=-2048":        "bytes=-2048",
		"bytes=0-1,10-20":    "",
		"items=0-10":         "",
		"bytes=invalid-1024": "",
	}
	for input, expected := range tests {
		if actual := normalizeSingleByteRange(input); actual != expected {
			t.Fatalf("normalizeSingleByteRange(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestHydrateNewAPIChannel1ResourceUsesSignedLocalURL(t *testing.T) {
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "true")
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	svc := newResourceTestService(t)
	t.Setenv("CANVAS_PUBLIC_BASE_URL", "https://127.0.0.1")
	resource := model.Resource{ID: "resource-local", UserID: "user-1", Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "local.png"}
	if err := svc.repo.CreateResource(&resource); err != nil {
		t.Fatal(err)
	}
	media := providerMedia{StorageKey: "resource:resource-local"}
	if err := svc.hydrateProviderMedia("user-1", &media, providerMediaHydrationPolicy{requireURL: true}); err != nil {
		t.Fatalf("hydrateProviderMedia() error = %v", err)
	}
	if !strings.HasPrefix(media.URL, "https://127.0.0.1/api/public/resources/resource-local/file?") || !strings.Contains(media.URL, "signature=") || media.DataURL != "" {
		t.Fatalf("media = %#v", media)
	}
	stored, err := svc.repo.Resource("resource-local")
	if err != nil || stored.Provider != "local" {
		t.Fatalf("resource provider changed: %#v, %v", stored, err)
	}
}

func TestPublicResourceSignatureRejectsExpiredAndAlteredLinks(t *testing.T) {
	svc := newResourceTestService(t)
	expires := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	signature, err := svc.signPublicResource("resource-local", expires)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.verifyPublicResourceSignature("resource-local", expires, signature); err != nil {
		t.Fatalf("verifyPublicResourceSignature() error = %v", err)
	}
	if err := svc.verifyPublicResourceSignature("resource-other", expires, signature); err == nil {
		t.Fatal("verifyPublicResourceSignature() accepted another resource ID")
	}
	expired := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	expiredSignature, err := svc.signPublicResource("resource-local", expired)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.verifyPublicResourceSignature("resource-local", expired, expiredSignature); err == nil {
		t.Fatal("verifyPublicResourceSignature() accepted an expired link")
	}
}

func newResourceTestService(t *testing.T) *Service {
	t.Helper()
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.UserDailyUploadUsage{}, &model.Resource{}, &model.UploadReservation{}); err != nil {
		t.Fatal(err)
	}
	return &Service{repo: repository.New(db), dataDir: t.TempDir()}
}

func TestStoreResourceReusesReadyUploadIdentity(t *testing.T) {
	svc := newResourceTestService(t)
	uploadKey := normalizedResourceUploadKey([]string{"image:user-1:logical-upload"})
	first, stored, err := svc.storeResource("user-1", "image", "first.png", "image/png", 7, 1, 1, 0, bytes.NewReader([]byte("payload")), uploadKey, false)
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("first upload was not stored")
	}
	second, stored, err := svc.storeResource("user-1", "image", "second.png", "image/png", 7, 1, 1, 0, bytes.NewReader([]byte("payload")), uploadKey, false)
	if err != nil {
		t.Fatal(err)
	}
	if stored || second.ID != first.ID || second.ObjectKey != first.ObjectKey {
		t.Fatalf("idempotent upload = %#v, stored=%v; first=%#v", second, stored, first)
	}
	resources, err := svc.repo.Resources("user-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(resources))
	}
}

func TestRetryStoredResourceKeepsOriginalObjectKey(t *testing.T) {
	svc := newResourceTestService(t)
	uploadKey := normalizedResourceUploadKey([]string{"image:user-1:retry-upload"})
	failed := &model.Resource{
		ID: "resource-failed", UserID: "user-1", Kind: "image", Status: model.ResourceStatusFailed,
		Provider: "local", ObjectKey: "users/user-1/image/fixed.png", MimeType: "image/png", Size: 7,
		UploadKey: uploadKey, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateResource(failed); err != nil {
		t.Fatal(err)
	}
	retried, err := svc.retryStoredResource("user-1", failed, "image", "image/png", 7, bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != failed.ID || retried.ObjectKey != "users/user-1/image/fixed.png" || retried.Status != model.ResourceStatusReady {
		t.Fatalf("retried resource = %#v", retried)
	}
	resources, err := svc.repo.Resources("user-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(resources))
	}
	day := time.Now().UTC().Format("2006-01-02")
	usage, err := svc.repo.DailyUploadBytes("user-1", day)
	if err != nil {
		t.Fatal(err)
	}
	if usage != 7 {
		t.Fatalf("daily upload usage = %d, want 7", usage)
	}
}

func TestRetryStoredResourceReleasesDailyQuotaAfterFailure(t *testing.T) {
	svc := newResourceTestService(t)
	uploadKey := normalizedResourceUploadKey([]string{"image:user-1:failed-retry"})
	failed := &model.Resource{
		ID: "resource-failed-retry", UserID: "user-1", Kind: "image", Status: model.ResourceStatusFailed,
		Provider: "local", ObjectKey: "users/user-1/image/failed.png", MimeType: "image/png", Size: 7,
		UploadKey: uploadKey, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateResource(failed); err != nil {
		t.Fatal(err)
	}
	_, err := svc.retryStoredResource("user-1", failed, "image", "image/png", 7, iotest.ErrReader(errors.New("write failed")))
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("retryStoredResource() error = %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	usage, usageErr := svc.repo.DailyUploadBytes("user-1", day)
	if usageErr != nil {
		t.Fatal(usageErr)
	}
	if usage != 0 {
		t.Fatalf("daily upload usage = %d, want 0", usage)
	}
}

func TestLegacyMediaMigrationSkipsInvalidDataURL(t *testing.T) {
	svc := &Service{}
	input := map[string]interface{}{
		"history": []interface{}{
			map[string]interface{}{"content": "data:video/mp4;base64,broken"},
		},
	}

	result, err := svc.persistLegacyGeneratedMediaResult("user-1", input)
	if err != nil {
		t.Fatalf("persistLegacyGeneratedMediaResult() error = %v", err)
	}
	history := result["history"].([]interface{})
	content := history[0].(map[string]interface{})["content"]
	if content != "data:video/mp4;base64,broken" {
		t.Fatalf("invalid legacy content changed to %v", content)
	}
}

func TestGeneratedMediaRejectsInvalidDataURL(t *testing.T) {
	svc := &Service{}
	_, err := svc.persistGeneratedMediaResult("user-1", map[string]interface{}{
		"content": "data:video/mp4;base64,broken",
	})
	if err == nil {
		t.Fatal("persistGeneratedMediaResult() error = nil, want invalid data URL error")
	}
}

func TestPersistGeneratedMediaAppliesStoredFileQuota(t *testing.T) {
	svc := newResourceTestService(t)
	if err := svc.repo.Create(&model.Resource{
		ID:     "existing",
		UserID: "user-1",
		Status: model.ResourceStatusReady,
		Size:   gigabytes(defaultRuntimePolicy().Resource.StoredFileGB) - 1,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.persistGeneratedMediaResult("user-1", map[string]interface{}{
		"image": map[string]interface{}{"dataUrl": "data:image/png;base64,YQ=="},
	})
	if err == nil || !strings.Contains(err.Error(), "20GB 上限") {
		t.Fatalf("persistGeneratedMediaResult() error = %v", err)
	}
}
