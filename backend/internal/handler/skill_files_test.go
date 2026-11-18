package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yingce/backend/internal/service"
)

func TestSkillPackageFilesEditHTTP(t *testing.T) {
	f := newStudioFixture(t)
	skill, err := f.svc.CreateSkill("101", service.SkillMutationRequest{SkillName: "分镜助手", Description: "把剧本拆成镜头", Instruction: "# 分镜助手\n\n正文\n", Tag: "creative"})
	if err != nil {
		t.Fatal(err)
	}
	send := func(user string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/skills/"+skill.SkillID+"/files", strings.NewReader(body))
		req.Host = "studio.example.test"
		req.Header.Set("Origin", "https://studio.example.test")
		req.Header.Set("X-Studio-Identity-Version", "platform-id-v1")
		actor, cookie := "101", "alice"
		if user == "other" {
			actor, cookie = "202", "bob"
		}
		req.Header.Set("X-Studio-Actor-ID", actor)
		req.Header.Set("Content-Type", "application/json")
		if user != "" {
			req.AddCookie(&http.Cookie{Name: "platform_session", Value: cookie})
		}
		out := httptest.NewRecorder()
		f.router.ServeHTTP(out, req)
		return out
	}
	body := `{"baseVersionId":"` + skill.VersionID + `","operations":[{"op":"write","path":"references/camera.md","content":"# 镜头"}]}`
	if out := send("", body); out.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d %s", out.Code, out.Body.String())
	}
	if out := send("other", body); out.Code != http.StatusForbidden {
		t.Fatalf("non-owner: %d %s", out.Code, out.Body.String())
	}
	if out := send("owner", `{"baseVersionId":"`+skill.VersionID+`","operations":[{"op":"write","path":"../x.md","content":"x"}]}`); out.Code != http.StatusBadRequest {
		t.Fatalf("traversal: %d %s", out.Code, out.Body.String())
	}
	out := send("owner", body)
	if out.Code != http.StatusOK {
		t.Fatalf("owner: %d %s", out.Code, out.Body.String())
	}
	var success struct {
		Code int `json:"code"`
		Data struct {
			Changed bool `json:"changed"`
			Skill   struct {
				VersionID string `json:"versionId"`
			} `json:"skill"`
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &success); err != nil {
		t.Fatal(err)
	}
	if success.Code != 0 || !success.Data.Changed || success.Data.Skill.VersionID == skill.VersionID || len(success.Data.Files) != 2 {
		t.Fatalf("unexpected success body: %s", out.Body.String())
	}
	stale := send("owner", body)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"reason":"skill_version_conflict"`) || !strings.Contains(stale.Body.String(), success.Data.Skill.VersionID) {
		t.Fatalf("stale base: %d %s", stale.Code, stale.Body.String())
	}
}
