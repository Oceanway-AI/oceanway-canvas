package handler

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/service"
)

func TestStudioAgentSkillsUseNativeCatalogAndAccountIsolation(t *testing.T) {
	f := newStudioFixture(t)
	f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 200)
	var alice model.User
	if err := f.db.First(&alice).Error; err != nil {
		t.Fatal(err)
	}
	skill, err := f.svc.CreateSkill(alice.ID, service.SkillMutationRequest{SkillName: "Private reference helper", Description: "Private reference guidance", Instruction: "Use the selected reference only.", Tag: "creative", IsPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/skills?scope=created", "/api/skills/added", "/api/skills/presets", "/api/skills/library-categories", "/api/skills/recommendations"} {
		f.request(t, "GET", path, "platform_session=alice", "", 200)
	}
	f.request(t, "GET", "/api/skills/added", "", "", 401)
	f.request(t, "GET", "/api/skills/"+skill.SkillID, "platform_session=alice", "", 200)
	f.request(t, "GET", "/api/skills/"+skill.SkillID, "platform_session=bob", "", 403)
	f.request(t, "POST", "/api/skills/"+skill.SkillID+"/add", "platform_session=bob", "{}", 403)
	f.request(t, "POST", "/api/skills/"+skill.SkillID+"/add", "platform_session=alice", "{}", 200)
	f.request(t, "PATCH", "/api/skills/"+skill.SkillID+"/agent-mode", "platform_session=alice", `{"mode":"manual"}`, 200)
	added := f.request(t, "GET", "/api/skills/added", "platform_session=alice", "", 200)
	if !strings.Contains(added.Body.String(), skill.SkillID) || !strings.Contains(added.Body.String(), `"agentMode":"manual"`) {
		t.Fatal("native skill attachment or mode was not persisted")
	}
	f.request(t, "DELETE", "/api/skills/"+skill.SkillID+"/add", "platform_session=alice", "", 400)
	public, err := f.svc.CreateSkill(alice.ID, service.SkillMutationRequest{SkillName: "Public reference helper", Description: "Reference guidance", Instruction: "Use the selected reference only.", Tag: "creative"})
	if err != nil {
		t.Fatal(err)
	}
	f.request(t, "POST", "/api/skills/"+public.SkillID+"/add", "platform_session=bob", "{}", 200)
	f.request(t, "DELETE", "/api/skills/"+public.SkillID+"/add", "platform_session=bob", "", 200)
	bob := f.request(t, "GET", "/api/skills?scope=created", "platform_session=bob", "", 200)
	if strings.Contains(bob.Body.String(), skill.SkillID) {
		t.Fatal("another account received private skill")
	}
	for _, path := range []string{"/api/skills", "/api/skills/install/github"} {
		f.request(t, "POST", path, "platform_session=alice", "{}", 501)
	}
	f.request(t, "PUT", "/api/admin/skill-curation", "platform_session=alice", "{}", 501)
}
