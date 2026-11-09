package handler

import (
	"testing"
	"time"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestStudioHistoryProtectsOnlyTheReferencedPlatformResource(t *testing.T) {
	f := newStudioFixture(t)
	user := studioLibraryResource(t, f, "historical-image")
	unrelated := model.Resource{ID: "unrelated-image", UserID: user.ID, Provider: "platform", Status: model.ResourceStatusReady, Kind: "image"}
	snapshot := model.CanvasSnapshot{ID: "studio-history", CanvasID: "canvas-a", UserID: user.ID, Revision: 1, Title: "原画布", PayloadJSON: `{}`, CreatedAt: time.Now()}
	ref := model.CanvasSnapshotResource{SnapshotID: snapshot.ID, ResourceID: "historical-image"}
	for _, v := range []any{&unrelated, &snapshot, &ref} {
		if err := f.db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := repository.New(f.db)
	historical, err := repo.ResourceForUser(user.ID, "historical-image")
	if err != nil {
		t.Fatal(err)
	}
	yes, err := repo.CanvasHistoryReferencesObject(historical)
	if err != nil || !yes {
		t.Fatalf("history must protect its Resource: %v %v", yes, err)
	}
	yes, err = repo.CanvasHistoryReferencesObject(&unrelated)
	if err != nil || yes {
		t.Fatalf("blank storage coordinates must not alias distinct Platform files: %v %v", yes, err)
	}
	if err := f.db.Delete(&ref).Error; err != nil {
		t.Fatal(err)
	}
	yes, err = repo.CanvasHistoryReferencesObject(historical)
	if err != nil || yes {
		t.Fatalf("removing history must release only that usage: %v %v", yes, err)
	}
	var count int64
	f.db.Model(&model.StudioMaterialReference{}).Count(&count)
	if count != 1 {
		t.Fatal("history removal changed the adopted file binding")
	}
}
