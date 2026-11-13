package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestStudioCanvasRemovalKeepsOtherCanvasTaskAndHistory(t *testing.T) {
	f := newStudioFixture(t)
	user := studioLibraryResource(t, f, "shared-image")
	save := func(id string, revision int64, withNode bool) {
		t.Helper()
		nodes := []any{}
		if withNode {
			nodes = append(nodes, map[string]any{"id": "image", "type": "image", "position": map[string]int{"x": 0, "y": 0}, "width": 320, "height": 180, "metadata": map[string]any{"storageKey": "resource:shared-image", "content": "/api/resources/shared-image/file"}})
		}
		raw, _ := json.Marshal(map[string]any{"id": id, "name": id, "revision": revision, "nodes": nodes, "connections": []any{}, "createdAt": time.Now().UTC(), "updatedAt": time.Now().UTC()})
		if _, err := f.svc.UpsertUserCanvasProject(user.ID, raw); err != nil {
			t.Fatal(err)
		}
	}
	save("canvas-a", 0, true)
	save("canvas-b", 0, true)
	task := model.Task{ID: "completed-original", UserID: user.ID, Type: "canvas_image", Status: model.TaskStatusSucceeded, ResultJSON: `{"images":[{"storageKey":"resource:shared-image"}]}`}
	if err := f.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	save("canvas-a", 1, false)
	history, err := f.svc.CanvasHistory(user.ID, "canvas-a")
	if err != nil || len(history.Snapshots) != 1 {
		t.Fatalf("history missing: %#v %v", history, err)
	}
	if _, err := f.svc.CanvasHistorySnapshot("foreign", "canvas-a", history.Snapshots[0].ID); err == nil {
		t.Fatal("history leaked across accounts")
	}
	revision := int64(2)
	if _, err := f.svc.RestoreCanvasHistory(user.ID, "canvas-a", history.Snapshots[0].ID, &revision); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(f.db)
	for _, id := range []string{"canvas-a", "canvas-b"} {
		p, err := repo.CanvasProjectForUser(user.ID, id)
		if err != nil || !strings.Contains(p.PayloadJSON, "resource:shared-image") {
			t.Fatalf("canvas lost file: %s %v", id, err)
		}
	}
	if err := f.svc.DeleteUserCanvasProject(user.ID, "canvas-a"); err != nil {
		t.Fatal(err)
	}
	var original model.Task
	if err := f.db.First(&original, "id = ?", task.ID).Error; err != nil || original.ResultJSON != task.ResultJSON {
		t.Fatal("original task was altered")
	}
	for _, check := range []struct {
		model any
		want  int64
	}{{&model.Resource{}, 1}, {&model.StudioMaterialReference{}, 1}, {&model.ResourceDeletionJob{}, 0}} {
		var count int64
		f.db.Model(check.model).Count(&count)
		if count != check.want {
			t.Fatalf("%T: got %d", check.model, count)
		}
	}
}

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
