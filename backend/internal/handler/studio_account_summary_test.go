package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStudioAccountSummaryUsesCurrentPlatformPayer(t *testing.T) {
	f := newStudioFixture(t)
	f.avatar = "data:image/png;base64,fixture"
	actorID := 101
	kind := "personal"
	f.account = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("X-Platform-Product-Key") != "fixture.secret" {
			t.Error("personal summary must use product credentials and the implicit personal space")
		}
		cookie, err := r.Cookie("platform_session")
		if err != nil || cookie.Value != "alice" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		w.Header().Add("Set-Cookie", "account_nonce=next; Secure; HttpOnly")
		_ = json.NewEncoder(w).Encode(gin.H{"success": true, "data": gin.H{
			"user":         gin.H{"id": 101, "status": 1, "display_name": "中央资料", "username": "account", "avatar_url": f.avatar},
			"workspace":    gin.H{"kind": kind, "id": "personal-101", "name": "个人空间", "actor_user_id": actorID},
			"balance":      gin.H{"kind": "personal", "available_points": "-0.012345678901"},
			"applications": []gin.H{{"product": "console", "url": "/console"}, {"product": "studio", "url": "https://studio.example.test"}, {"product": "bad", "url": "javascript:void(0)"}},
			"account_path": "/console/account", "wallet_path": "/console/wallet", "usage_path": "/console/records",
		}})
	}
	identity := f.request(t, "GET", "/api/auth/profile", "platform_session=alice", "", 200)
	var profile struct {
		Data struct {
			User struct {
				ID        string `json:"id"`
				AvatarURL string `json:"avatarUrl"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(identity.Body.Bytes(), &profile); err != nil || profile.Data.User.ID == "" {
		t.Fatal("profile missing")
	}
	response := f.request(t, "GET", "/api/auth/account-summary", "platform_session=alice", "", 200)
	var body struct {
		Data struct {
			UserID    string `json:"userId"`
			AvatarURL string `json:"avatarUrl"`
			UsageURL  string `json:"usageUrl"`
			Balance   struct {
				Points string `json:"available_points"`
			} `json:"balance"`
			Applications []struct {
				URL string `json:"url"`
			} `json:"applications"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.UserID != profile.Data.User.ID || body.Data.Balance.Points != "-0.012345678901" || body.Data.AvatarURL != f.avatar || profile.Data.User.AvatarURL != f.avatar || !strings.HasSuffix(body.Data.UsageURL, "/console/records") || len(body.Data.Applications) != 2 {
		t.Fatal("summary lost the account binding, exact balance, avatar or registered destinations")
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") || !strings.Contains(response.Header().Get("Set-Cookie"), "account_nonce=") {
		t.Fatal("summary must remain private and preserve rotated cookies")
	}
	actorID = 202
	f.request(t, "GET", "/api/auth/account-summary", "platform_session=alice", "", 503)
	actorID = 101
	kind = "team"
	f.request(t, "GET", "/api/auth/account-summary", "platform_session=alice", "", 503)
	f.request(t, "GET", "/api/auth/account-summary", "platform_session=bob", "", 401)
}
