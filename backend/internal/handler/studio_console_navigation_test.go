package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/model"
	"yingce/backend/internal/service"
)

func TestStudioConsoleNavigationKeepsOriginalAccountAndPage(t *testing.T) {
	f := newStudioFixtureWithBase(t, "/studio")
	profile := f.request(t, "GET", "/api/auth/profile", "platform_session=alice", "", 200)
	var identity struct {
		Data struct{ User struct{ ID string } }
	}
	if err := json.Unmarshal(profile.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	userID := identity.Data.User.ID
	for _, canvas := range []model.CanvasProject{{ID: "original-canvas", UserID: userID, PayloadJSON: "{}"}, {ID: "foreign", UserID: "other", PayloadJSON: "{}"}} {
		if err := f.db.Create(&canvas).Error; err != nil {
			t.Fatal(err)
		}
	}
	skill, err := f.svc.CreateSkill(userID, service.SkillMutationRequest{SkillName: "导航测试", Description: "技能返回测试", Instruction: "# 导航测试\n正文", Tag: "creative"})
	if err != nil {
		t.Fatal(err)
	}
	kind, actor, rejected, malformed := "personal", 101, false, false
	navigationCalls := 0
	f.account = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Platform-Product-Key") != "fixture.secret" || r.URL.RawQuery != "" {
			t.Error("personal account authority missing")
		}
		cookie, err := r.Cookie("platform_session")
		if err != nil || cookie.Value != "alice" {
			w.WriteHeader(401)
			return
		}
		w.Header().Add("Set-Cookie", "summary_nonce=next; Secure; HttpOnly")
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{"user": gin.H{"id": actor, "status": 1}, "workspace": gin.H{"id": "personal-101", "kind": kind, "actor_user_id": actor}, "applications": []any{}}})
	}
	f.navigation = func(w http.ResponseWriter, r *http.Request) {
		navigationCalls++
		q := r.URL.Query()
		if r.Header.Get("X-Platform-Product-Key") != "fixture.secret" || r.Header.Get("Cookie") == "" || q.Get("workspace_id") != "personal-101" || q.Get("membership_generation") != "0" {
			t.Error("navigation authority mismatch")
		}
		w.Header().Add("Set-Cookie", "navigation_nonce=next; Secure; HttpOnly")
		if rejected {
			w.WriteHeader(403)
			return
		}
		paths := map[string]string{"wallet": "/console/wallet", "account": "/console/account", "usage": "/console/records", "subscriptions": "/console/subscriptions", "plans": "/console/plans", "orders": "/console/orders", "recharge": "/console/wallet"}
		params := url.Values{"workspace_id": {"personal-101"}, "return_workspace_id": {"personal-101"}, "return_generation": {"0"}, "return_product": {"studio"}, "return_path": {q.Get("return_path")}}
		if q.Get("destination") == "recharge" {
			params.Set("recharge", "1")
		}
		consolePath := paths[q.Get("destination")] + "?" + params.Encode()
		if malformed {
			consolePath = "https://other.example/console/wallet"
		}
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{"version": "oceanway.console-navigation.v1", "actor_user_id": 101, "product": "studio", "workspace_id": "personal-101", "membership_generation": 0, "destination": q.Get("destination"), "return_path": q.Get("return_path"), "console_path": consolePath, "return_url": "https://studio.example.test" + q.Get("return_path") + "?workspace_id=personal-101"}})
	}
	query := url.Values{"user_id": {userID}, "workspace_id": {"personal-101"}, "destination": {"wallet"}, "return_path": {"/studio/canvas/original-canvas"}}
	request := func(status int) *http.Response {
		return f.request(t, "GET", "/api/auth/console-navigation?"+query.Encode(), "platform_session=alice", "", status).Result()
	}
	for _, returnPath := range []string{"/studio", "/studio/create", "/studio/tasks", "/studio/assets", "/studio/settings", "/studio/canvas", "/studio/skills", "/studio/skills/" + skill.SkillID + "/edit", "/studio/canvas/original-canvas"} {
		for _, destination := range []string{"account", "models", "wallet", "usage", "subscriptions", "plans", "orders", "recharge"} {
			query.Set("return_path", returnPath)
			query.Set("destination", destination)
			response := request(302)
			location, err := response.Location()
			if err != nil || location.Host != "console.example.test" || location.Query().Get("return_path") != returnPath || location.Query().Get("return_workspace_id") != "personal-101" {
				t.Fatal("original page or configured Console origin lost")
			}
			if destination == "models" && (location.Path != "/console/account" || location.Query().Get("section") != "models") {
				t.Fatal("shared model settings not selected")
			}
			if response.Header.Get("Cache-Control") != "private, no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" || len(response.Cookies()) != 2 {
				t.Fatal("private response/cookie propagation missing")
			}
		}
	}
	query.Set("destination", "wallet")
	before := navigationCalls
	for _, path := range []string{"/studio/canvas/foreign", "/studio/canvas/missing"} {
		query.Set("return_path", path)
		request(404)
	}
	for _, path := range []string{"//evil.test", "/drama/create", "/studio/create?next=x", "/studio/create#x", "/studio/create/", "/studio//create", "/studio/canvas/../create", "/studio/canvas/a%2Fb", "/studio/canvas/a/b", "/studio/skills/missing/edit", "/studio/unknown", "/studio/skills/x/edit/extra", "/studio/skills/x"} {
		query.Set("return_path", path)
		request(400)
	}
	query.Set("return_path", "/studio/create")
	query.Set("user_id", "stale-user")
	request(409)
	query.Set("user_id", userID)
	query.Set("workspace_id", "another-space")
	request(409)
	query.Set("workspace_id", "personal-101")
	query.Set("destination", "invalid")
	request(400)
	query.Set("destination", "wallet")
	query.Add("return_path", "/studio/tasks")
	request(400)
	query.Set("return_path", "/studio/create")
	kind = "team"
	request(503)
	kind = "personal"
	actor = 202
	request(409)
	actor = 101
	if navigationCalls != before {
		t.Fatal("unverified account/path reached navigation provider")
	}
	f.request(t, "GET", "/api/auth/console-navigation?"+query.Encode(), "", "", 401)
	rejected = true
	request(403)
	rejected = false
	malformed = true
	response := request(503)
	if strings.Contains(response.Header.Get("Location"), "other.example") {
		t.Fatal("unverified provider redirect escaped")
	}
}
