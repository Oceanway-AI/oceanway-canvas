package handler

import (
	"encoding/json"
	"testing"
	"time"

	"yingce/backend/internal/assets"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestStudioTransferredConversationProtectsHistoricalResults(t *testing.T) {
	f := newStudioFixture(t)
	user := studioLibraryResource(t, f, "conversation-output")
	for revision := 0; revision < 2; revision++ {
		messages := []any{}
		if revision == 0 {
			messages = append(messages, map[string]any{"id": "answer", "role": "assistant", "detail": map[string]any{"resultStorageKeys": []string{"resource:conversation-output"}}})
		}
		body, _ := json.Marshal(map[string]any{"id": "handoff-history", "name": "结果会话", "revision": revision, "nodes": []any{}, "connections": []any{}, "chatMessages": messages, "createdAt": time.Now().UTC(), "updatedAt": time.Now().UTC()})
		if _, err := f.svc.UpsertUserCanvasProject(user.ID, body); err != nil {
			t.Fatal(err)
		}
	}
	history, err := f.svc.CanvasHistory(user.ID, "handoff-history")
	if err != nil || len(history.Snapshots) != 1 {
		t.Fatalf("missing history: %+v %v", history, err)
	}
	var count int64
	if err := f.db.Model(&model.CanvasSnapshotResource{}).Where("snapshot_id = ? AND resource_id = ?", history.Snapshots[0].ID, "conversation-output").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("retained chat result lacks history protection: count=%d %v", count, err)
	}
}

func TestStudioNativeDeletionCannotRemovePlatformResources(t *testing.T) {
	for _, action := range []string{"candidate", "detached", "asset-purge", "admin", "direct", "announcement-draft", "announcement-replace"} {
		t.Run(action, func(t *testing.T) {
			f := newStudioFixture(t)
			user := studioLibraryResource(t, f, "managed-file")
			repo := repository.New(f.db)
			old := time.Now().Add(-48 * time.Hour)
			if err := f.db.Model(&model.Resource{}).Where("id = ?", "managed-file").Updates(map[string]any{"created_at": old, "updated_at": old}).Error; err != nil {
				t.Fatal(err)
			}
			local := model.Resource{ID: "local-file", UserID: user.ID, Provider: "local", Status: model.ResourceStatusReady, CreatedAt: old, UpdatedAt: old}
			if err := f.db.Create(&local).Error; err != nil {
				t.Fatal(err)
			}
			if action == "candidate" {
				candidates, err := repo.ResourceCleanupCandidates(time.Now(), time.Now(), 100)
				if err != nil || len(candidates) != 1 || candidates[0].ID != local.ID {
					t.Fatalf("local cleanup includes managed file: %+v %v", candidates, err)
				}
				return
			}
			// A stale/forged caller snapshot must not bypass the provider stored in DB.
			rows := []model.Resource{local, {ID: "managed-file", UserID: user.ID, Provider: "local"}}
			jobs := []model.ResourceDeletionJob{{ID: "unsafe-delete", UserID: user.ID, Provider: "local", ObjectKey: "should-not-delete"}}
			var err error
			switch action {
			case "detached":
				err = repo.DeleteDetachedResources(rows, jobs)
			case "asset-purge":
				asset := model.Asset{ID: "asset", UserID: user.ID, PayloadJSON: `{"data":{"storageKey":"resource:managed-file"}}`}
				snapshot := model.CanvasSnapshot{ID: "history", UserID: user.ID, CanvasID: "canvas", Revision: 1, PayloadJSON: `{}`}
				for _, row := range []any{&asset, &snapshot, &model.CanvasSnapshotResource{SnapshotID: snapshot.ID, ResourceID: "managed-file"}} {
					if err := f.db.Create(row).Error; err != nil {
						t.Fatal(err)
					}
				}
				err = repo.DeleteAssetsAndResources(user.ID, []string{asset.ID}, []string{local.ID, "managed-file"}, jobs, true)
				for _, row := range []any{&model.Asset{}, &model.CanvasSnapshotResource{}} {
					var count int64
					if e := f.db.Model(row).Count(&count).Error; e != nil || count != 1 {
						t.Errorf("rejected purge changed %T: %d %v", row, count, e)
					}
				}
			case "admin":
				err = repo.DeleteAdminResources(rows, jobs, []model.AdminAuditEvent{{ID: "audit-1"}, {ID: "audit-2"}})
			case "direct":
				err = repo.DeleteResource(user.ID, "managed-file")
			case "announcement-draft":
				if e := f.db.Create(&model.AnnouncementImageDraft{ResourceID: "managed-file", UserID: user.ID}).Error; e != nil {
					t.Fatal(e)
				}
				err = repo.DiscardAnnouncementImageDraft(user.ID, &rows[1], &jobs[0])
			case "announcement-replace":
				announcement := model.Announcement{ID: "announcement", ImageResourceID: "managed-file", Title: "before"}
				if e := f.db.Create(&announcement).Error; e != nil {
					t.Fatal(e)
				}
				announcement.ImageResourceID = ""
				announcement.Title = "after"
				err = repo.UpdateAnnouncementWithImage(&announcement, user.ID, "", &rows[1], &jobs[0])
				var saved model.Announcement
				if e := f.db.First(&saved, "id = ?", announcement.ID).Error; e != nil || saved.Title != "before" || saved.ImageResourceID != "managed-file" {
					t.Errorf("rejected update changed announcement: %+v %v", saved, e)
				}
			}
			if err == nil {
				t.Errorf("%s deleted a Platform Resource without releasing its binding", action)
			}
			for _, check := range []struct {
				row  any
				want int64
			}{{&model.Resource{}, 2}, {&model.StudioMaterialReference{}, 1}, {&model.ResourceDeletionJob{}, 0}, {&model.AdminAuditEvent{}, 0}} {
				var count int64
				if e := f.db.Model(check.row).Count(&count).Error; e != nil || count != check.want {
					t.Errorf("%T changed: got=%d want=%d %v", check.row, count, check.want, e)
				}
			}
		})
	}
}

