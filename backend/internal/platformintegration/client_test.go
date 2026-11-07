package platformintegration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSessionAuthorityAndCookieRotation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "session=fixture" || r.Header.Get("X-Platform-Product-Key") != "" || r.Header.Get("X-Forwarded-Host") != "" {
			t.Error("identity credentials were rewritten or elevated")
		}
		w.Header().Add("Set-Cookie", "session=rotated; Secure; HttpOnly")
		w.Header().Add("Set-Cookie", "nonce=next; Secure; HttpOnly")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"user":{"id":42},"session":{"sid":"reference"}}}`))
	}))
	defer server.Close()
	env := map[string]string{
		"STUDIO_PLATFORM_ORIGIN":          server.URL,
		"STUDIO_PLATFORM_PRODUCT_KEY":     "fixture.secret",
		"STUDIO_PLATFORM_SESSION_HOSTS":   "studio.example.test",
		"STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test",
	}
	client, err := NewFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://studio.example.test/api/auth/me", nil)
	req.Header.Set("Cookie", "session=fixture")
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	result, err := client.Identity(context.Background(), Session(req))
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.User.ID != 42 {
		t.Fatal("identity not decoded")
	}
	headers := http.Header{}
	result.Response.AppendCookies(headers)
	if len(headers.Values("Set-Cookie")) != 2 {
		t.Fatal("cookie rotation lost")
	}
	// A mutation must not invent an Origin from the trusted host.
	if _, err := client.Logout(context.Background(), Session(req)); err == nil {
		t.Fatal("missing Origin accepted")
	}
	req.Host = "attacker.example"
	req.Header.Set("X-Forwarded-Host", "studio.example.test")
	if _, err := client.Identity(context.Background(), Session(req)); err == nil {
		t.Fatal("forwarded host granted access")
	}
	if calls.Load() != 1 {
		t.Fatal("rejected requests reached Platform")
	}
}

func TestMissingConfigurationDoesNotExposeKey(t *testing.T) {
	_, err := NewFromEnv(func(k string) string {
		if k == "STUDIO_PLATFORM_PRODUCT_KEY" {
			return "private-fixture-key"
		}
		return ""
	})
	if err == nil || strings.Contains(err.Error(), "private-fixture-key") {
		t.Fatal("configuration must fail without exposing credentials")
	}
}
