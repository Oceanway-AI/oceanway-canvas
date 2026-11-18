package app

import (
	"strings"
	"testing"
	"yingce/backend/internal/assets"
	"yingce/backend/internal/model"
)

func TestEcommerceExportRequiresResourceOwner(t *testing.T) {
	svc := newResourceTestService(t)
	resource := model.Resource{ID: "commerce-export-image", UserID: "owner", Kind: "image", Provider: "local", Status: model.ResourceStatusReady, ObjectKey: "image.png", MimeType: "image/png", Size: 1024}
	if err := svc.repo.CreateResource(&resource); err != nil {
		t.Fatal(err)
	}
	request := []ResourceAccessRequest{{ResourceID: resource.ID, AccessOptions: ResourceAccessOptions{Purpose: assets.PurposeEcommerceExport}}}
	result, err := svc.ResourceAccessBatch("owner", request)
	if err != nil || len(result) != 1 || result[0].Access == nil || result[0].Access.Delivery != assets.DeliveryProxy || !strings.Contains(result[0].Access.URL, "purpose=ecommerce-export") {
		t.Fatalf("owner export access = %#v, %v", result, err)
	}
	other, err := svc.ResourceAccessBatch("other", request)
	if err != nil || len(other) != 1 || other[0].Access != nil || other[0].Error == nil {
		t.Fatalf("cross-user export accepted: %#v, %v", other, err)
	}
	if _, err := svc.ResourceAccessBatch("", request); err == nil {
		t.Fatal("guest export accepted")
	}
}