func TestStudioDurableAgentReferencesSurviveLeaseExpiry(t *testing.T) {
	f := newStudioFixture(t)
	user := studioLibraryResource(t, f, "agent-history-file")
	for _, row := range []any{
		&model.CloudAgentExecution{ID: "retained", UserID: user.ID, Status: "completed", StateJSON: `{"transientReferences":{"input":{"storageKey":"resource:agent-history-file"}}}`},
		&model.CloudAgentMessageRecord{RunID: "retained", UserID: user.ID, Kind: "canonical", Sequence: 1, MessageJSON: `{"role":"user","content":[{"type":"image_url","image_url":{"url":"resource:agent-message-file"}}]}`},
		&model.CloudAgentEventRecord{RunID: "retained", UserID: user.ID, Sequence: 1, EventJSON: `{"type":"tool_result","payload":{"result":{"storageKey":"resource:agent-event-file"}}}`},
		&model.CloudAgentCanvasMutation{ID: "undo", RunID: "retained", UserID: user.ID, CanvasID: "canvas", Status: "applied", BeforeJSON: `{"nodes":[{"metadata":{"storageKey":"resource:agent-undo-file"}}]}`},
		&model.CloudAgentPiSession{RunID: "retained", UserID: user.ID, SessionJSONL: "{\"type\":\"session\"}\n{\"type\":\"message\",\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"image_url\",\"image_url\":{\"url\":\"resource:agent-pi-file\"}}]}}\n"},
		&model.CloudAgentExecution{ID: "foreign", UserID: "other", Status: "completed", StateJSON: `{"resourceId":"foreign-only"}`},
		&model.CloudAgentExecution{ID: "diagnostic", UserID: user.ID, Status: "failed", StateJSON: `{"prompt":"resource:diagnostic-only","log":"/api/resources/diagnostic-only/file"}`},
		&model.CloudAgentResourceLease{UserID: user.ID, RunID: "retained", OwnerID: "old-approval", ResourceID: "agent-history-file", ExpiresAt: time.Now().Add(-time.Hour)},
	} {
		if err := f.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Recreate the repository and remove expired leases; persisted originals must suffice.
	repo := repository.New(f.db)
	if err := repo.ReleaseExpiredCloudAgentResourceLeases(time.Now()); err != nil {
		t.Fatal(err)
	}
	ids := []string{"agent-history-file", "agent-message-file", "agent-event-file", "agent-undo-file", "agent-pi-file", "foreign-only", "diagnostic-only"}
	snapshot, err := repo.ResourceReferenceSnapshot(user.ID, "", ids)
	if err != nil {
		t.Fatal(err)
	}
	got := studioSnapshotReferenceIDs(snapshot, ids)
	for _, id := range ids[:5] {
		if _, ok := got[id]; !ok {
			t.Errorf("retained Agent document lost %s", id)
		}
	}
	for _, id := range ids[5:] {
		if _, ok := got[id]; ok {
			t.Errorf("foreign/free text became reference: %s", id)
		}
	}
}

func studioSnapshotReferenceIDs(snapshot repository.ResourceReferenceSnapshot, ids []string) map[string]struct{} {
	candidates := map[string]struct{}{}
	for _, id := range ids {
		candidates[id] = struct{}{}
	}
	got := map[string]struct{}{}
	for _, ref := range snapshot.Direct {
		got[ref.ResourceID] = struct{}{}
	}
	for _, doc := range snapshot.Documents {
		for _, raw := range []string{doc.PrimaryJSON, doc.SecondaryJSON} {
			for id := range assets.DocumentReferencedIDs(raw, candidates) {
				got[id] = struct{}{}
			}
		}
	}
	return got
}
