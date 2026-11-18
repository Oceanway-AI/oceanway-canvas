package app

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"yingce/backend/internal/assets"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioLocalStorageRejectsUserMaterialWithoutCreatingResource(t *testing.T) {
	s := newResourceTestService(t)
	s.studioPlatform = &platformintegration.Gateway{}
	_, stored, err := s.storeResource("123", "image", "test.png", "image/png", 3, 1, 1, 0, bytes.NewReader([]byte("png")), nil, false)
	var failure *AppError
	if !errors.As(err, &failure) || failure.Code != 501 || stored {
		t.Fatalf("user material bypassed Platform: stored=%v err=%v", stored, err)
	}
	rows, err := s.repo.Resources("123", 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rejected material left resources: %v %v", rows, err)
	}
	resource, stored, err := s.storeResource("123", "image", "logo.png", "image/png", 3, 1, 1, 0, bytes.NewReader([]byte("png")), nil, true)
	if err != nil || !stored || resource.Provider != "local" {
		t.Fatalf("system asset not saved: %v %v", resource, err)
	}
	stream, err := s.openResourceRange("123", resource, "")
	if err != nil {
		t.Fatal(err)
	}
	stream.Body.Close()
	if _, err := s.openResourceRange("456", resource, ""); err == nil {
		t.Fatal("another account read system resource")
	}
}

func TestStudioLocalStorageCannotAccessOrDeletePlatformResource(t *testing.T) {
	s := newResourceTestService(t)
	resource := &model.Resource{ID: "remote", UserID: "123", Provider: "platform", Status: model.ResourceStatusReady, ObjectKey: "asset/version"}
	if _, err := s.openResourceRange("123", resource, ""); err == nil {
		t.Fatal("Platform resource read through local storage")
	}
	if _, err := s.storeResourceObject(resource, "image.png", bytes.NewReader([]byte("png"))); err == nil {
		t.Fatal("Platform resource written through local storage")
	}
	if err := s.deleteStoredResourceObject("123", resource); err == nil {
		t.Fatal("Platform resource physically deleted")
	}
	signed := false
	_, err := assets.ResolveAccess(resource, assets.AccessPolicy{}, assets.AccessOptions{}, time.Now(), func(assets.ResourceVariant, time.Time) (string, error) {
		signed = true
		return "", nil
	})
	if err == nil || signed {
		t.Fatal("Platform resource received a local access signature")
	}
}

func TestStudioExecutionEncryptionSurvivesStorageConfigurationRemoval(t *testing.T) {
	directory := t.TempDir()
	first := &Service{dataDir: directory}
	encrypted, err := first.encryptSettingSecret("execution-test-authorization")
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{dataDir: directory}
	plain, err := restarted.decryptSettingSecret(encrypted)
	if err != nil || plain != "execution-test-authorization" {
		t.Fatalf("execution authorization lost after restart: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".settings-key" {
		t.Fatalf("unexpected encryption state: %v %v", entries, err)
	}
}
