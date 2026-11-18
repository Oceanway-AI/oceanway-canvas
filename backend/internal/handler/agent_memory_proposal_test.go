package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yingce/backend/internal/model"
)

func TestAgentMemoryProposalHTTPVersionAndOwnership(t *testing.T) {
	f := newStudioFixture(t)
	db := f.db
	for _, row := range []any{
		&model.AgentMemoryProposal{ID: "owned", UserID: "101", Status: model.AgentMemoryProposalPending, Version: 2, InputJSON: "[]", ChangesJSON: "[]", SummaryJSON: "{}"},
		&model.AgentMemoryProposal{ID: "foreign", UserID: "other", Status: model.AgentMemoryProposalPending, Version: 2, InputJSON: "[]", ChangesJSON: "[]", SummaryJSON: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	registerAgentMemoryProposalRoutes(f.router.Group("/api"), f.svc)
	for _, tc := range []struct {
		method, path, body string
		authenticated      bool
		status             int
	}{
		{"GET", "/api/agent/memories/proposals", "", false, 401},
		{"GET", "/api/agent/memories/proposals", "", true, 200},
		{"GET", "/api/agent/memories/revisions", "", true, 200},
		{"POST", "/api/agent/memories/proposals/foreign/decide", `{"decision":"reject","version":2}`, true, 404},
		{"POST", "/api/agent/memories/proposals/owned/decide", `{"decision":"reject"}`, true, 400},
		{"POST", "/api/agent/memories/proposals/owned/decide", `{"decision":"reject","version":1}`, true, 409},
		{"POST", "/api/agent/memories/proposals/owned/decide", `{"decision":"reject","version":2,"userId":"other"}`, true, 400},
		{"POST", "/api/agent/memories/proposals/owned/decide", `{"decision":"reject","version":2}`, true, 200},
		{"POST", "/api/agent/memories/proposals/owned/decide", `{"decision":"approve","version":2}`, true, 409},
		{"POST", "/api/agent/memories/revisions/missing/restore", `{"version":1}`, true, 404},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		if tc.authenticated {
			r.AddCookie(&http.Cookie{Name: "platform_session", Value: "alice"})
		}
		r.Host = "studio.example.test"
		r.Header.Set("Origin", "https://studio.example.test")
		r.Header.Set("X-Studio-Identity-Version", "platform-id-v1")
		r.Header.Set("X-Studio-Actor-ID", "101")
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d expected %d: %s", tc.method, tc.path, w.Code, tc.status, w.Body.String())
		}
	}
}
