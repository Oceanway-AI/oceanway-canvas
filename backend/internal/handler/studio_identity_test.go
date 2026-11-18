package handler

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/service"
)

func TestStudioRejectsWritesFromOldIdentityClients(t *testing.T) {
	f := newStudioFixture(t)
	for _, version := range []string{"", "uuid-v0"} {
		r := httptest.NewRequest("PATCH", "https://studio.example.test/api/canvas-projects/old-draft/operations", strings.NewReader(`{"mutationId":"old-draft","operations":[{"type":"create_canvas","id":"old-draft","patch":{"title":"Old draft"}}]}`))
		r.Header.Set("Cookie", "platform_session=alice")
		r.Header.Set("Origin", "https://studio.example.test")
		r.Header.Set("X-Studio-Identity-Version", version)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "studio_identity_version_changed") {
			t.Fatalf("old client accepted: %d %s", w.Code, w.Body.String())
		}
	}
	var count int64
	if err := f.db.Model(&model.CanvasProject{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("old draft persisted: %d %v", count, err)
	}
	f.request(t, "PATCH", "/api/canvas-projects/new-draft/operations", "platform_session=alice", `{"mutationId":"new-draft","operations":[{"type":"create_canvas","id":"new-draft","patch":{"title":"New draft"}}]}`, 200)
}

func (f *studioFixture) user(t *testing.T, account string) model.User {
	t.Helper()
	w := f.request(t, "GET", "/api/auth/session", "platform_session="+account, "", 200)
	var body struct{ Data struct{ User model.User } }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.User.ID == "" {
		t.Fatalf("missing principal: %s (%v)", w.Body.String(), err)
	}
	return body.Data.User
}

func TestStudioRejectsOldTabWritesAfterCookieAccountChanges(t *testing.T) {
	f := newStudioFixture(t)
	for _, actor := range []string{"", "101"} {
		r := httptest.NewRequest("PATCH", "https://studio.example.test/api/canvas-projects/alice-draft/operations", strings.NewReader(`{"mutationId":"stale-owner","operations":[{"type":"create_canvas","id":"alice-draft","patch":{"title":"Alice draft"}}]}`))
		r.Header.Set("Cookie", "platform_session=bob")
		r.Header.Set("Origin", "https://studio.example.test")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Studio-Identity-Version", "platform-id-v1")
		r.Header.Set("X-Studio-Actor-ID", actor)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "studio_actor_changed") {
			t.Fatalf("stale account write accepted: %d %s", w.Code, w.Body.String())
		}
	}
	var count int64
	if err := f.db.Model(&model.CanvasProject{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("stale draft crossed accounts: %d %v", count, err)
	}
}

func TestStudioDirectIdentityWithoutLocalAccounts(t *testing.T) {
	for _, base := range []string{"", "/studio"} {
		t.Run(base, func(t *testing.T) {
			f := newStudioFixtureWithBase(t, base)
			if err := f.db.Migrator().DropTable("user_identities", "users"); err != nil {
				t.Fatal(err)
			}
			if got := f.user(t, "alice").ID; got != "101" {
				t.Fatalf("principal=%q", got)
			}
			if got := f.user(t, "bob").ID; got != "202" {
				t.Fatalf("principal=%q", got)
			}
			body := `{"mutationId":"direct-owner","operations":[{"type":"create_canvas","id":"direct-canvas","patch":{"title":"Direct ID"}}]}`
			f.request(t, "PATCH", "/api/canvas-projects/direct-canvas/operations", "platform_session=alice", body, 200)
			f.request(t, "GET", "/api/canvas-projects/direct-canvas", "platform_session=alice", "", 200)
			f.request(t, "GET", "/api/canvas-projects/direct-canvas", "platform_session=bob", "", 404)
			var owner string
			if err := f.db.Table("canvas_projects").Select("user_id").Where("id = ?", "direct-canvas").Scan(&owner).Error; err != nil || owner != "101" {
				t.Fatalf("owner=%q err=%v", owner, err)
			}
		})
	}
}

func TestStudioSkillsWithoutLocalAccounts(t *testing.T) {
	f := newStudioFixture(t)
	if err := f.db.Migrator().DropTable("user_identities", "users"); err != nil {
		t.Fatal(err)
	}
	f.avatar = "https://cdn.example.test/profile.png"
	body := `{"skillName":"Direct principal","description":"Author snapshot","instruction":"Use current authority.","tag":"creative","author":{"id":"202","displayName":"forged"}}`
	created := f.request(t, "POST", "/api/skills", "platform_session=alice", body, 200)
	var result struct {
		Data struct{ Skill service.SkillItem }
	}
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	skill := result.Data.Skill
	if skill.OwnerUID != "101" || skill.EffectiveUser.Name != "Platform user" || skill.EffectiveUser.AvatarURL != f.avatar {
		t.Fatalf("author=%+v", skill.EffectiveUser)
	}
	list := f.request(t, "GET", "/api/skills?search="+url.QueryEscape("Platform user"), "platform_session=bob", "", 200)
	if !strings.Contains(list.Body.String(), skill.SkillID) {
		t.Fatal("author search lost snapshot")
	}
	f.request(t, "PUT", "/api/skills/"+skill.SkillID, "platform_session=bob", body, 403)
	f.request(t, "PUT", "/api/skills/"+skill.SkillID, "platform_session=alice", body, 200)
	f.request(t, "POST", "/api/skills/library-categories", "platform_session=alice", `{"name":"Personal","scope":"personal"}`, 200)
	f.request(t, "POST", "/api/skills/library-categories", "platform_session=alice", `{"name":"Shared","scope":"platform"}`, 403)
	f.admin = true
	f.request(t, "POST", "/api/skills/library-categories", "platform_session=alice", `{"name":"Shared","scope":"platform"}`, 200)
	f.admin = false
	f.request(t, "POST", "/api/skills/library-categories", "platform_session=alice", `{"name":"Revoked","scope":"platform"}`, 403)
	f.request(t, "DELETE", "/api/skills/"+skill.SkillID, "platform_session=alice", "", 200)
}
