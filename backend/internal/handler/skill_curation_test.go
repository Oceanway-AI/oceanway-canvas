package handler

import "testing"

func TestSkillCurationHTTP(t *testing.T) {
	f := newStudioFixture(t)
	RegisterAdminSkillRoutes(f.router.Group("/api"), f.svc)
	for _, tc := range []struct {
		role, method, path, body string
		status                   int
	}{
		{"", "GET", "/api/skills/curation", "", 401},
		{"user", "GET", "/api/admin/skill-curation", "", 403},
		{"user", "PUT", "/api/admin/skill-curation", `{"expectedRevision":0,"enabled":true}`, 403},
		{"user", "GET", "/api/skills/curation", "", 200},
		{"admin", "PUT", "/api/admin/skill-curation", `{"enabled":true}`, 400},
		{"admin", "PUT", "/api/admin/skill-curation", `{"expectedRevision":0,"enabled":true,"requestId":"forged"}`, 400},
		{"admin", "PUT", "/api/admin/skill-curation", `{"expectedRevision":0,"enabled":true} {}`, 400},
		{"admin", "PUT", "/api/admin/skill-curation", `{"expectedRevision":0,"enabled":true}`, 200},
		{"admin", "PUT", "/api/admin/skill-curation", `{"expectedRevision":0,"enabled":false}`, 409},
	} {
		f.admin = tc.role == "admin"
		cookie := ""
		if tc.role != "" {
			cookie = "platform_session=alice"
		}
		f.request(t, tc.method, tc.path, cookie, tc.body, tc.status)
	}
}
