package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func studioWorkspaceFixture(f *studioFixture, selection string) *[]string {
	paths := []string{}
	f.workspace = func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "workspace_nonce=next; Secure; HttpOnly")
		personal := gin.H{"id": "personal-101", "kind": "personal", "status": "active", "actor_user_id": 101, "actions": []string{"workspace:read"}}
		team := gin.H{"id": "team_alpha", "kind": "team", "name": "Alpha", "status": "active", "actor_user_id": 101, "membership_generation": 2, "actions": []string{"workspace:read"}}
		var data any
		switch r.URL.Path {
		case "/api/platform/v1/me/workspace-selection":
			data = gin.H{"actor_user_id": 101, "state": selection, "workspace_id": "team_alpha", "membership_generation": 2, "workspace": team}
		case "/api/platform/v1/me/workspaces":
			data = gin.H{"items": []any{personal}, "personal_workspace_id": "personal-101"}
		case "/api/platform/v1/workspaces/personal-101/context":
			data = personal
		case "/api/platform/v1/workspaces/team_alpha/context":
			data = team
		default:
			w.WriteHeader(403)
			return
		}
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": data})
	}
	return &paths
}

func TestStudioWorkspaceEntryResolvesBeforePersonalData(t *testing.T) {
	for _, tt := range []struct {
		name, query, selection, workspace string
		status                            int
	}{
		{"explicit team", "?workspace_id=team_alpha", "unavailable", "team_alpha", 200},
		{"confirmed team", "", "available", "team_alpha", 200},
		{"unavailable default", "", "unavailable", "", 409},
		{"unset default", "", "unset", "personal-101", 200},
		{"old generation", "?workspace_id=team_alpha&membership_generation=1", "available", "", 409},
		{"not a member", "?workspace_id=other_team", "available", "", 403},
		{"invalid generation", "?workspace_id=team_alpha&membership_generation=abc", "available", "", 400},
		{"duplicate intent", "?workspace_id=team_alpha&workspace_id=personal-101", "available", "", 400},
		{"array intent", "?workspace_id%5B%5D=team_alpha&workspace_id%5B%5D=personal-101", "unset", "", 400},
		{"malformed intent", "?workspace_id=team_alpha;personal-101", "unset", "", 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newStudioFixture(t)
			paths := studioWorkspaceFixture(f, tt.selection)
			response := f.request(t, "GET", "/api/auth/workspace-context"+tt.query, "platform_session=alice", "", tt.status)
			if tt.status == 200 && !strings.Contains(response.Body.String(), `"workspaceId":"`+tt.workspace+`"`) {
				t.Fatal("resolved the wrong workspace")
			}
			if tt.workspace == "team_alpha" && !strings.Contains(response.Body.String(), `"supported":false`) {
				t.Fatal("team persistence must not be advertised before ST2")
			}
			if tt.workspace == "personal-101" && !strings.Contains(response.Body.String(), `"supported":true`) {
				t.Fatal("existing personal entry must remain usable")
			}
			if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("workspace context must not be cached")
			}
			for _, path := range *paths {
				if strings.HasPrefix(tt.query, "?workspace_id=team_alpha") && strings.Contains(path, "personal") {
					t.Fatal("explicit team intent read personal context")
				}
			}
		})
	}
}

func TestStudioWorkspaceDeepLinkKeepsStoredPersonalOwnership(t *testing.T) {
	f := newStudioFixture(t)
	paths := studioWorkspaceFixture(f, "available")
	f.request(t, "PATCH", "/api/canvas-projects/original/operations", "platform_session=alice", `{"mutationId":"create-original","operations":[{"type":"create_canvas","id":"original","patch":{"title":"Personal"}}]}`, 200)
	response := f.request(t, "GET", "/api/auth/workspace-context?canvas_id=original&workspace_id=team_alpha", "platform_session=alice", "", 200)
	if !strings.Contains(response.Body.String(), `"workspaceId":"personal-101"`) {
		t.Fatal("navigation intent replaced original canvas ownership")
	}
	for _, path := range *paths {
		if strings.HasSuffix(path, "workspace-selection") {
			t.Fatal("an existing canvas must not use a new-entry preference")
		}
	}
	f.request(t, "GET", "/api/auth/workspace-context?canvas_id=original", "platform_session=bob", "", 404)
}
