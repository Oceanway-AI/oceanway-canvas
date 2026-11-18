package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStudioSharedAccountControlsUsePublishedResourcesAndFreshCatalog(t *testing.T) {
	f := newStudioFixture(t)
	studioWorkspaceFixture(f, "unset")
	available := true
	f.account = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("workspace_id") != "personal-101" {
			t.Error("missing explicit workspace")
		}
		apps := []gin.H{{"product": "drama", "url": "https://drama.example.test/drama"}}
		if !available {
			apps = nil
		}
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{
			"version": "oceanway.account-summary.v1", "product": "studio", "user": gin.H{"id": 101, "status": 1},
			"workspace":  gin.H{"id": "personal-101", "kind": "personal", "actor_user_id": 101},
			"balance":    gin.H{"state": "ready", "kind": "personal", "available_points": "0"},
			"membership": gin.H{"state": "ready", "active_count": 0},
			"benefits":   gin.H{"state": "unavailable"}, "capabilities": gin.H{"can_read_finance": true, "can_purchase": false},
			"applications": apps,
		}})
	}
	response := f.request(t, "GET", "/api/auth/account-summary", "platform_session=alice", "", 200)
	var body struct {
		Data struct {
			UserID     string `json:"userId"`
			Membership struct {
				State       string
				ActiveCount *int `json:"active_count"`
			}
			Benefits     struct{ State string }
			Capabilities struct {
				CanPurchase bool `json:"can_purchase"`
			}
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Membership.State != "ready" || body.Data.Membership.ActiveCount == nil || *body.Data.Membership.ActiveCount != 0 || body.Data.Benefits.State != "unavailable" || body.Data.Capabilities.CanPurchase {
		t.Fatal("partial resource states were lost")
	}
	q := url.Values{"user_id": {body.Data.UserID}, "workspace_id": {"personal-101"}, "membership_generation": {"0"}, "product": {"drama"}}
	result := f.request(t, "GET", "/api/auth/product-navigation?"+q.Encode(), "platform_session=alice", "", 200)
	if !strings.Contains(result.Body.String(), "https://drama.example.test/drama?membership_generation=0") || !strings.Contains(result.Body.String(), "workspace_id=personal-101") {
		t.Fatal("registered path or original context lost")
	}
	available = false
	f.request(t, "GET", "/api/auth/product-navigation?"+q.Encode(), "platform_session=alice", "", 403)
	q.Set("user_id", "stale")
	f.request(t, "GET", "/api/auth/product-navigation?"+q.Encode(), "platform_session=alice", "", 409)
	q.Add("workspace_id", "team_other")
	f.request(t, "GET", "/api/auth/product-navigation?"+q.Encode(), "platform_session=alice", "", 400)
}

func TestStudioWorkspaceChoicesKeepTeamsClosedAndRejectOtherActors(t *testing.T) {
	f := newStudioFixture(t)
	actor := 101
	f.workspace = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/platform/v1/me/workspaces" {
			t.Error("list must not mutate preference")
		}
		w.Header().Add("Set-Cookie", "choice_nonce=rotated; HttpOnly; Secure")
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{"items": []gin.H{
			{"id": "personal-101", "kind": "personal", "status": "active", "actor_user_id": actor, "actions": []string{"workspace:read"}},
			{"id": "team_alpha", "name": "Alpha", "kind": "team", "membership_generation": 2, "status": "active", "actor_user_id": actor, "actions": []string{"workspace:read"}},
		}}})
	}
	result := f.request(t, "GET", "/api/auth/workspaces", "platform_session=alice", "", 200)
	if !strings.Contains(result.Body.String(), `"name":"Alpha"`) || !strings.Contains(result.Body.String(), `"supported":false`) || strings.Contains(result.Body.String(), "owner") || !strings.Contains(result.Header().Get("Set-Cookie"), "choice_nonce=") {
		t.Fatal("list, unavailable status or rotated cookie lost")
	}
	actor = 202
	f.request(t, "GET", "/api/auth/workspaces", "platform_session=alice", "", 503)
	f.request(t, "GET", "/api/auth/workspaces", "", "", 401)
}
